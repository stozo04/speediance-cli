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
	out := w.SessionOutput(false)
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

// TestSessionWeightNeverUsesMaxWeight is the negative-assertion regression guard
// for issue #23: a completed-program set whose top-level weight is null must NOT
// inherit the *planned* exercise maxWeight (the old fabrication). It must instead
// report the real performed load derived from the per-rep telemetry, tagged
// "derived_avg"; a set that does carry a real weight stays "actual"; a set with
// no load signal at all is "unavailable", not maxWeight. Reintroducing the
// maxWeight fallback fails this loudly.
func TestSessionWeightNeverUsesMaxWeight(t *testing.T) {
	w := workout.Workout{TrainingID: 1}
	// Set 1: weight null, but per-rep telemetry present (a 15x5 -> 10x9 drop set,
	// exactly the issue's hammer-curl example). maxWeight is the planned 15.
	// Set 2: a real per-rep weight (50) -> reported verbatim as "actual".
	// Set 3: weight null AND no telemetry/capacity -> "unavailable" 0.0.
	detail := json.RawMessage(`[
		{"actionLibraryName":"Row","maxWeight":15,"finishedReps":[
			{"finishedCount":14,"targetCount":14,"capacity":330,"leftRight":0,
			 "trainingInfoDetail":{"weights":[15,15,15,15,15,10,10,10,10,10,10,10,10,10]}},
			{"finishedCount":10,"targetCount":12,"weight":50.0,"maxHeartRate":140},
			{"finishedCount":8,"targetCount":8,"leftRight":0}
		]}
	]`)
	if err := w.AddDetailSets(detail); err != nil {
		t.Fatal(err)
	}
	out := w.SessionOutput(false)
	if len(out.Exercises) != 1 || out.Exercises[0].Name != "Row" {
		t.Fatalf("exercises wrong: %+v", out.Exercises)
	}
	sets := out.Exercises[0].Sets

	// Set 1 must be the mean of weights[] (165/14 = 11.785... -> 11.8), tagged
	// derived_avg — and crucially NOT the planned maxWeight of 15.
	if sets[0].Weight == json.Number("15") || sets[0].Weight == json.Number("15.0") {
		t.Errorf("set 1 weight = %q — regressed to the planned maxWeight (issue #23)", sets[0].Weight)
	}
	if sets[0].Weight != json.Number("11.8") {
		t.Errorf("set 1 weight = %q, want 11.8 (mean of weights[])", sets[0].Weight)
	}
	if sets[0].WeightSource != "derived_avg" {
		t.Errorf("set 1 weight_source = %q, want derived_avg", sets[0].WeightSource)
	}
	// capacity is always emitted now.
	if sets[0].Capacity != json.Number("330") {
		t.Errorf("set 1 capacity = %q, want 330", sets[0].Capacity)
	}

	// Set 2: real weight -> verbatim + actual.
	if sets[1].Weight != json.Number("50.0") || sets[1].WeightSource != "actual" {
		t.Errorf("set 2 = {%q, %q}, want {50.0, actual}", sets[1].Weight, sets[1].WeightSource)
	}
	if sets[1].MaxHR != json.Number("140") {
		t.Errorf("set 2 max_hr = %q, want 140", sets[1].MaxHR)
	}

	// Set 3: no signal at all -> 0.0 + unavailable, never maxWeight.
	if sets[2].Weight != json.Number("0.0") || sets[2].WeightSource != "unavailable" {
		t.Errorf("set 3 = {%q, %q}, want {0.0, unavailable}", sets[2].Weight, sets[2].WeightSource)
	}

	// The lean (non-telemetry) view must NOT carry the rich fields.
	if sets[0].RepsDetail != nil || sets[0].WeightAvgPerHandle != nil {
		t.Errorf("non-telemetry set leaked telemetry fields: %+v", sets[0])
	}
	if out.Exercises[0].Scores != nil || out.Exercises[0].MaxWeight != nil {
		t.Errorf("non-telemetry exercise leaked telemetry fields: %+v", out.Exercises[0])
	}
}

// TestSessionTelemetryExposesPerRepDetail verifies --telemetry surfaces the real
// per-rep, per-side arrays and per-exercise scores the API returns, and that
// single-attachment moves (left-only arrays) omit the right-side fields.
func TestSessionTelemetryExposesPerRepDetail(t *testing.T) {
	w := workout.Workout{TrainingID: 1}
	detail := json.RawMessage(`[
		{"actionLibraryName":"Hammer Curl","maxWeight":15,"maxWeightCount":5,
		 "score":16,"completionScore":5,"forceControlScore":4,
		 "bilateralBalanceScore":4,"amplitudeStableScore":3,"actionRating":3,
		 "finishedReps":[
			{"finishedCount":2,"targetCount":2,"capacity":50,"leftRight":0,
			 "trainingInfoDetail":{
				"weights":[15,10],
				"leftWatts":[41.65,35.67],"rightWatts":[26.28,37.79],
				"leftAmplitudes":[0.46,0.65],"rightAmplitudes":[0.46,0.76]}}
		]},
		{"actionLibraryName":"Rope Face Pull","maxWeight":20,"finishedReps":[
			{"finishedCount":2,"targetCount":2,"capacity":40,"leftRight":0,
			 "trainingInfoDetail":{"weights":[10,10],"leftWatts":[30.0,31.0]}}
		]}
	]`)
	if err := w.AddDetailSets(detail); err != nil {
		t.Fatal(err)
	}
	out := w.SessionOutput(true)
	if len(out.Exercises) != 2 {
		t.Fatalf("exercises = %d, want 2", len(out.Exercises))
	}

	hammer := out.Exercises[0]
	if hammer.Scores == nil || hammer.Scores.Total != 16 || hammer.Scores.Completion != 5 {
		t.Errorf("scores wrong: %+v", hammer.Scores)
	}
	if hammer.MaxWeight == nil || *hammer.MaxWeight != json.Number("15") {
		t.Errorf("max_weight = %v, want 15", hammer.MaxWeight)
	}
	if hammer.MaxWeightCount == nil || *hammer.MaxWeightCount != 5 {
		t.Errorf("max_weight_count = %v, want 5", hammer.MaxWeightCount)
	}
	set := hammer.Sets[0]
	if set.WeightAvgPerHandle == nil || *set.WeightAvgPerHandle != json.Number("12.5") {
		t.Errorf("weight_avg_per_handle = %v, want 12.5 (mean of 15,10)", set.WeightAvgPerHandle)
	}
	if len(set.RepsDetail) != 2 {
		t.Fatalf("reps_detail len = %d, want 2", len(set.RepsDetail))
	}
	r0 := set.RepsDetail[0]
	if r0.Rep != 1 || r0.Weight == nil || *r0.Weight != json.Number("15") {
		t.Errorf("rep 1 wrong: %+v", r0)
	}
	if r0.LeftWatts == nil || r0.RightWatts == nil || r0.LeftAmp == nil {
		t.Errorf("rep 1 missing per-side telemetry: %+v", r0)
	}

	// Single-attachment move: left-only arrays -> right-side fields omitted.
	face := out.Exercises[1].Sets[0]
	if rd := face.RepsDetail[0]; rd.LeftWatts == nil || rd.RightWatts != nil {
		t.Errorf("single-attachment rep should have left-only watts: %+v", rd)
	}
}
