package sheet

import (
	"regexp"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// Fuzzy exercise matching, ported from speediance/sheet.py's _norm/_match.
//
// GOAL.md §11 allows "good enough" matching here, but exact parity is both
// achievable and safer: difflib.SequenceMatcher.Ratio() from pmezard/go-difflib
// is a faithful port of Python's difflib (and is already in our module graph via
// testify), so we reproduce the Python blend exactly —
//
//	score = 0.5*SequenceMatcher.ratio(full_a, full_b) + 0.5*jaccard(core_a, core_b)
//	match when score >= 0.45
//
// — rather than approximating it with a different algorithm and re-tuning.

const matchThreshold = 0.45

var (
	parenRe   = regexp.MustCompile(`\(.*?\)`)    // parentheticals, e.g. (Gym Monster)
	nonAlnum  = regexp.MustCompile(`[^a-z0-9 ]`) // keep letters, digits, spaces
	synonyms  = map[string]string{"rdl": "romanian deadlift", "ohp": "overhead press"}
	stopwords = map[string]bool{
		"the": true, "a": true, "machine": true,
		"cable": true, "db": true, "press": true,
	}
)

// norm normalizes an exercise name to (full, core): `full` is the space-joined
// sorted unique token string used for the character-level ratio; `core` is the
// set of non-stopword tokens used for Jaccard overlap. A synonym expands to a
// single compound token (e.g. "rdl" -> "romanian deadlift"), exactly as the
// Python list comprehension does.
func norm(name string) (full string, core map[string]bool) {
	s := strings.ToLower(name)
	s = parenRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "dumbbell", "db")
	s = strings.ReplaceAll(s, "barbell", "bb")
	s = nonAlnum.ReplaceAllString(s, " ")

	var toks []string
	for _, t := range strings.Fields(s) {
		if syn, ok := synonyms[t]; ok {
			toks = append(toks, syn)
		} else {
			toks = append(toks, t)
		}
	}

	// full = " ".join(sorted(set(toks)))
	uniq := make(map[string]bool, len(toks))
	for _, t := range toks {
		uniq[t] = true
	}
	keys := make([]string, 0, len(uniq))
	for k := range uniq {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	full = strings.Join(keys, " ")

	// core = non-stopword tokens, or all tokens if every token is a stopword.
	core = make(map[string]bool)
	for _, t := range toks {
		if !stopwords[t] {
			core[t] = true
		}
	}
	if len(core) == 0 {
		for _, t := range toks {
			core[t] = true
		}
	}
	return full, core
}

// match returns the best-matching workout name for a sheet exercise, or ok=false
// when no candidate clears the threshold. Each workout name may be matched only
// once; the caller enforces that via the candidates slice it passes.
func match(sheetExercise string, workoutNames []string) (string, bool) {
	sFull, sCore := norm(sheetExercise)
	sFullRunes := runeStrings(sFull)

	best := ""
	bestScore := 0.0
	for _, wn := range workoutNames {
		wFull, wCore := norm(wn)
		m := difflib.NewMatcher(sFullRunes, runeStrings(wFull))
		ratio := m.Ratio()
		overlap := jaccard(sCore, wCore)
		score := 0.5*ratio + 0.5*overlap
		if score > bestScore {
			best, bestScore = wn, score
		}
	}
	if bestScore >= matchThreshold {
		return best, true
	}
	return "", false
}

// jaccard returns |a ∩ b| / |a ∪ b| (with a 1 floor on the denominator), matching
// Python's len(s_core & w_core) / max(1, len(s_core | w_core)).
func jaccard(a, b map[string]bool) float64 {
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a)
	for k := range b {
		if !a[k] {
			union++
		}
	}
	if union < 1 {
		union = 1
	}
	return float64(inter) / float64(union)
}

// runeStrings splits a string into one-rune strings so go-difflib (which diffs
// []string) computes a character-level ratio, matching Python's
// SequenceMatcher(None, str_a, str_b).
func runeStrings(s string) []string {
	rs := []rune(s)
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}
