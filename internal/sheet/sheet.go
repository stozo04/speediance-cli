// Package sheet writes a completed Speediance session into a Markdown
// WEEKS/Week-XX.md checklist. This is the optional `sync` integration
// (GOAL.md §11); its structure (checkbox glyphs, the "Logged from Speediance"
// block, the date token format) is preserved from speediance/sheet.py. Fuzzy
// matching lives in match.go.
package sheet

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stozo04/speediance-cli/internal/workout"
)

const (
	checkEmpty = "☐"
	checkDone  = "☑"
)

// MatchPair records a sheet exercise cell that was matched to a workout name.
type MatchPair struct {
	Exercise string
	Name     string
}

// Result summarizes a write_session run (GOAL.md §11).
type Result struct {
	Sheet         string
	Matched       []MatchPair
	Unmatched     []string
	ExerciseCount int
}

// DateToken renders a date as "M/D" with no leading zeros (e.g. 6/15), matching
// the sheet headers.
func DateToken(d time.Time) string {
	return fmt.Sprintf("%d/%d", int(d.Month()), d.Day())
}

// FindWeekSheet picks the Week-*.md file covering target_date: the first whose a
// "## ... M/D" header contains the date token, else the highest-numbered file.
// Returns "" when the directory has no such sheets (GOAL.md §11).
func FindWeekSheet(weeksDir string, target time.Time) (string, error) {
	files, err := filepath.Glob(filepath.Join(weeksDir, "Week-*.md"))
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	sort.Strings(files)
	tok := DateToken(target)
	headerRe := regexp.MustCompile(`(?m)^##.*\b` + regexp.QuoteMeta(tok) + `\b`)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if headerRe.Match(data) {
			return path, nil
		}
	}
	return files[len(files)-1], nil // fallback: highest-numbered week.
}

// WriteSession writes one completed session into the sheet at path, ticking
// matched exercise rows, filling weight cells, flipping the at-a-glance checkbox,
// and appending a full "Logged from Speediance" block (GOAL.md §11).
func WriteSession(path string, w *workout.Workout, target time.Time, unit string) (*Result, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Split on "\n" exactly like Python so a re-join round-trips byte-for-byte
	// (any trailing "\r" stays attached to its line).
	lines := strings.Split(string(raw), "\n")

	tok := DateToken(target)
	order, byName := w.GroupedExercises()
	workoutNames := order

	var matched []MatchPair
	used := map[string]bool{}

	// Locate the workout section for this date (e.g. "## PUSH - Mon 6/15 ...").
	secStart := -1
	for i, ln := range lines {
		if strings.HasPrefix(ln, "##") && strings.Contains(ln, tok) && !strings.Contains(ln, "Notes") {
			secStart = i
			break
		}
	}

	// 1) Fill weight cells + tick boxes inside that section's exercise table.
	if secStart >= 0 {
		for i := secStart + 1; i < len(lines); i++ {
			ln := lines[i]
			if strings.HasPrefix(ln, "##") {
				break
			}
			if !strings.HasPrefix(strings.TrimSpace(ln), "|") {
				continue
			}
			cells := splitCells(ln)
			if len(cells) != 4 || (cells[0] != checkEmpty && cells[0] != checkDone) {
				continue
			}
			exercise := cells[1]
			// Match against ALL workout names, then reject if already used —
			// exactly as Python does (it does not pre-filter, so a row whose
			// best match is taken stays unmatched rather than falling back).
			name, ok := match(exercise, workoutNames)
			if ok && !used[name] {
				cells[0] = checkDone
				cells[3] = setsStr(byName[name], unit)
				lines[i] = "| " + strings.Join(cells, " | ") + " |"
				matched = append(matched, MatchPair{Exercise: exercise, Name: name})
				used[name] = true
			}
		}
	}

	// 2) Flip the at-a-glance row for this date (its last cell is the checkbox).
	for i, ln := range lines {
		if strings.Contains(ln, tok) && strings.HasPrefix(strings.TrimSpace(ln), "|") {
			cells := splitCells(ln)
			if len(cells) > 0 && cells[len(cells)-1] == checkEmpty {
				cells[len(cells)-1] = checkDone
				lines[i] = "| " + strings.Join(cells, " | ") + " |"
				break
			}
		}
	}

	// 3) Build the "Logged from Speediance" block (captures everything).
	dur := "-"
	if w.DurationSec != 0 {
		dur = fmt.Sprintf("%d min", w.DurationSec/60)
	}
	cal := "-"
	if w.Calories != 0 {
		cal = fmt.Sprintf("%d kcal", w.Calories)
	}
	comp := ""
	if cr := numFloat(w.Completion); cr != 0 {
		comp = fmt.Sprintf("%s%% complete", strconv.FormatFloat(cr, 'f', 0, 64))
	}
	header := fmt.Sprintf("  - **Logged from Speediance - %s** (%s) - %s - %s", w.Title, tok, dur, cal)
	if comp != "" {
		header += " - " + comp
	}
	block := []string{header}
	for _, name := range order {
		block = append(block, fmt.Sprintf("    - %s: %s", name, setsStr(byName[name], unit)))
	}
	if len(order) == 0 {
		block = append(block, "    - (no per-set detail returned)")
	}

	// Insert under the matching Notes bullet, else append a Logged Sessions
	// section.
	noteRe := regexp.MustCompile(`^\s*-\s.*` + regexp.QuoteMeta(tok))
	noteIdx := -1
	for i, ln := range lines {
		if noteRe.MatchString(ln) {
			noteIdx = i
			break
		}
	}
	if noteIdx >= 0 {
		out := make([]string, 0, len(lines)+len(block))
		out = append(out, lines[:noteIdx+1]...)
		out = append(out, block...)
		out = append(out, lines[noteIdx+1:]...)
		lines = out
	} else {
		lines = append(lines, "", "## Logged Sessions", "")
		lines = append(lines, block...)
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return nil, err
	}

	var unmatched []string
	for _, n := range workoutNames {
		if !used[n] {
			unmatched = append(unmatched, n)
		}
	}
	return &Result{
		Sheet:         path,
		Matched:       matched,
		Unmatched:     unmatched,
		ExerciseCount: len(workoutNames),
	}, nil
}

// splitCells parses a Markdown table row into trimmed cells, mirroring
// [c.strip() for c in ln.strip().strip("|").split("|")].
func splitCells(ln string) []string {
	s := strings.Trim(strings.TrimSpace(ln), "|")
	parts := strings.Split(s, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

// setsStr renders a compact "<w><unit>×<reps>, ..." string for a list of sets.
func setsStr(sets []workout.SetData, unit string) string {
	parts := make([]string, 0, len(sets))
	for _, s := range sets {
		parts = append(parts, fmtWeight(s.Weight)+unit+"×"+strconv.Itoa(s.FinishedReps))
	}
	return strings.Join(parts, ", ")
}

// fmtWeight renders a weight like Python's _fmt_weight: an integer-valued weight
// drops its decimals ("45"), otherwise uses %g ("22.5").
func fmtWeight(n json.Number) string {
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil {
		return string(n)
	}
	if f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func numFloat(n json.Number) float64 {
	f, _ := strconv.ParseFloat(string(n), 64)
	return f
}
