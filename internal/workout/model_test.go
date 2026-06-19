package workout_test

import (
	"encoding/json"
	"strings"
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

// genuineDetail940759 is the real `cttTrainingInfoDetail/940759` data array
// captured in issue #23 (2026-06-18): a hammer-curl exercise whose set 1 carries
// the full per-rep telemetry and whose set 2 is a sparse capture (weights only).
// It is the fixture the faithful-passthrough guards assert against.
const genuineDetail940759 = `[
  {
    "actionLibraryName": "Standing Dual-Handle Hammer Curl",
    "score": 16, "completionScore": 5, "forceControlScore": 4,
    "bilateralBalanceScore": 4, "amplitudeStableScore": 3, "actionRating": 3,
    "maxWeight": 15.0, "maxWeightCount": 5, "totalCapacity": 554.0,
    "finishedReps": [
      {
        "finishedCount": 14, "targetCount": 14, "capacity": 330.0, "time": 14, "leftRight": 0, "breakTime": 60,
        "trainingInfoDetail": {
          "weights":           [15,15,15,15,15,10,10,10,10,10,10,10,10,10],
          "leftWeights":       [15,15,15,15,15,10,10,10,10,10,10,10,10,10],
          "rightWeights":      [15,15,15,15,15,10,10,10,10,10,10,10,10,10],
          "leftWatts":         [41.65,51.84,49.19,44.87,41.33,35.67,28.90,27.35,25.21,35.90,29.44,28.50,32.58,27.50],
          "rightWatts":        [26.28,55.05,54.90,47.86,39.97,37.79,33.65,28.38,24.25,28.60,29.96,29.07,33.35,27.62],
          "leftAmplitudes":    [0.46,0.68,0.65,0.71,0.69,0.65,0.62,0.67,0.70,0.73,0.74,0.72,0.71,0.73],
          "rightAmplitudes":   [0.46,0.66,0.67,0.70,0.67,0.76,0.66,0.65,0.73,0.78,0.70,0.73,0.73,0.73],
          "leftRopeSpeeds":    [0.66,0.80,0.76,0.71,0.65,0.83,0.67,0.63,0.58,0.86,0.70,0.68,0.77,0.64],
          "leftFinishedTimes": [1.13,4.69,2.79,2.94,3.16,2.17,2.82,2.59,2.96,1.26,2.95,3.03,3.01,2.87],
          "leftBreakTimes":    [1.23,0.42,0.14,0.07,0.35,0.70,0,0.14,1.61,0,0.28,0.14,1.96,0.14],
          "leftTimestamps":    [1781815035511]
        }
      },
      {
        "finishedCount": 14, "targetCount": 14, "capacity": 224.0, "leftRight": 0,
        "trainingInfoDetail": { "weights": [8,8,8,8,8,8,8,8,8,8,8,8,8,8] }
      }
    ]
  }
]`

// TestSessionDetailIsVerbatimPassthrough is the faithful-output guard: the
// session document carries the raw Speediance payloads through unaltered — every
// server key reaches the consumer with its original name, and none of the
// derived/renamed fields from the superseded design appear. (Issue #23: the prior
// typed/derived output both fabricated weights and silently dropped this exact
// telemetry — this asserts neither can recur.)
func TestSessionDetailIsVerbatimPassthrough(t *testing.T) {
	info := json.RawMessage(`{"completionRate":0.95,"trainingId":940759,"totalCapacity":554.0}`)
	sd := workout.SessionDetail{
		TrainingID: 940759,
		Info:       info,
		Detail:     json.RawMessage(genuineDetail940759),
	}
	b, err := json.Marshal(sd)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)

	// Both endpoints are emitted (the earlier dump showed only one was, so
	// completionRate was lost): the session-level info payload is present verbatim.
	if !strings.Contains(out, `"completionRate":0.95`) {
		t.Errorf("info payload (completionRate) not passed through:\n%s", out)
	}

	// Every Speediance key flows through verbatim and unrenamed.
	for _, key := range []string{
		`"forceControlScore"`, `"bilateralBalanceScore"`, `"amplitudeStableScore"`,
		`"completionScore"`, `"actionRating"`, `"score"`, `"maxWeight"`, `"maxWeightCount"`,
		`"actionLibraryName"`, `"finishedReps"`, `"trainingInfoDetail"`,
		`"weights"`, `"leftWeights"`, `"rightWeights"`,
		`"leftWatts"`, `"rightWatts"`, `"leftAmplitudes"`, `"rightAmplitudes"`,
		`"leftRopeSpeeds"`, `"leftFinishedTimes"`, `"leftBreakTimes"`, `"leftTimestamps"`,
	} {
		if !strings.Contains(out, key) {
			t.Errorf("verbatim key %s missing from session output", key)
		}
	}

	// None of the derived / renamed / "smart" fields from the superseded design.
	for _, banned := range []string{
		"weight_source", "weight_avg_per_handle", "derived_avg", "reps_detail",
		"left_watts", "right_watts", "max_hr", "completion_rate", "exercises",
	} {
		if strings.Contains(out, banned) {
			t.Errorf("session output contains forbidden derived/renamed field %q:\n%s", banned, out)
		}
	}

	// The sparse set is preserved faithfully: only the fields Speediance actually
	// returned for it (weights) — no fabricated gap-fill of the missing arrays.
	var doc struct {
		Detail []struct {
			FinishedReps []struct {
				Detail map[string]json.RawMessage `json:"trainingInfoDetail"`
			} `json:"finishedReps"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	sparse := doc.Detail[0].FinishedReps[1].Detail
	if _, ok := sparse["weights"]; !ok {
		t.Errorf("sparse set lost its only real field (weights): %v", sparse)
	}
	if len(sparse) != 1 {
		t.Errorf("sparse set gained fabricated fields, want only weights, got %v", sparse)
	}
}

// TestSessionDetailNilPayloadsAreNull asserts absence is preserved, not invented:
// a session for which Speediance returns nothing emits JSON null for each payload
// (never {} or a fabricated default).
func TestSessionDetailNilPayloadsAreNull(t *testing.T) {
	b, err := json.Marshal(workout.SessionDetail{TrainingID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"training_id":7,"kind":"","info":null,"detail":null}`; got != want {
		t.Errorf("nil payloads:\n got %s\nwant %s", got, want)
	}
}
