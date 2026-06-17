// Package template builds the customTrainingTemplate POST body and fetches the
// exercise library. The produced JSON is the API wire body and the weight math
// sets real loads on a physical machine, so field names, CSV encodings, and
// float formatting are frozen to match the Python tool byte-for-byte
// (GOAL.md §10). Ports speediance/templates.py.
package template

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/stozo04/speediance-cli/internal/api"
)

// kgToAPI converts plan kilograms to the API's internal weight unit, per the
// upstream reverse-engineering (GOAL.md §10). Do not change without a verified
// recalibration — it determines real machine loads.
const kgToAPI = 2.2

// Exercise is one library entry. Field order (id, name, muscle, tab) matches the
// library --json contract (GOAL.md §9.4).
type Exercise struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Muscle string `json:"muscle"`
	Tab    string `json:"tab"`
}

// --- plan input (GOAL.md §9.5) ---

// Plan is a user-authored training plan.
type Plan struct {
	Name      string         `json:"name"`
	Exercises []PlanExercise `json:"exercises"`
}

// PlanExercise is one exercise with its sets. ID is a json.Number to carry large
// catalog ids (e.g. 372783897509889) without float rounding.
type PlanExercise struct {
	ID    json.Number `json:"id"`
	Title string      `json:"title"`
	Sets  []PlanSet   `json:"sets"`
}

// PlanSet is one set. Mode/Rest are pointers so absent keys take their non-zero
// defaults (mode 1, rest 60) exactly as the Python `s.get(key, default)` does.
type PlanSet struct {
	Reps   int     `json:"reps"`
	Weight float64 `json:"weight"`
	Mode   *int    `json:"mode"`
	Rest   *int    `json:"rest"`
}

// LoadPlan reads and validates a plan file. It must contain "name" and
// "exercises" (GOAL.md §9.5).
func LoadPlan(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan %s: %w", path, err)
	}
	// Check required keys by presence, matching Python's `"name" not in plan`.
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("parse plan %s: %w", path, err)
	}
	if _, ok := keys["name"]; !ok {
		return nil, fmt.Errorf("plan JSON must have 'name' and 'exercises'")
	}
	if _, ok := keys["exercises"]; !ok {
		return nil, fmt.Errorf("plan JSON must have 'name' and 'exercises'")
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parse plan %s: %w", path, err)
	}
	return &plan, nil
}

// --- payload (frozen field names + order — GOAL.md §10) ---

// Action is one entry in actionLibraryList. Field order is significant: it must
// match the Python dict insertion order so json output is byte-identical.
type Action struct {
	GroupID                int64   `json:"groupId"`
	ActionLibraryID        int64   `json:"actionLibraryId"`
	TemplatePresetID       int     `json:"templatePresetId"`
	SetsAndReps            string  `json:"setsAndReps"`
	BreakTime              string  `json:"breakTime"`
	BreakTime2             string  `json:"breakTime2"`
	SportMode              string  `json:"sportMode"`
	LeftRight              string  `json:"leftRight"`
	SelectCompletionMethod string  `json:"selectCompletionMethod"`
	CompletionMethod       string  `json:"completionMethod"`
	CountType              string  `json:"countType"`
	Weights                string  `json:"weights"`
	Counterweight2         string  `json:"counterweight2"`
	Level                  string  `json:"level"`
	Capacity               pyFloat `json:"capacity"`
}

// Payload is the full customTrainingTemplate POST body.
type Payload struct {
	Name              string   `json:"name"`
	ActionLibraryList []Action `json:"actionLibraryList"`
	TotalCapacity     pyFloat  `json:"totalCapacity"`
	DeviceType        int      `json:"deviceType"`
	BgColor           int      `json:"bgColor"`
}

