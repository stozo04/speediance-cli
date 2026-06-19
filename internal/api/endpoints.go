package api

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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
	return c.fetchRecords(ctx, start, end)
}

// FetchDay returns each session recorded on the given local calendar date, fully
// resolved to its type-correct detail. This is the engine behind the `today`
// command: the agent asks for "the workout(s) on this day" without knowing what
// kind they were, and the tool discovers each session's type from the list (which
// carries the authoritative numeric type alongside the trainingId — so dispatch
// is collision-proof, unlike resolving from a bare id) and fetches the right
// namespace. Returns a non-nil (possibly empty) slice so JSON encodes as [].
func (c *Client) FetchDay(ctx context.Context, date time.Time) ([]workout.SessionDetail, error) {
	y, m, d := date.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	ws, err := c.fetchRecords(ctx, day, day.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	target := day.Format("2006-01-02")
	out := make([]workout.SessionDetail, 0, len(ws))
	for _, w := range ws {
		// The window can spill a neighbouring day; keep only the target date.
		if dt := w.Date(); dt == nil || *dt != target {
			continue
		}
		sd, err := c.ResolveSession(ctx, w.TrainingID, modeForType(w.SessionType))
		if err != nil {
			return nil, err
		}
		out = append(out, *sd)
	}
	return out, nil
}

// fetchRecords pulls the userTrainingDataRecord list for [start, end) and parses
// it. A non-zero API code is a soft failure (empty list + warning), mirroring the
// Python client.
func (c *Client) fetchRecords(ctx context.Context, start, end time.Time) ([]workout.Workout, error) {
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

// DispatchMode selects which session namespace(s) ResolveSession consults.
type DispatchMode int

const (
	// Auto probes the program namespace first, then falls back to free. It is the
	// default for `session <id>` when the session type is unknown.
	Auto DispatchMode = iota
	// FreeFirst probes free first, then program. Used when the list says the
	// session is type 1, so the (rare) trainingId collision with a program of the
	// same id is sidestepped — the free session is found before program is queried.
	FreeFirst
	// ProgramOnly consults only the program namespace and never falls back; it
	// backs the --program override for an id valid in both namespaces.
	ProgramOnly
	// FreeOnly consults only the free namespace and never falls back; it backs the
	// --free override for an id valid in both namespaces.
	FreeOnly
)

// modeForType picks a dispatch mode from the list's numeric session type. Known
// types steer to their namespace (probing the other only as a fallback); an
// unknown type uses Auto so a future session kind still resolves.
func modeForType(sessionType int) DispatchMode {
	switch sessionType {
	case 1:
		return FreeFirst
	case 5:
		return Auto // program-first
	default:
		return Auto
	}
}

// ResolveSession fetches one session's detail, autonomously selecting the right
// Speediance namespace. Program detail lives at cttTrainingInfo[Detail]; free-lift
// detail (incl. rowing/ski) lives at freeTraining[Detail]; and a trainingId can be
// valid in BOTH namespaces as unrelated sessions, so the caller's mode disambiguates.
// The result is the uniform, verbatim SessionDetail (info/detail carry whichever
// namespace's raw payload answered; kind names it; both null when neither has data).
func (c *Client) ResolveSession(ctx context.Context, trainingID int64, mode DispatchMode) (*workout.SessionDetail, error) {
	id := strconv.FormatInt(trainingID, 10)
	sd := &workout.SessionDetail{TrainingID: trainingID}

	tryProgram := func() (bool, error) {
		info, detail, err := c.programDetail(ctx, id)
		if err != nil {
			return false, err
		}
		if hasData(info) || hasData(detail) {
			sd.Kind, sd.Info, sd.Detail = "program", info, detail
			return true, nil
		}
		return false, nil
	}
	tryFree := func() (bool, error) {
		info, detail, err := c.freeDetail(ctx, id)
		if err != nil {
			return false, err
		}
		if hasData(info) || hasData(detail) {
			sd.Kind, sd.Info, sd.Detail = "free", info, detail
			return true, nil
		}
		return false, nil
	}

	var err error
	switch mode {
	case ProgramOnly:
		_, err = tryProgram()
	case FreeOnly:
		_, err = tryFree()
	case FreeFirst:
		err = firstThen(tryFree, tryProgram)
	default: // Auto / ProgramFirst
		err = firstThen(tryProgram, tryFree)
	}
	if err != nil {
		return nil, err
	}
	return sd, nil
}

// firstThen runs primary; if it found nothing (and didn't error), runs fallback.
func firstThen(primary, fallback func() (bool, error)) error {
	found, err := primary()
	if err != nil || found {
		return err
	}
	_, err = fallback()
	return err
}

// programDetail fetches the program (Coach/template) namespace payloads verbatim.
func (c *Client) programDetail(ctx context.Context, id string) (info, detail json.RawMessage, err error) {
	return c.pair(ctx,
		"/app/trainingInfo/cttTrainingInfo/"+id,
		"/app/trainingInfo/cttTrainingInfoDetail/"+id)
}

// freeDetail fetches the free-lift (freestyle/rowing/ski) namespace payloads verbatim.
func (c *Client) freeDetail(ctx context.Context, id string) (info, detail json.RawMessage, err error) {
	return c.pair(ctx,
		"/app/trainingInfo/freeTraining/"+id,
		"/app/trainingInfo/freeTrainingDetail/"+id)
}

// pair GETs an info path and a detail path and returns their verbatim data
// payloads (nil when the call returned a non-zero code or empty body).
func (c *Client) pair(ctx context.Context, infoPath, detailPath string) (info, detail json.RawMessage, err error) {
	ei, err := c.GetJSON(ctx, infoPath)
	if err != nil {
		return nil, nil, err
	}
	ed, err := c.GetJSON(ctx, detailPath)
	if err != nil {
		return nil, nil, err
	}
	if ei.Code == 0 {
		info = rawData(ei.Data)
	}
	if ed.Code == 0 {
		detail = rawData(ed.Data)
	}
	return info, detail, nil
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

// hasData reports whether a payload carries something a session actually has —
// i.e. not absent, null, or an empty array/object. It decides whether a namespace
// "answered" so ResolveSession knows to stop or fall back.
func hasData(d json.RawMessage) bool {
	s := strings.TrimSpace(string(d))
	return s != "" && s != "null" && s != "[]" && s != "{}"
}
