package api

import (
	"context"
	"encoding/json"
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

// FetchSessionDetail returns the full detail for one session id (GOAL.md §9.3) as
// a faithful, lossless passthrough. A session spans two endpoints — cttTrainingInfo
// (session-level, including completionRate) and cttTrainingInfoDetail (per-exercise,
// per-rep detail) — and this fetches both and carries their *verbatim* data
// payloads through untouched. The CLI parses, renames, reshapes, or derives
// nothing here; consumers decide shape. A non-zero API code or an empty body for
// either call leaves that payload nil (emitted as JSON null), preserving absence
// rather than fabricating data — e.g. a "Free Lift" freestyle session simply
// returns a null detail.
func (c *Client) FetchSessionDetail(ctx context.Context, trainingID int64) (*workout.SessionDetail, error) {
	id := strconv.FormatInt(trainingID, 10)

	info, err := c.GetJSON(ctx, "/app/trainingInfo/cttTrainingInfo/"+id)
	if err != nil {
		return nil, err
	}
	detail, err := c.GetJSON(ctx, "/app/trainingInfo/cttTrainingInfoDetail/"+id)
	if err != nil {
		return nil, err
	}

	sd := &workout.SessionDetail{TrainingID: trainingID}
	if info.Code == 0 {
		sd.Info = rawData(info.Data)
	}
	if detail.Code == 0 {
		sd.Detail = rawData(detail.Data)
	}
	return sd, nil
}

// rawData returns the payload verbatim, or nil for an absent/empty one so it
// serializes as JSON null rather than invalid empty bytes. The payload itself is
// never altered.
func rawData(d json.RawMessage) json.RawMessage {
	if len(d) == 0 {
		return nil
	}
	return d
}
