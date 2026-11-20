package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stozo04/speediance-cli/v2/internal/config"
)

const (
	reservationPath    = "/app/templateReservation"
	createTemplatePath = "/app/v2/customTrainingTemplate"
	loginPath          = "/app/v2/login/verifyIdentity"
	reservationOK      = `{"code":0,"msg":"Success","data":true}`
)

// scheduleServer stands in for the account during push: it can override the
// create and reservation replies and fail re-login, and proxies every other
// request to fakeAPI. It records each path and the reservation body.
type scheduleServer struct {
	*httptest.Server
	createReply      string
	reservationReply string
	reservationCode  int
	reloginFails     bool

	mu              sync.Mutex
	paths           []string
	reservationBody map[string]any
}

func newScheduleServer(t *testing.T, configure func(*scheduleServer)) *scheduleServer {
	t.Helper()
	api := fakeAPI(t)
	t.Cleanup(api.Close)
	target, err := url.Parse(api.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	s := &scheduleServer{reservationReply: reservationOK, reservationCode: http.StatusOK}
	if configure != nil {
		configure(s)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.record(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == reservationPath:
			s.recordReservation(t, r)
			w.WriteHeader(s.reservationCode)
			_, _ = w.Write([]byte(s.reservationReply))
		case r.URL.Path == createTemplatePath && s.createReply != "":
			_, _ = w.Write([]byte(s.createReply))
		case r.URL.Path == loginPath && s.reloginFails:
			_, _ = w.Write([]byte(`{"code":1,"msg":"account locked"}`))
		default:
			proxy.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *scheduleServer) record(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths = append(s.paths, path)
}

func (s *scheduleServer) recordReservation(t *testing.T, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Errorf("reservation body is not JSON: %s", raw)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reservationBody = body
}

func (s *scheduleServer) called(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.paths {
		if p == path {
			return true
		}
	}
	return false
}

func examplePlan(t *testing.T) string {
	t.Helper()
	planAbs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "plans", "example-push.json"))
	if err != nil {
		t.Fatal(err)
	}
	return planAbs
}

func daysFromToday(n int) string {
	return time.Now().AddDate(0, 0, n).Format(time.DateOnly)
}

func decodeScheduledPush(t *testing.T, out string) scheduledPush {
	t.Helper()
	var res scheduledPush
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("push --date --json is not a scheduled push: %v\n%s", err, out)
	}
	return res
}

func requireExit(t *testing.T, err error, want int) {
	t.Helper()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != want {
		t.Fatalf("err = %v, want exit %d", err, want)
	}
}

func TestPushJSONWithoutDatePrintsCreateDataVerbatim(t *testing.T) {
	srv := newScheduleServer(t, nil)
	out, errb, err := runCLI(t, srv.URL, "push", examplePlan(t), "--json")
	if err != nil {
		t.Fatalf("push failed: %v\nstderr: %s", err, errb)
	}
	if strings.TrimSpace(out) != `"tpl_code_123"` {
		t.Errorf("push --json = %s, want the create response data unchanged", out)
	}
	if srv.called(reservationPath) {
		t.Error("push without --date must not touch the calendar")
	}
}

func TestPushWithoutDateToleratesUnexpectedCreateData(t *testing.T) {
	srv := newScheduleServer(t, func(s *scheduleServer) { s.createReply = `{"code":0,"data":{"id":999}}` })
	out, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--json")
	if err != nil {
		t.Fatalf("plain push must not depend on the create data's shape: %v", err)
	}
	if strings.TrimSpace(out) != "{\n  \"id\": 999\n}" {
		t.Errorf("push --json = %s, want the create data unchanged", out)
	}
}

func TestPushScheduleDateReservesTheCreatedTemplate(t *testing.T) {
	day := daysFromToday(7)
	srv := newScheduleServer(t, nil)
	out, errb, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", day, "--json")
	if err != nil {
		t.Fatalf("push --date failed: %v\nstderr: %s", err, errb)
	}
	res := decodeScheduledPush(t, out)
	if res.TemplateCode != "tpl_code_123" || res.ScheduledDate == nil || *res.ScheduledDate != day {
		t.Errorf("push --date --json = %+v, want template_code tpl_code_123 scheduled for %s", res, day)
	}
	want := map[string]any{
		"status":       float64(1),
		"deviceType":   float64(config.DefaultDeviceType),
		"thatDay":      day,
		"templateCode": "tpl_code_123",
	}
	for k, v := range want {
		if srv.reservationBody[k] != v {
			t.Errorf("reservation %s = %v, want %v (body %v)", k, srv.reservationBody[k], v, srv.reservationBody)
		}
	}
}

func TestPushScheduleDateHumanOutput(t *testing.T) {
	before := daysFromToday(0)
	srv := newScheduleServer(t, nil)
	out, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", "today")
	if err != nil {
		t.Fatal(err)
	}
	after := daysFromToday(0)
	if !strings.Contains(out, "scheduled it for "+before) && !strings.Contains(out, "scheduled it for "+after) {
		t.Errorf("human output does not name today: %s", out)
	}
}

