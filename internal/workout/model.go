// Package workout models the Speediance data the CLI emits. The workouts list is
// a small typed summary (GOAL.md §9.2); the per-session detail is a faithful,
// lossless passthrough (SessionDetail) — the raw Speediance payloads, unparsed
// and unaltered — so nothing the server returns is ever silently dropped.
//
// Numbers whose int-vs-float form must round-trip exactly (volume, completion
// rate, …) are carried as json.Number so we re-emit the server's value verbatim.
package workout

import (
	"encoding/json"
	"time"
)

// zeroFloat is the default for passthrough number fields, matching Python's 0.0
// default (which serializes as "0.0", not "0").
const zeroFloat = json.Number("0.0")

// Workout is one row of the completed-session list. Only the subset the
// `workouts` summary needs is modeled; the full per-session data is fetched
// separately and emitted verbatim (see SessionDetail).
type Workout struct {
	TrainingID  int64
	Title       string
	WorkoutType string
	StartTS     int64
	EndTS       int64
	DurationSec int64
	Calories    int64
	Volume      json.Number // raw totalCapacity, for verbatim re-emit
}

// rawRecord mirrors a userTrainingDataRecord entry. Pointers distinguish absent
// keys (fall through to a default) from present-but-zero, matching the Python
// `r.get(key, default) or fallback` logic exactly.
type rawRecord struct {
	TrainingID         *json.Number `json:"trainingId"`
	ID                 *json.Number `json:"id"`
	Title              *string      `json:"title"`
	CourseTypeStr      *string      `json:"courseTypeStr"`
	CourseCategoryName *string      `json:"courseCategoryName"`
	StartTimestamp     *json.Number `json:"startTimestamp"`
	EndTimestamp       *json.Number `json:"endTimestamp"`
	TrainingTime       *json.Number `json:"trainingTime"`
	Calorie            *json.Number `json:"calorie"`
	TotalCapacity      *json.Number `json:"totalCapacity"`
}

// ParseRecords decodes the userTrainingDataRecord array into Workouts.
func ParseRecords(data json.RawMessage) ([]Workout, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var raws []rawRecord
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, err
	}
	out := make([]Workout, 0, len(raws))
	for _, r := range raws {
		out = append(out, r.toWorkout())
	}
	return out, nil
}

func (r rawRecord) toWorkout() Workout {
	// training_id = trainingId if truthy, else id (Python `or`).
	tid := numInt(r.TrainingID)
	if tid == 0 {
		tid = numInt(r.ID)
	}
	// title = title if non-empty, else "Workout".
	title := "Workout"
	if r.Title != nil && *r.Title != "" {
		title = *r.Title
	}
	// type = courseTypeStr if non-empty, else courseCategoryName.
	wtype := ""
	switch {
	case r.CourseTypeStr != nil && *r.CourseTypeStr != "":
		wtype = *r.CourseTypeStr
	case r.CourseCategoryName != nil:
		wtype = *r.CourseCategoryName
	}
	vol := zeroFloat
	if r.TotalCapacity != nil {
		vol = *r.TotalCapacity
	}
	return Workout{
		TrainingID:  tid,
		Title:       title,
		WorkoutType: wtype,
		StartTS:     numInt(r.StartTimestamp),
		EndTS:       numInt(r.EndTimestamp),
		DurationSec: numInt(r.TrainingTime),
		Calories:    numInt(r.Calorie),
		Volume:      vol,
	}
}

// Date returns the session's calendar date as "YYYY-MM-DD", or nil when there is
// no usable timestamp. The timestamp comes from start || end; values above 1e12
// are milliseconds (GOAL.md §12 edge). Conversion uses local time to match
// Python's datetime.fromtimestamp.
func (w Workout) Date() *string {
	ts := w.StartTS
	if ts == 0 {
		ts = w.EndTS
	}
	if ts == 0 {
		return nil
	}
	sec := ts
	if ts > 1_000_000_000_000 { // > 1e12 → milliseconds
		sec = ts / 1000
	}
	s := time.Unix(sec, 0).Format("2006-01-02")
	return &s
}

// Summary is the workouts --json row (GOAL.md §9.2). Field order matches Python.
type Summary struct {
	TrainingID   int64       `json:"training_id"`
	Title        string      `json:"title"`
	Date         *string     `json:"date"`
	DurationSecs int64       `json:"duration_secs"`
	Calories     int64       `json:"calories"`
	Volume       json.Number `json:"volume"`
	Type         string      `json:"type"`
}

// Summary builds the workouts --json row for this workout.
func (w Workout) Summary() Summary {
	return Summary{
		TrainingID:   w.TrainingID,
		Title:        w.Title,
		Date:         w.Date(),
		DurationSecs: w.DurationSec,
		Calories:     w.Calories,
		Volume:       w.Volume,
		Type:         w.WorkoutType,
	}
}

// --- session detail: faithful, lossless passthrough (GOAL.md §9.3) ---

// SessionDetail is the `session <id> --json` document. A session spans two
// Speediance endpoints; this carries each endpoint's *verbatim* data payload
// (the transport envelope unwrapped, the payload itself untouched) so every
// field — modeled or not, present today or added by a future app update — flows
// straight through. The CLI does not parse, rename, reshape, summarize, or
// derive anything here; consumers decide shape, derivation, and storage.
//
//	info   = GET /app/trainingInfo/cttTrainingInfo/<id>        (session-level, incl. completionRate)
//	detail = GET /app/trainingInfo/cttTrainingInfoDetail/<id>  (per-exercise, per-rep arrays)
//
// A payload Speediance does not return is emitted as JSON null — never
// fabricated, never back-filled (e.g. a "Free Lift" freestyle session yields a
// null detail). training_id echoes the requested id — the only added field, a
// stable correlation key for the caller, derived from nothing in the payloads.
type SessionDetail struct {
	TrainingID int64           `json:"training_id"`
	Info       json.RawMessage `json:"info"`
	Detail     json.RawMessage `json:"detail"`
}

// numInt parses an optional json.Number to int64, treating absent/invalid as 0.
func numInt(n *json.Number) int64 {
	if n == nil {
		return 0
	}
	if i, err := n.Int64(); err == nil {
		return i
	}
	if f, err := n.Float64(); err == nil {
		return int64(f)
	}
	return 0
}