// BuildPayload constructs the POST body from a plan's exercises (GOAL.md §10).
// It resolves variant ids and unilateral flags via the API, then encodes each
// exercise's sets into the frozen CSV strings.
func BuildPayload(ctx context.Context, client *api.Client, name string, exercises []PlanExercise, deviceType int) (*Payload, error) {
	groupIDs, err := uniqueGroupIDs(exercises)
	if err != nil {
		return nil, err
	}
	idMap, err := resolveVariantIDs(ctx, client, groupIDs)
	if err != nil {
		return nil, err
	}
	unilateral := make(map[int64]bool, len(groupIDs))
	for _, g := range groupIDs {
		u, err := isUnilateral(ctx, client, g)
		if err != nil {
			return nil, err
		}
		unilateral[g] = u
	}

	actions := make([]Action, 0, len(exercises))
	var totalCapacity float64

	for _, ex := range exercises {
		groupID, err := numToInt64(ex.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid exercise id %q: %w", ex.ID.String(), err)
		}
		variantID, ok := idMap[groupID]
		if !ok {
			title := ex.Title
			if title == "" {
				title = "?"
			}
			return nil, fmt.Errorf("could not resolve exercise id %d (%s) — "+
				"run `speediance-cli library` and check the id", groupID, title)
		}
		isUni := unilateral[groupID]

		var reps, breaks, modes, lr, level, completion, completionMethod, countType, weights []string
		var setCapacity float64

		for i, s := range ex.Sets {
			mode := 1
			if s.Mode != nil {
				mode = *s.Mode
			}
			rest := 60
			if s.Rest != nil {
				rest = *s.Rest
			}

			reps = append(reps, strconv.Itoa(s.Reps))
			breaks = append(breaks, strconv.Itoa(rest))
			modes = append(modes, strconv.Itoa(mode))
			level = append(level, "0")
			if isUni {
				if i%2 == 0 {
					lr = append(lr, "1")
				} else {
					lr = append(lr, "2")
				}
			} else {
				lr = append(lr, "0")
			}
			completionMethod = append(completionMethod, "1") // rep-based
			countType = append(countType, "1")
			completion = append(completion, "1")

			apiWeight := s.Weight * kgToAPI
			weights = append(weights, strconv.FormatFloat(apiWeight, 'f', 1, 64))
			setCapacity += float64(s.Reps) * apiWeight
		}

		totalCapacity += setCapacity
		actions = append(actions, Action{
			GroupID:                groupID,
			ActionLibraryID:        variantID,
			TemplatePresetID:       -1,
			SetsAndReps:            strings.Join(reps, ","),
			BreakTime:              strings.Join(breaks, ","),
			BreakTime2:             strings.Join(breaks, ","),
			SportMode:              strings.Join(modes, ","),
			LeftRight:              strings.Join(lr, ","),
			SelectCompletionMethod: strings.Join(completion, ","),
			CompletionMethod:       strings.Join(completionMethod, ","),
			CountType:              strings.Join(countType, ","),
			Weights:                strings.Join(weights, ","),
			Counterweight2:         "",
			Level:                  strings.Join(level, ","),
			Capacity:               pyFloat(setCapacity),
		})
	}

	return &Payload{
		Name:              name,
		ActionLibraryList: actions,
		TotalCapacity:     pyFloat(totalCapacity),
		DeviceType:        deviceType,
		BgColor:           0,
	}, nil
}

// CreateTemplate builds and POSTs the payload, returning the response data
// (GOAL.md §9.5).
func CreateTemplate(ctx context.Context, client *api.Client, name string, exercises []PlanExercise, deviceType int) (json.RawMessage, error) {
	payload, err := BuildPayload(ctx, client, name, exercises, deviceType)
	if err != nil {
		return nil, err
	}
	env, err := client.PostJSON(ctx, "/app/v2/customTrainingTemplate", payload)
	if err != nil {
		return nil, err
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("create template failed: %s (code %d)", env.Message, env.Code)
	}
	return env.Data, nil
}

// uniqueGroupIDs returns the plan's distinct exercise ids in first-seen order.
func uniqueGroupIDs(exercises []PlanExercise) ([]int64, error) {
	seen := make(map[int64]bool)
	var ids []int64
	for _, ex := range exercises {
		id, err := numToInt64(ex.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid exercise id %q: %w", ex.ID.String(), err)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// resolveVariantIDs maps each groupId to its first variant's actionLibraryId via
// GET /app/actionLibraryGroup/list?ids=...&ids=... (GOAL.md §10).
func resolveVariantIDs(ctx context.Context, client *api.Client, groupIDs []int64) (map[int64]int64, error) {
	parts := make([]string, len(groupIDs))
	for i, g := range groupIDs {
		parts[i] = "ids=" + strconv.FormatInt(g, 10)
	}
	env, err := client.GetJSON(ctx, "/app/actionLibraryGroup/list?"+strings.Join(parts, "&"))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID                json.Number `json:"id"`
		ActionLibraryList []struct {
			ID json.Number `json:"id"`
		} `json:"actionLibraryList"`
	}
	_ = json.Unmarshal(env.Data, &rows)

	idMap := make(map[int64]int64, len(rows))
	for _, d := range rows {
		if len(d.ActionLibraryList) == 0 {
			continue
		}
		gid, err := numToInt64(d.ID)
		if err != nil {
			continue
		}
		vid, err := numToInt64(d.ActionLibraryList[0].ID)
		if err != nil {
			continue
		}
		idMap[gid] = vid
	}
	return idMap, nil
}

// isUnilateral reports whether a group is left/right (unilateral) via
// GET /app/actionLibraryGroup/<id>?isDisplay=1 → data.isLeftRight == 1.
func isUnilateral(ctx context.Context, client *api.Client, groupID int64) (bool, error) {
	env, err := client.GetJSON(ctx, "/app/actionLibraryGroup/"+strconv.FormatInt(groupID, 10)+"?isDisplay=1")
	if err != nil {
		return false, err
	}
	var d struct {
		IsLeftRight int `json:"isLeftRight"`
	}
	_ = json.Unmarshal(env.Data, &d)
	return d.IsLeftRight == 1, nil
}

// numToInt64 parses a json.Number to int64, accepting integer-valued floats.
func numToInt64(n json.Number) (int64, error) {
	if i, err := n.Int64(); err == nil {
		return i, nil
	}
	f, err := n.Float64()
	if err != nil {
		return 0, err
	}
	return int64(f), nil
}
