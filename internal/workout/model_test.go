package workout_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stozo04/speediance-cli/internal/workout"
)

func TestParseRecordsFallbacks(t *testing.T) {
	// trainingId 0 falls back to id; empty title falls back to "Workout";
	// type falls back to courseCategoryName.
	data := json.RawMessage(`[
		{"id":555,"trainingId":0,"courseCategoryName":"Cardio","trainingTime":600,"calorie":50},
		{"trainingId":777,"title":"Leg Day","courseTypeStr":"Strength","totalCapacity":1234}
	]`)
	ws, err := workout.ParseRecords(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 {
		t.Fatalf("got %d", len(ws))
	}
	a := ws[0].Summary()
	if a.TrainingID != 555 {
		t.Errorf("training_id fallback to id failed: %d", a.TrainingID)
	}
	if a.Title != "Workout" {
		t.Errorf("title fallback failed: %q", a.Title)
	}
	if a.Type != "Cardio" {
		t.Errorf("type fallback failed: %q", a.Type)
	}
	// totalCapacity absent => volume defaults to 0.0 (not "0").
	if a.Volume != json.Number("0.0") {
		t.Errorf("default volume = %q, want 0.0", a.Volume)
	}

	b := ws[1].Summary()
	if b.TrainingID != 777 || b.Title != "Leg Day" || b.Type != "Strength" {
		t.Errorf("record 2 wrong: %+v", b)
	}
	// integer totalCapacity stays integer-formed.
	if b.Volume != json.Number("1234") {
		t.Errorf("volume = %q, want 1234", b.Volume)
	}
}

func TestDateMilliseconds(t *testing.T) {
	// A value > 1e12 is milliseconds and must be divided by 1000.
	ms := int64(1718400000123)
	data := json.RawMessage(`[{"trainingId":1,"startTimestamp":1718400000123}]`)
	ws, _ := workout.ParseRecords(data)
	got := ws[0].Date()
	want := time.Unix(ms/1000, 0).Format("2006-01-02")
	if got == nil || *got != want {
		t.Errorf("date = %v, want %s", got, want)
	}
}

func TestDateNilWhenNoTimestamp(t *testing.T) {
	data := json.RawMessage(`[{"trainingId":1}]`)
	ws, _ := workout.ParseRecords(data)
	if d := ws[0].Date(); d != nil {
		t.Errorf("date = %v, want nil", *d)
	}
}

func TestSessionOutputEmptyExercisesIsArray(t *testing.T) {
	// A freestyle session with no detail must encode exercises as [] not null.
	w := workout.Workout{TrainingID: 9}
	out := w.SessionOutput()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"exercises":[]`)) {
		t.Errorf("empty exercises not encoded as []: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte(`"completion_rate":0.0`)) {
		t.Errorf("default completion_rate not 0.0: %s", buf.String())
	}
}

func TestSessionWeightFallsBackToMaxWeight(t *testing.T) {
	w := workout.Workout{TrainingID: 1}
	// rep omits "weight" => use exercise maxWeight (45); grouping by name.
	detail := json.RawMessage(`[
		{"actionLibraryName":"Row","maxWeight":45,"finishedReps":[
			{"finishedCount":12,"targetCount":12,"leftRight":0},
			{"finishedCount":10,"targetCount":12,"weight":50.0,"maxHeartRate":140}
		]}
	]`)
	if err := w.AddDetailSets(detail); err != nil {
		t.Fatal(err)
	}
	out := w.SessionOutput()
	if len(out.Exercises) != 1 || out.Exercises[0].Name != "Row" {
		t.Fatalf("exercises wrong: %+v", out.Exercises)
	}
	sets := out.Exercises[0].Sets
	if sets[0].Weight != json.Number("45") {
		t.Errorf("set 1 weight = %q, want 45 (maxWeight fallback)", sets[0].Weight)
	}
	if sets[1].Weight != json.Number("50.0") {
		t.Errorf("set 2 weight = %q, want 50.0", sets[1].Weight)
	}
	if sets[0].MaxHR != json.Number("0.0") {
		t.Errorf("set 1 max_hr = %q, want 0.0 default", sets[0].MaxHR)
	}
}
