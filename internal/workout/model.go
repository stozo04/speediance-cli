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
	"strconv"
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
	// exMeta holds per-exercise telemetry (scores, max weight) keyed by exercise
	// name, surfaced only under --telemetry. Unexported: Workout is never JSON-
	// encoded directly (SessionOutput/Summary build the wire types).
	exMeta map[string]*exerciseMeta
}

// SetData is one performed set within an exercise.
type SetData struct {
	ExerciseName string
	SetIndex     int
	FinishedReps int
	TargetReps   int
	// Weight is the per-set weight scalar. It is the API's per-rep weight when
	// present ("actual"), otherwise the mean of the real per-rep telemetry
	// ("derived_avg"), and never the *planned* exercise maxWeight (issue #23: the
	// old maxWeight fallback fabricated flat per-set weights). WeightSource names
	// which of these it is so a consumer can't mistake a derived value for a
	// logged one.
	Weight       json.Number
	WeightSource string // "actual" | "derived_avg" | "unavailable"
	Capacity     json.Number
	MaxHeartRate json.Number
	LeftRight    int // 0=both, 1=left, 2=right
	// AvgPerHandle is mean(weights[]) when per-rep telemetry exists, emitted only
	// under --telemetry. Reps is the per-rep telemetry, likewise --telemetry-only.
	AvgPerHandle *json.Number
	Reps         []RepDetail
}