func TestPushDryRunWithDateShowsDayWithoutWriting(t *testing.T) {
	day := daysFromToday(3)
	srv := newScheduleServer(t, nil)
	out, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--dry-run", "--date", day, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("bad dry-run json: %v\n%s", err, out)
	}
	if res["scheduled_date"] != day || res["payload"] == nil {
		t.Errorf("dry-run --json = %v, want the payload and scheduled_date %s", res, day)
	}
	if srv.called(createTemplatePath) || srv.called(reservationPath) {
		t.Error("--dry-run must not create or reserve anything")
	}
}

func TestPushRejectsBadDateBeforeWriting(t *testing.T) {
	for _, day := range []string{"yesterday", "2020-01-01", " ", "next week"} {
		t.Run(day, func(t *testing.T) {
			srv := newScheduleServer(t, nil)
			_, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", day)
			requireExit(t, err, ExitUsage)
			if srv.called(createTemplatePath) {
				t.Errorf("--date %q created a template before rejecting the date", day)
			}
		})
	}
}

func TestParseScheduleDate(t *testing.T) {
	lateEvening := time.Date(2026, 11, 20, 23, 59, 0, 0, time.Local)
	cases := []struct {
		in, want string
		usage    bool
	}{
		{in: "", want: ""},
		{in: "today", want: "2026-11-20"},
		{in: "TODAY", want: "2026-11-20"},
		{in: "2026-11-20", want: "2026-11-20"},
		{in: "2026-11-21", want: "2026-11-21"},
		{in: "2026-11-19", usage: true},
		{in: "yesterday", usage: true},
		{in: "  ", usage: true},
		{in: "11/21/2026", usage: true},
	}
	for _, c := range cases {
		got, err := parseScheduleDate(c.in, lateEvening)
		if c.usage {
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != ExitUsage {
				t.Errorf("parseScheduleDate(%q) err = %v, want a usage error", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("parseScheduleDate(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestPushScheduleFailureKeepsTemplateCodeAndExitsThree(t *testing.T) {
	cases := map[string]func(*scheduleServer){
		"error code with msg": func(s *scheduleServer) { s.reservationReply = `{"code":1,"msg":"busy"}` },
		"false response":      func(s *scheduleServer) { s.reservationReply = `{"code":0,"data":false}` },
		"http 500": func(s *scheduleServer) {
			s.reservationCode = http.StatusInternalServerError
			s.reservationReply = `oops`
		},
		"re-login fails": func(s *scheduleServer) {
			s.reservationReply = `{"code":91,"msg":"token expired"}`
			s.reloginFails = true
		},
	}
	for name, configure := range cases {
		t.Run(name, func(t *testing.T) {
			srv := newScheduleServer(t, configure)
			out, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", "today", "--json")
			requireExit(t, err, ExitScheduleFailed)
			if !strings.Contains(err.Error(), "tpl_code_123") {
				t.Errorf("error does not name the created template code: %v", err)
			}
			res := decodeScheduledPush(t, out)
			if res.TemplateCode != "tpl_code_123" || res.ScheduledDate != nil {
				t.Errorf("stdout = %+v, want the created template_code with a null scheduled_date", res)
			}
		})
	}
}

func TestPushScheduleFailureShowsServerMsg(t *testing.T) {
	srv := newScheduleServer(t, func(s *scheduleServer) { s.reservationReply = `{"code":1,"msg":"busy"}` })
	_, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", "today")
	if err == nil || !strings.Contains(err.Error(), "busy (code 1)") {
		t.Errorf("err = %v, want the server's msg in the error", err)
	}
}

func TestPushScheduleFailureHumanModePrintsNoSuccess(t *testing.T) {
	srv := newScheduleServer(t, func(s *scheduleServer) { s.reservationReply = `{"code":1,"msg":"busy"}` })
	out, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", "today")
	requireExit(t, err, ExitScheduleFailed)
	if out != "" {
		t.Errorf("stdout = %q, want nothing when the reservation fails", out)
	}
}

func TestPushScheduleWithUnreadableTemplateCodeExitsThree(t *testing.T) {
	srv := newScheduleServer(t, func(s *scheduleServer) { s.createReply = `{"code":0,"data":{"id":999}}` })
	_, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", "today", "--json")
	requireExit(t, err, ExitScheduleFailed)
	if srv.called(reservationPath) {
		t.Error("reserved a template without a code")
	}
}

func TestTemplateCodeFromRejectsNonStringData(t *testing.T) {
	for _, data := range []string{`{"id":999}`, `""`, `null`} {
		if _, err := templateCodeFrom(json.RawMessage(data)); err == nil {
			t.Errorf("templateCodeFrom(%s) = nil error, want an error", data)
		}
	}
	if code, err := templateCodeFrom(json.RawMessage(`"6afe53cee4b06ab9dced9026"`)); err != nil || code != "6afe53cee4b06ab9dced9026" {
		t.Errorf("templateCodeFrom(real response) = %q, %v", code, err)
	}
}
