package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// fakeAPI dispatches the endpoints the commands hit, returning small fixtures.
// Push uses the same rules as the template golden (variant=id*10, unilateral on
// odd ids) so a CLI-level push --dry-run can be checked against the golden file.
func fakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	idsRe := regexp.MustCompile(`ids=(\d+)`)
	uniRe := regexp.MustCompile(`actionLibraryGroup/(\d+)\?isDisplay=1`)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		q := r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch {
		case p == "/app/v2/login/verifyIdentity":
			_, _ = w.Write([]byte(`{"code":0,"data":{"isExist":true,"hasPwd":true}}`))
		case p == "/app/v2/login/byPass":
			_, _ = w.Write([]byte(`{"code":0,"data":{"token":"T","appUserId":1}}`))
		case p == "/mobile/v2/report/userTrainingDataRecord":
			_, _ = w.Write([]byte(`{"code":0,"data":[
				{"trainingId":123,"title":"Upper Body","courseTypeStr":"Strength",
				 "startTimestamp":1718400000,"trainingTime":2700,"calorie":320,"totalCapacity":4200.0}]}`))
		case strings.HasPrefix(p, "/app/trainingInfo/cttTrainingInfoDetail/"):
			_, _ = w.Write([]byte(`{"code":0,"data":[
				{"actionLibraryName":"Row","maxWeight":45,"finishedReps":[
					{"finishedCount":12,"targetCount":12,"weight":20.0,"maxHeartRate":148,"leftRight":0}]}]}`))
		case strings.HasPrefix(p, "/app/trainingInfo/cttTrainingInfo/"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"completionRate":0.95}}`))
		case p == "/app/actionLibraryTab/list":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":1,"name":"Chest","isCustom":false}]}`))
		case p == "/app/actionLibraryGroup/trainingPartGroup":
			_, _ = w.Write([]byte(`{"code":0,"data":[{"actionLibraryGroupList":[
				{"id":304,"title":"Chest Press"},{"id":465,"title":"Lateral Raise"}]}]}`))
		case strings.HasPrefix(p, "/app/actionLibraryGroup/list"):
			// Used by both library muscle-enrichment and push variant-resolution.
			matches := idsRe.FindAllStringSubmatch(q, -1)
			var rows []string
			for _, m := range matches {
				id := m[1]
				v, _ := strconv.ParseInt(id, 10, 64)
				rows = append(rows, `{"id":`+id+`,"mainMuscleGroupName":"Chest",`+
					`"actionLibraryList":[{"id":`+strconv.FormatInt(v*10, 10)+`}]}`)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[` + strings.Join(rows, ",") + `]}`))
		case uniRe.MatchString(p + "?" + q):
			m := uniRe.FindStringSubmatch(p + "?" + q)
			gid, _ := strconv.ParseInt(m[1], 10, 64)
			lr := 0
			if gid%2 == 1 {
				lr = 1
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"isLeftRight":` + strconv.Itoa(lr) + `}}`))
		case p == "/app/v2/customTrainingTemplate":
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":999,"name":"created"}}`))
		default:
			t.Logf("unhandled path: %s?%s", p, q)
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		}
	}))
}