// exerciseMeta carries the per-exercise summary the API returns alongside the
// per-rep detail: form scores and the heaviest per-handle weight reached. Shown
// only under --telemetry.
type exerciseMeta struct {
	scores         *Scores
	maxWeight      *json.Number
	maxWeightCount *int
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

// SetOut is one set in the session --json output. weight_source and capacity are
// always emitted (issue #23); the weight_avg_per_handle and reps_detail fields are
// populated only under --telemetry and omitted otherwise.
type SetOut struct {
	Set          int         `json:"set"`
	Reps         int         `json:"reps"`
	TargetRep    int         `json:"target_reps"`
	Weight       json.Number `json:"weight"`
	WeightSource string      `json:"weight_source"`
	Capacity     json.Number `json:"capacity"`
	MaxHR        json.Number `json:"max_hr"`
	LeftRight    int         `json:"left_right"`
	// --telemetry only:
	WeightAvgPerHandle *json.Number `json:"weight_avg_per_handle,omitempty"`
	RepsDetail         []RepDetail  `json:"reps_detail,omitempty"`
}

// RepDetail is the per-rep, per-side telemetry the hardware captures, emitted
// under --telemetry. Right-side fields are omitted for single-attachment moves
// (e.g. a rope face pull) that populate only the left arrays.
type RepDetail struct {
	Rep              int          `json:"rep"`
	Weight           *json.Number `json:"weight,omitempty"`
	LeftWatts        *json.Number `json:"left_watts,omitempty"`
	RightWatts       *json.Number `json:"right_watts,omitempty"`
	LeftAmp          *json.Number `json:"left_amp,omitempty"`
	RightAmp         *json.Number `json:"right_amp,omitempty"`
	LeftRopeSpeed    *json.Number `json:"left_rope_speed,omitempty"`
	RightRopeSpeed   *json.Number `json:"right_rope_speed,omitempty"`
	LeftFinishedTime *json.Number `json:"left_finished_time,omitempty"`
	LeftBreakTime    *json.Number `json:"left_break_time,omitempty"`
	LeftTimestamp    *json.Number `json:"left_timestamp,omitempty"`
}

// Scores is the per-exercise form scoring (each /5 except total and rating),
// emitted under --telemetry.
type Scores struct {
	Total            int `json:"total"`
	Completion       int `json:"completion"`
	ForceControl     int `json:"force_control"`
	BilateralBalance int `json:"bilateral_balance"`
	AmplitudeStable  int `json:"amplitude_stable"`
	Rating           int `json:"rating"`
}

// ExerciseOut groups sets under an exercise name. scores, max_weight, and
// max_weight_count are populated only under --telemetry and omitted otherwise.
type ExerciseOut struct {
	Name           string       `json:"name"`
	Scores         *Scores      `json:"scores,omitempty"`
	MaxWeight      *json.Number `json:"max_weight,omitempty"`
	MaxWeightCount *int         `json:"max_weight_count,omitempty"`
	Sets           []SetOut     `json:"sets"`
}

// Session is the full session --json document.
type Session struct {
	TrainingID     int64         `json:"training_id"`
	CompletionRate json.Number   `json:"completion_rate"`
	Exercises      []ExerciseOut `json:"exercises"`
}

// rawExercise mirrors a cttTrainingInfoDetail entry. The score fields and
// maxWeightCount are the per-exercise summary surfaced under --telemetry.
type rawExercise struct {
	ActionLibraryName     string      `json:"actionLibraryName"`
	MaxWeight             json.Number `json:"maxWeight"`
	MaxWeightCount        *int        `json:"maxWeightCount"`
	Score                 *int        `json:"score"`
	CompletionScore       *int        `json:"completionScore"`
	ForceControlScore     *int        `json:"forceControlScore"`
	BilateralBalanceScore *int        `json:"bilateralBalanceScore"`
	AmplitudeStableScore  *int        `json:"amplitudeStableScore"`
	ActionRating          *int        `json:"actionRating"`
	FinishedReps          []rawRep    `json:"finishedReps"`
}

type rawRep struct {
	FinishedCount int                 `json:"finishedCount"`
	TargetCount   int                 `json:"targetCount"`
	Weight        *json.Number        `json:"weight"`
	Capacity      *json.Number        `json:"capacity"`
	MaxHeartRate  *json.Number        `json:"maxHeartRate"`
	LeftRight     int                 `json:"leftRight"`
	Detail        *trainingInfoDetail `json:"trainingInfoDetail"`
}

// trainingInfoDetail is the nested per-rep telemetry block on each finished set.
// Every array here is per-rep (length == reps). Deliberately NOT modeled:
// leftMinRopeLengths/rightMinRopeLengths are high-frequency position traces (many
// samples per rep), not per-rep, so treating them as per-rep would be wrong.
type trainingInfoDetail struct {
	Weights           []json.Number `json:"weights"`
	LeftWatts         []json.Number `json:"leftWatts"`
	RightWatts        []json.Number `json:"rightWatts"`
	LeftAmplitudes    []json.Number `json:"leftAmplitudes"`
	RightAmplitudes   []json.Number `json:"rightAmplitudes"`
	LeftRopeSpeeds    []json.Number `json:"leftRopeSpeeds"`
	RightRopeSpeeds   []json.Number `json:"rightRopeSpeeds"`
	LeftFinishedTimes []json.Number `json:"leftFinishedTimes"`
	LeftBreakTimes    []json.Number `json:"leftBreakTimes"`
	LeftTimestamps    []json.Number `json:"leftTimestamps"`
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
// grouping by exercise.
//
// Per-set weight (issue #23): for a completed program session the API leaves each
// rep's top-level weight null. We must NOT fall back to the exercise maxWeight —
// that is the *planned* heaviest load and stamping it on every set fabricated
// flat, wrong per-set weights that looked logged. Instead we report the real
// performed load: the per-rep weight when present ("actual"), else the mean of
// the per-rep telemetry weights[] / capacity ("derived_avg"), else 0.0
// ("unavailable"). The weights[] array is already in per-attachment units, so its
// mean is the correct per-handle average for both dual-handle and single-rope
// moves with no fragile handle-count detection. WeightSource records which path
// produced the value so a downstream logger can never mistake a derived average
// for a logged weight.
func (w *Workout) AddDetailSets(detailData json.RawMessage) error {
	if len(detailData) == 0 {
		return nil
	}
	var exs []rawExercise
	if err := json.Unmarshal(detailData, &exs); err != nil {
		return err
	}
	for _, ex := range exs {
		w.recordExerciseMeta(ex)
		for i, rep := range ex.FinishedReps {
			weight, source, avg := deriveWeight(rep)
			w.Sets = append(w.Sets, SetData{
				ExerciseName: ex.ActionLibraryName,
				SetIndex:     i + 1,
				FinishedReps: rep.FinishedCount,
				TargetReps:   rep.TargetCount,
				Weight:       weight,
				WeightSource: source,
				Capacity:     numOrZero(rep.Capacity),
				MaxHeartRate: numOrZero(rep.MaxHeartRate),
				LeftRight:    rep.LeftRight,
				AvgPerHandle: avg,
				Reps:         buildRepDetails(rep.Detail, rep.FinishedCount),
			})
		}
	}
	return nil
}

// deriveWeight resolves the per-set weight scalar, its source marker, and the
// per-handle average (for --telemetry). See AddDetailSets for the contract.
func deriveWeight(rep rawRep) (weight json.Number, source string, avg *json.Number) {
	if rep.Detail != nil {
		if m, ok := meanNumber(rep.Detail.Weights); ok {
			avg = &m
		}
	}
	switch {
	case rep.Weight != nil:
		// The API gave a real per-rep/per-set weight: report it verbatim.
		return *rep.Weight, "actual", avg
	case avg != nil:
		// No per-rep weight, but real per-rep telemetry exists: its mean is the
		// true average load lifted (per attachment).
		return *avg, "derived_avg", avg
	case rep.Capacity != nil && rep.FinishedCount > 0:
		// No weights[] array; fall back to capacity / reps. This is a coarser
		// estimate (it can't distinguish dual-handle from single-rope), used only
		// when the per-rep telemetry is absent.
		if c, err := strconv.ParseFloat(string(*rep.Capacity), 64); err == nil {
			return formatWeight(c / float64(rep.FinishedCount)), "derived_avg", avg
		}
	}
	// Genuinely no load signal: emit 0.0 and say so, rather than inventing a value.
	return zeroFloat, "unavailable", avg
}

// recordExerciseMeta stashes the per-exercise summary (form scores, heaviest
// weight) for --telemetry output, keyed by exercise name (first occurrence wins,
// matching the first-seen grouping order).
func (w *Workout) recordExerciseMeta(ex rawExercise) {
	if w.exMeta == nil {
		w.exMeta = map[string]*exerciseMeta{}
	}
	if _, ok := w.exMeta[ex.ActionLibraryName]; ok {
		return
	}
	meta := &exerciseMeta{scores: buildScores(ex), maxWeightCount: ex.MaxWeightCount}
	if ex.MaxWeight != "" {
		mw := ex.MaxWeight
		meta.maxWeight = &mw
	}
	w.exMeta[ex.ActionLibraryName] = meta
}

// buildScores returns the per-exercise form scores, or nil when the response
// carried none (so --telemetry omits the block rather than emitting all-zeros).
func buildScores(ex rawExercise) *Scores {
	if ex.Score == nil && ex.CompletionScore == nil && ex.ForceControlScore == nil &&
		ex.BilateralBalanceScore == nil && ex.AmplitudeStableScore == nil && ex.ActionRating == nil {
		return nil
	}
	return &Scores{
		Total:            derefInt(ex.Score),
		Completion:       derefInt(ex.CompletionScore),
		ForceControl:     derefInt(ex.ForceControlScore),
		BilateralBalance: derefInt(ex.BilateralBalanceScore),
		AmplitudeStable:  derefInt(ex.AmplitudeStableScore),
		Rating:           derefInt(ex.ActionRating),
	}
}

// buildRepDetails projects the nested per-rep telemetry into n RepDetail rows
// (n == reps). A side's array that is absent or short simply yields an omitted
// field, which gracefully handles single-attachment moves (left-only arrays).
func buildRepDetails(d *trainingInfoDetail, n int) []RepDetail {
	if d == nil || n <= 0 {
		return nil
	}
	out := make([]RepDetail, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, RepDetail{
			Rep:              i + 1,
			Weight:           at(d.Weights, i),
			LeftWatts:        at(d.LeftWatts, i),
			RightWatts:       at(d.RightWatts, i),
			LeftAmp:          at(d.LeftAmplitudes, i),
			RightAmp:         at(d.RightAmplitudes, i),
			LeftRopeSpeed:    at(d.LeftRopeSpeeds, i),
			RightRopeSpeed:   at(d.RightRopeSpeeds, i),
			LeftFinishedTime: at(d.LeftFinishedTimes, i),
			LeftBreakTime:    at(d.LeftBreakTimes, i),
			LeftTimestamp:    at(d.LeftTimestamps, i),
		})
	}
	return out
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
// name in first-seen order (GOAL.md §9.3). When telemetry is true it additionally
// emits the per-rep arrays and per-exercise scores/max-weight (issue #23); when
// false those fields are omitted and the output is the lean per-set view.
func (w Workout) SessionOutput(telemetry bool) Session {
	comp := w.Completion
	if comp == "" {
		comp = zeroFloat
	}
	order, byName := w.GroupedExercises()
	exercises := make([]ExerciseOut, 0, len(order))
	for _, name := range order {
		ex := ExerciseOut{Name: name}
		if telemetry {
			if m := w.exMeta[name]; m != nil {
				ex.Scores = m.scores
				ex.MaxWeight = m.maxWeight
				ex.MaxWeightCount = m.maxWeightCount
			}
		}
		ex.Sets = make([]SetOut, 0, len(byName[name]))
		for _, s := range byName[name] {
			set := SetOut{
				Set:          s.SetIndex,
				Reps:         s.FinishedReps,
				TargetRep:    s.TargetReps,
				Weight:       s.Weight,
				WeightSource: s.WeightSource,
				Capacity:     s.Capacity,
				MaxHR:        s.MaxHeartRate,
				LeftRight:    s.LeftRight,
			}
			if telemetry {
				set.WeightAvgPerHandle = s.AvgPerHandle
				set.RepsDetail = s.Reps
			}
			ex.Sets = append(ex.Sets, set)
		}
		exercises = append(exercises, ex)
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

// derefInt returns the pointed-to int, or 0 when absent.
func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// at returns a copy-backed pointer to the i-th element of a per-rep array, or nil
// when i is out of range — so a missing/short side array yields an omitted field.
func at(a []json.Number, i int) *json.Number {
	if i < 0 || i >= len(a) {
		return nil
	}
	v := a[i]
	return &v
}

// meanNumber returns the mean of a numeric array (formatted like a weight), and
// false when the array is empty or holds no parseable numbers.
func meanNumber(a []json.Number) (json.Number, bool) {
	var sum float64
	cnt := 0
	for _, n := range a {
		if f, err := strconv.ParseFloat(string(n), 64); err == nil {
			sum += f
			cnt++
		}
	}
	if cnt == 0 {
		return "", false
	}
	return formatWeight(sum / float64(cnt)), true
}

// formatWeight renders a derived weight to one decimal place (e.g. 11.785 ->
// "11.8", 8 -> "8.0"). The non-round form is intentional: it signals a computed
// average rather than a dial setting, and the exact figures live in capacity and
// the --telemetry per-rep arrays.
func formatWeight(f float64) json.Number {
	return json.Number(strconv.FormatFloat(f, 'f', 1, 64))
}
