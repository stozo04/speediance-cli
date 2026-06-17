// Package workout models Speediance session data and the exact JSON shapes the
// CLI emits. Field names, types, and order in the output structs are part of the
// frozen --json contract (GOAL.md §2, §9.2, §9.3) and mirror the Python
// dataclasses in models.py.
//
// Numbers whose int-vs-float form must round-trip exactly (volume, weight,
// max_hr, completion_rate) are carried as json.Number so we re-emit the server's
// value verbatim, exactly as Python's json round-trip does.
package workout

import (
	"encoding/json"
	"time"
)

// zeroFloat is the default for passthrough number fields, matching Python's 0.0
// default (which serializes as "0.0", not "0").
const zeroFloat = json.Number("0.0")

// Workout is a completed session. Only the subset the CLI needs is modeled.
type Workout struct {
	TrainingID  int64
	Title       string
	WorkoutType string
	StartTS     int64
	EndTS       int64
	DurationSec int64
	Calories    int64
	Volume      json.Number // raw totalCapacity, for verbatim re-emit
	Completion  json.Number // completionRate, populated by detail fetch
	Sets        []SetData
}

// SetData is one performed set within an exercise.
type SetData struct {
	ExerciseName string
	SetIndex     int
	FinishedReps int
	TargetReps   int
	Weight       json.Number // rep weight, falling back to the exercise maxWeight
	Capacity     json.Number
	MaxHeartRate json.Number
	LeftRight    int // 0=both, 1=left, 2=right
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

// --- session detail (GOAL.md §9.3) ---

// SetOut is one set in the session --json output.
type SetOut struct {
	Set       int         `json:"set"`
	Reps      int         `json:"reps"`
	TargetRep int         `json:"target_reps"`
	Weight    json.Number `json:"weight"`
	MaxHR     json.Number `json:"max_hr"`
	LeftRight int         `json:"left_right"`
}

// ExerciseOut groups sets under an exercise name.
type ExerciseOut struct {
	Name string   `json:"name"`
	Sets []SetOut `json:"sets"`
}

// Session is the full session --json document.
type Session struct {
	TrainingID     int64         `json:"training_id"`
	CompletionRate json.Number   `json:"completion_rate"`
	Exercises      []ExerciseOut `json:"exercises"`
}

// rawExercise mirrors a cttTrainingInfoDetail entry.
type rawExercise struct {
	ActionLibraryName string      `json:"actionLibraryName"`
	MaxWeight         json.Number `json:"maxWeight"`
	FinishedReps      []rawRep    `json:"finishedReps"`
}

type rawRep struct {
	FinishedCount int          `json:"finishedCount"`
	TargetCount   int          `json:"targetCount"`
	Weight        *json.Number `json:"weight"`
	Capacity      *json.Number `json:"capacity"`
	MaxHeartRate  *json.Number `json:"maxHeartRate"`
	LeftRight     int          `json:"leftRight"`
}

// SetCompletionRate decodes the cttTrainingInfo payload's completionRate,
// defaulting to "0.0" (GOAL.md §9.3). It mirrors `data.get("completionRate", 0.0)`.
func (w *Workout) SetCompletionRate(infoData json.RawMessage) {
	w.Completion = zeroFloat
	if len(infoData) == 0 {
		return
	}
	var d struct {
		CompletionRate *json.Number `json:"completionRate"`
	}
	if err := json.Unmarshal(infoData, &d); err == nil && d.CompletionRate != nil {
		w.Completion = *d.CompletionRate
	}
}

// AddDetailSets decodes a cttTrainingInfoDetail list and appends its sets,
// grouping by exercise. Weight falls back to the exercise maxWeight when a rep
// omits it (Python `rep.get("weight", max_weight)`).
func (w *Workout) AddDetailSets(detailData json.RawMessage) error {
	if len(detailData) == 0 {
		return nil
	}
	var exs []rawExercise
	if err := json.Unmarshal(detailData, &exs); err != nil {
		return err
	}
	for _, ex := range exs {
		maxWeight := ex.MaxWeight
		if maxWeight == "" {
			maxWeight = zeroFloat
		}
		for i, rep := range ex.FinishedReps {
			weight := maxWeight
			if rep.Weight != nil {
				weight = *rep.Weight
			}
			w.Sets = append(w.Sets, SetData{
				ExerciseName: ex.ActionLibraryName,
				SetIndex:     i + 1,
				FinishedReps: rep.FinishedCount,
				TargetReps:   rep.TargetCount,
				Weight:       weight,
				Capacity:     numOrZero(rep.Capacity),
				MaxHeartRate: numOrZero(rep.MaxHeartRate),
				LeftRight:    rep.LeftRight,
			})
		}
	}
	return nil
}

// GroupedExercises returns the workout's sets grouped by exercise name in
// first-seen order — the equivalent of the Python Workout.exercises() dict. Used
// to build the session output.
func (w Workout) GroupedExercises() (order []string, byName map[string][]SetData) {
	byName = map[string][]SetData{}
	for _, s := range w.Sets {
		if _, ok := byName[s.ExerciseName]; !ok {
			order = append(order, s.ExerciseName)
		}
		byName[s.ExerciseName] = append(byName[s.ExerciseName], s)
	}
	return order, byName
}

// SessionOutput builds the session --json document, grouping sets by exercise
// name in first-seen order (GOAL.md §9.3).
func (w Workout) SessionOutput() Session {
	comp := w.Completion
	if comp == "" {
		comp = zeroFloat
	}
	order, byName := w.GroupedExercises()
	exercises := make([]ExerciseOut, 0, len(order))
	for _, name := range order {
		sets := make([]SetOut, 0, len(byName[name]))
		for _, s := range byName[name] {
			sets = append(sets, SetOut{
				Set:       s.SetIndex,
				Reps:      s.FinishedReps,
				TargetRep: s.TargetReps,
				Weight:    s.Weight,
				MaxHR:     s.MaxHeartRate,
				LeftRight: s.LeftRight,
			})
		}
		exercises = append(exercises, ExerciseOut{Name: name, Sets: sets})
	}
	return Session{
		TrainingID:     w.TrainingID,
		CompletionRate: comp,
		Exercises:      exercises,
	}
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

// numOrZero returns the pointed-to number, or "0.0" when absent.
func numOrZero(n *json.Number) json.Number {
	if n == nil {
		return zeroFloat
	}
	return *n
}
