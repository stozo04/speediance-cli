package template

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/stozo04/speediance-cli/internal/api"
)

// muscleChunkSize is how many ids are enriched per detail call (GOAL.md §9.4).
const muscleChunkSize = 50

// FetchLibrary returns the exercise catalog for a device as [{id,name,muscle,tab}]
// in first-seen order (GOAL.md §9.4). It walks tabs (skipping custom ones),
// collects actions deduped by id, then enriches muscle names in id chunks.
func FetchLibrary(ctx context.Context, client *api.Client, deviceType int) ([]Exercise, error) {
	dt := strconv.Itoa(deviceType)

	tabsEnv, err := client.GetJSON(ctx, "/app/actionLibraryTab/list?deviceType="+dt)
	if err != nil {
		return nil, err
	}
	var tabs []struct {
		ID       json.Number     `json:"id"`
		Name     string          `json:"name"`
		IsCustom json.RawMessage `json:"isCustom"`
	}
	_ = json.Unmarshal(tabsEnv.Data, &tabs)

	// Ordered set of actions, deduped by id (first tab/group wins).
	var order []int64
	byID := make(map[int64]*Exercise)

	for _, t := range tabs {
		if truthy(t.IsCustom) {
			continue
		}
		grpEnv, err := client.GetJSON(ctx,
			"/app/actionLibraryGroup/trainingPartGroup?tabId="+t.ID.String()+"&deviceTypeList="+dt)
		if err != nil {
			return nil, err
		}
		var groups []struct {
			ActionLibraryGroupList []struct {
				ID    *json.Number `json:"id"`
				Title string       `json:"title"`
			} `json:"actionLibraryGroupList"`
		}
		_ = json.Unmarshal(grpEnv.Data, &groups)

		for _, mg := range groups {
			for _, a := range mg.ActionLibraryGroupList {
				if a.ID == nil {
					continue
				}
				aid, err := numToInt64(*a.ID)
				if err != nil {
					continue
				}
				if _, exists := byID[aid]; exists {
					continue
				}
				order = append(order, aid)
				byID[aid] = &Exercise{ID: aid, Name: a.Title, Muscle: "", Tab: t.Name}
			}
		}
	}

	// Enrich muscle names in chunks of 50 ids.
	for i := 0; i < len(order); i += muscleChunkSize {
		end := i + muscleChunkSize
		if end > len(order) {
			end = len(order)
		}
		chunk := order[i:end]
		parts := make([]string, len(chunk))
		for j, id := range chunk {
			parts[j] = "ids=" + strconv.FormatInt(id, 10)
		}
		detEnv, err := client.GetJSON(ctx, "/app/actionLibraryGroup/list?"+strings.Join(parts, "&"))
		if err != nil {
			return nil, err
		}
		var details []struct {
			ID                  *json.Number `json:"id"`
			MainMuscleGroupName string       `json:"mainMuscleGroupName"`
		}
		_ = json.Unmarshal(detEnv.Data, &details)
		for _, d := range details {
			if d.ID == nil {
				continue
			}
			did, err := numToInt64(*d.ID)
			if err != nil {
				continue
			}
			if ex, ok := byID[did]; ok {
				ex.Muscle = d.MainMuscleGroupName
			}
		}
	}

	out := make([]Exercise, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// truthy reports whether a raw JSON value is "truthy" in the Python sense, used
// for the isCustom tab flag (true / non-zero / non-empty). Absent, null, false,
// 0, and "" are falsy.
func truthy(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	switch s {
	case "", "null", "false", "0", "0.0", `""`, "[]", "{}":
		return false
	default:
		return true
	}
}
