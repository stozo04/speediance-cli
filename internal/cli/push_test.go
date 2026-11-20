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

const reservationPath = "/app/templateReservation"

// scheduleServer answers the calendar reservation with reservationReply and
// proxies every other request to fakeAPI, recording the paths and the
// reservation body so tests can assert what reached the account.
type scheduleServer struct {
	*httptest.Server
	mu              sync.Mutex
	paths           []string
	reservationBody map[string]any
}

func newScheduleServer(t *testing.T, reservationReply string) *scheduleServer {
	t.Helper()
	api := fakeAPI(t)
	t.Cleanup(api.Close)
	target, err := url.Parse(api.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	s := &scheduleServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		s.mu.Unlock()
		if r.URL.Path != reservationPath {
			proxy.ServeHTTP(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("reservation body is not JSON: %s", raw)
		}
		s.mu.Lock()
		s.reservationBody = body
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reservationReply))
	}))
	t.Cleanup(s.Close)
	return s
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

func decodePushResult(t *testing.T, out string) pushResult {
	t.Helper()
	var res pushResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("push --json is not a push result: %v\n%s", err, out)
	}
	return res
}

func TestPushJSONWithoutDateReportsTemplateCode(t *testing.T) {
	srv := newScheduleServer(t, `{"code":0,"data":true}`)
	out, errb, err := runCLI(t, srv.URL, "push", examplePlan(t), "--json")
	if err != nil {
		t.Fatalf("push failed: %v\nstderr: %s", err, errb)
	}
	if strings.TrimSpace(out) != "{\n  \"template_code\": \"tpl_code_123\",\n  \"scheduled_date\": null\n}" {
		t.Errorf("push --json = %s, want template_code and a null scheduled_date", out)
	}
	if srv.called(reservationPath) {
		t.Error("push without --date must not touch the calendar")
	}
}

func TestPushScheduleDateReservesTheCreatedTemplate(t *testing.T) {
	day := daysFromToday(7)
	srv := newScheduleServer(t, `{"code":0,"data":true}`)
	out, errb, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", day, "--json")
	if err != nil {
		t.Fatalf("push --date failed: %v\nstderr: %s", err, errb)
	}
	res := decodePushResult(t, out)
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
	srv := newScheduleServer(t, `{"code":0,"data":true}`)
	out, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", "today")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "scheduled it for "+daysFromToday(0)) {
		t.Errorf("human output does not name the scheduled day: %s", out)
	}
}

func TestPushDryRunWithDateShowsDayWithoutWriting(t *testing.T) {
	day := daysFromToday(3)
	srv := newScheduleServer(t, `{"code":0,"data":true}`)
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
	if srv.called("/app/v2/customTrainingTemplate") || srv.called(reservationPath) {
		t.Error("--dry-run must not create or reserve anything")
	}
}

func TestPushRejectsPastDateBeforeWriting(t *testing.T) {
	for _, day := range []string{"yesterday", "2020-01-01"} {
		t.Run(day, func(t *testing.T) {
			srv := newScheduleServer(t, `{"code":0,"data":true}`)
			_, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", day)
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != ExitUsage {
				t.Fatalf("--date %s: err = %v, want a usage error (exit %d)", day, err, ExitUsage)
			}
			if srv.called("/app/v2/customTrainingTemplate") {
				t.Errorf("--date %s created a template before rejecting the date", day)
			}
		})
	}
}

func TestPushScheduleFailureKeepsTemplateCodeAndExitsThree(t *testing.T) {
	replies := map[string]string{
		"error code":     `{"code":1,"message":"busy"}`,
		"false response": `{"code":0,"data":false}`,
	}
	for name, reply := range replies {
		t.Run(name, func(t *testing.T) {
			srv := newScheduleServer(t, reply)
			out, _, err := runCLI(t, srv.URL, "push", examplePlan(t), "--date", "today", "--json")
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != ExitScheduleFailed {
				t.Fatalf("err = %v, want exit %d", err, ExitScheduleFailed)
			}
			if !strings.Contains(err.Error(), "tpl_code_123") {
				t.Errorf("error does not name the created template code: %v", err)
			}
			res := decodePushResult(t, out)
			if res.TemplateCode != "tpl_code_123" || res.ScheduledDate != nil {
				t.Errorf("stdout = %+v, want the created template_code with a null scheduled_date", res)
			}
		})
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