// runCLI executes the root command in an isolated temp CWD with credentials and
// a preset token cache (so no login round-trip), capturing stdout and stderr.
func runCLI(t *testing.T, serverURL string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	clearSpeedianceEnv(t)
	t.Setenv("SPEEDIANCE_EMAIL", "e@example.com")
	t.Setenv("SPEEDIANCE_PASSWORD", "pw")
	if err := os.WriteFile(filepath.Join(dir, ".token.json"),
		[]byte(`{"token":"T","user_id":"1"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	baseURLOverride = serverURL
	t.Cleanup(func() { baseURLOverride = "" })

	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errb.String(), err
}

func clearSpeedianceEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SPEEDIANCE_EMAIL", "SPEEDIANCE_PASSWORD", "SPEEDIANCE_REGION",
		"SPEEDIANCE_DEVICE_TYPE", "SPEEDIANCE_CONFIG",
		"SPEEDIANCE_TOKEN_CACHE",
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

func TestWorkoutsJSON(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()
	out, _, err := runCLI(t, srv.URL, "workouts", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	r := rows[0]
	// Exact field set + types per the §9.2 contract.
	wantKeys := []string{"training_id", "title", "date", "duration_secs", "calories", "volume", "type"}
	for _, k := range wantKeys {
		if _, ok := r[k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}
	if r["title"] != "Upper Body" {
		t.Errorf("title = %v", r["title"])
	}
	// volume must serialize as a float (4200.0), not 4200.
	if !strings.Contains(out, `"volume": 4200.0`) {
		t.Errorf("volume not float-formatted:\n%s", out)
	}
}

func TestWorkoutsHumanToStdout(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()
	out, _, err := runCLI(t, srv.URL, "workouts")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Found 1 session(s)") || !strings.Contains(out, "(id 123)") {
		t.Errorf("human output wrong:\n%s", out)
	}
}

func TestSessionJSON(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()
	out, _, err := runCLI(t, srv.URL, "session", "123", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		TrainingID     int             `json:"training_id"`
		CompletionRate json.RawMessage `json:"completion_rate"`
		Exercises      []struct {
			Name string `json:"name"`
			Sets []struct {
				Set       int             `json:"set"`
				Weight    json.RawMessage `json:"weight"`
				LeftRight int             `json:"left_right"`
			} `json:"sets"`
		} `json:"exercises"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if doc.TrainingID != 123 || string(doc.CompletionRate) != "0.95" {
		t.Errorf("session header wrong: %+v", doc)
	}
	if len(doc.Exercises) != 1 || doc.Exercises[0].Name != "Row" {
		t.Fatalf("exercises wrong: %+v", doc.Exercises)
	}
	if string(doc.Exercises[0].Sets[0].Weight) != "20.0" {
		t.Errorf("weight = %s, want 20.0", doc.Exercises[0].Sets[0].Weight)
	}
}

func TestLibraryWritesFileAndStderr(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()
	out, errb, err := runCLI(t, srv.URL, "library", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errb, "Saved 2 exercises to library.json") {
		t.Errorf("stderr save line missing: %q", errb)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 || rows[0]["muscle"] != "Chest" {
		t.Errorf("library rows wrong: %+v", rows)
	}
	// The catalog file must have been written to the CWD.
	if _, err := os.Stat("library.json"); err != nil {
		t.Errorf("library.json not written: %v", err)
	}
}

func TestPushDryRunGoldenParity(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()

	// Copy the sample plan into the isolated CWD set up by runCLI is awkward
	// (runCLI chdirs), so pass an absolute plan path.
	planAbs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "plans", "example-push.json"))
	if err != nil {
		t.Fatal(err)
	}
	goldenAbs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "golden", "example-push.payload.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(goldenAbs)
	if err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, srv.URL, "push", planAbs, "--dry-run", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if out != string(want) {
		t.Errorf("push --dry-run --json not byte-identical to golden.\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestAuthErrorExitsTwo(t *testing.T) {
	// A login that fails should surface as exit code 2.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"message":"bad identity"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Chdir(dir)
	clearSpeedianceEnv(t)
	t.Setenv("SPEEDIANCE_EMAIL", "e@example.com")
	t.Setenv("SPEEDIANCE_PASSWORD", "pw")
	baseURLOverride = srv.URL
	t.Cleanup(func() { baseURLOverride = "" })

	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"login"})
	err := root.Execute()

	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want *ExitError, got %v", err)
	}
	if exit.Code != ExitAuth {
		t.Errorf("exit code = %d, want %d", exit.Code, ExitAuth)
	}
}

func TestMissingCredentialsErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearSpeedianceEnv(t) // no email/password anywhere.

	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"workouts", "--json"})
	err := root.Execute()

	var exit *ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want *ExitError, got %v", err)
	}
	if exit.Code != ExitConfig {
		t.Errorf("exit code = %d, want %d", exit.Code, ExitConfig)
	}
}

func TestWorkoutsEmptyJSONIsArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
	}))
	defer srv.Close()
	out, _, err := runCLI(t, srv.URL, "workouts", "--json")
	if err != nil {
		t.Fatal(err)
	}
	// Must be "[]" (like Python's json.dumps([])), never "null".
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("empty workouts --json = %q, want []", strings.TrimSpace(out))
	}
}

func TestSessionFreestyleEmptyExercises(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "cttTrainingInfoDetail") {
			// Free Lift sessions return no per-set list.
			_, _ = w.Write([]byte(`{"code":0,"data":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"completionRate":0.0}}`))
	}))
	defer srv.Close()
	out, _, err := runCLI(t, srv.URL, "session", "55", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"exercises": []`) {
		t.Errorf("freestyle session should have empty exercises array:\n%s", out)
	}
	// Human mode prints the freestyle hint.
	hout, _, err := runCLI(t, srv.URL, "session", "55")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hout, "freestyle 'Free Lift'") {
		t.Errorf("missing freestyle hint:\n%s", hout)
	}
}

func TestVersionJSON(t *testing.T) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"version", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatalf("version --json not valid: %v\n%s", err, out.String())
	}
	for _, k := range []string{"version", "commit", "date", "go"} {
		if _, ok := v[k]; !ok {
			t.Errorf("version json missing %q", k)
		}
	}
}
