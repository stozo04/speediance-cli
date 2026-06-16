package sheet_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stozo04/speediance-cli/internal/sheet"
	"github.com/stozo04/speediance-cli/internal/workout"
)

const done = "☑"

// makeWorkout mirrors tests/test_sheet.py's make_workout: Speediance-style names
// that deliberately differ from the sheet wording, to exercise fuzzy matching.
func makeWorkout() *workout.Workout {
	w := &workout.Workout{
		TrainingID:  123,
		Title:       "Push Day",
		WorkoutType: "Strength",
		DurationSec: 2700,
		Calories:    310,
		Completion:  "100.0",
	}
	add := func(name string, weight string, reps []int) {
		for i, r := range reps {
			w.Sets = append(w.Sets, workout.SetData{
				ExerciseName: name,
				SetIndex:     i + 1,
				FinishedReps: r,
				TargetReps:   15,
				Weight:       json.Number(weight),
			})
		}
	}
	add("Chest Press", "45", []int{15, 15, 14})
	add("Incline Dumbbell Press", "30", []int{15, 13, 12})
	add("Seated Shoulder Press", "25", []int{15, 15, 15})
	add("Lateral Raise", "12", []int{18, 16, 15})
	add("Triceps Pushdown", "35", []int{15, 15, 13})
	add("Overhead Triceps Extension", "20", []int{15, 14})
	return w
}

func TestWriteSession(t *testing.T) {
	dir := t.TempDir()
	sheetPath := filepath.Join(dir, "Week-01.md")
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sample_week.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sheetPath, src, 0o600); err != nil {
		t.Fatal(err)
	}

	w := makeWorkout()
	target := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.Local)
	res, err := sheet.WriteSession(sheetPath, w, target, "lb")
	if err != nil {
		t.Fatalf("WriteSession: %v", err)
	}

	out, err := os.ReadFile(sheetPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)

	if !strings.Contains(text, "45lb") {
		t.Error("weight not written (expected 45lb)")
	}
	if n := strings.Count(text, done); n < 6 {
		t.Errorf("checkboxes not flipped: got %d done, want >= 6", n)
	}
	if !strings.Contains(text, "Logged from Speediance") {
		t.Error("notes block missing")
	}
	beforeNotes := strings.SplitN(text, "## Notes", 2)[0]
	if strings.Contains(beforeNotes, "______") {
		t.Error("some weight cells left blank before ## Notes")
	}
	// The at-a-glance row for the day must be checked.
	var glance string
	for _, ln := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "| **Mon 6/15**") {
			glance = ln
			break
		}
	}
	if glance == "" {
		t.Fatal("glance row not found")
	}
	cells := strings.Split(strings.Trim(strings.TrimSpace(glance), "|"), "|")
	last := strings.TrimSpace(cells[len(cells)-1])
	if last != done {
		t.Errorf("glance day not checked: last cell = %q", last)
	}

	// All six exercises should match (the sheet has six checkbox rows).
	if len(res.Matched) < 6 {
		t.Errorf("matched %d/6 exercises; unmatched=%v", len(res.Matched), res.Unmatched)
	}
}
