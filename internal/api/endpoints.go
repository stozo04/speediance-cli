package api

import (
	"context"
	"strconv"
	"time"

	"github.com/stozo04/speediance-cli/internal/workout"
)

// FetchWorkouts returns completed sessions over the last `days` days (GOAL.md
// §9.2). The date window is [today+1-days, today+1) so "today" is always
// included, matching the Python client. A non-zero API code yields an empty
// list plus a stderr warning (not an error), again mirroring Python.
func (c *Client) FetchWorkouts(ctx context.Context, days int) ([]workout.Workout, error) {
	y, m, d := c.now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	end := today.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -days)

	path := "/mobile/v2/report/userTrainingDataRecord" +
		"?startDate=" + start.Format("2006-01-02") +
		"&endDate=" + end.Format("2006-01-02")

	env, err := c.GetJSON(ctx, path)
	if err != nil {
		return nil, err
	}
	if env.Code != 0 {
		c.logger.Warn("fetch_workouts failed", "message", env.Message)
		return nil, nil
	}
	return workout.ParseRecords(env.Data)
}

// FetchDetail returns per-set detail for one session id (GOAL.md §9.3), used by
// the `session` command which has only the id.
func (c *Client) FetchDetail(ctx context.Context, trainingID int64) (*workout.Workout, error) {
	w := &workout.Workout{TrainingID: trainingID}
	if err := c.PopulateDetail(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// PopulateDetail adds the completion rate and per-set detail to an existing
// workout (used by `sync`, which already has the summary fields). It makes two
// GETs: cttTrainingInfo for the completion rate, then cttTrainingInfoDetail for
// the per-set list. Freestyle "Free Lift" sessions return no detail, leaving
// Sets empty. Mirrors the Python client's fetch_detail, which mutates in place.
func (c *Client) PopulateDetail(ctx context.Context, w *workout.Workout) error {
	id := strconv.FormatInt(w.TrainingID, 10)

	info, err := c.GetJSON(ctx, "/app/trainingInfo/cttTrainingInfo/"+id)
	if err != nil {
		return err
	}
	if info.Code == 0 {
		w.SetCompletionRate(info.Data)
	}

	detail, err := c.GetJSON(ctx, "/app/trainingInfo/cttTrainingInfoDetail/"+id)
	if err != nil {
		return err
	}
	if detail.Code == 0 {
		// Only a JSON array yields sets; any other shape (null/object) is
		// silently treated as "no per-set detail", matching Python's
		// isinstance(data, list) guard.
		if err := w.AddDetailSets(detail.Data); err != nil {
			c.logger.Debug("no per-set detail list", "training_id", id, "err", err)
		}
	}
	return nil
}
