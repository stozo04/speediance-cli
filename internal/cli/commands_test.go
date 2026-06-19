package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
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
				{"trainingId":123,"title":"Upper Body","courseTypeStr":"Strength","type":5,
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
	// Pin the token cache to this temp dir (an explicit override) so the test
	// stays hermetic: it neither reads/writes the real per-user cache nor
	// triggers the legacy-migration path.
	tokenPath := filepath.Join(dir, ".token.json")
	t.Setenv("SPEEDIANCE_TOKEN_CACHE", tokenPath)
	if err := os.WriteFile(tokenPath,
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
	// Exact field set + types per the §9.2 contract (+ kind, added for dispatch).
	wantKeys := []string{"training_id", "title", "date", "duration_secs", "calories", "volume", "type", "kind"}
	for _, k := range wantKeys {
		if _, ok := r[k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}
	if r["title"] != "Upper Body" {
		t.Errorf("title = %v", r["title"])
	}
	// type 5 maps to kind "program" so an agent can filter without endpoint knowledge.
	if r["kind"] != "program" {
		t.Errorf("kind = %v, want program (type 5)", r["kind"])
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
	// Faithful passthrough: {training_id, kind, info, detail} carrying both
	// endpoints' raw payloads verbatim — no renaming, no derived fields.
	var doc struct {
		TrainingID int             `json:"training_id"`
		Kind       string          `json:"kind"`
		Info       json.RawMessage `json:"info"`
		Detail     []struct {
			ActionLibraryName string `json:"actionLibraryName"`
			FinishedReps      []struct {
				Weight       json.RawMessage `json:"weight"`
				MaxHeartRate json.RawMessage `json:"maxHeartRate"`
			} `json:"finishedReps"`
		} `json:"detail"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if doc.TrainingID != 123 {
		t.Errorf("training_id = %d, want 123", doc.TrainingID)
	}
	if doc.Kind != "program" {
		t.Errorf("kind = %q, want program", doc.Kind)
	}
	// The session-level info payload is carried through verbatim (incl. completionRate).
	if !strings.Contains(string(doc.Info), `"completionRate": 0.95`) {
		t.Errorf("info not passed through verbatim: %s", doc.Info)
	}
	if len(doc.Detail) != 1 || doc.Detail[0].ActionLibraryName != "Row" {
		t.Fatalf("detail wrong: %+v", doc.Detail)
	}
	// Per-rep fields are emitted with their original Speediance names and values.
	rep := doc.Detail[0].FinishedReps[0]
	if string(rep.Weight) != "20.0" || string(rep.MaxHeartRate) != "148" {
		t.Errorf("rep not verbatim: weight=%s maxHeartRate=%s", rep.Weight, rep.MaxHeartRate)
	}
	// None of the renamed / derived fields from the superseded design.
	for _, banned := range []string{"completion_rate", `"exercises"`, "weight_source", "reps_detail", "max_hr"} {
		if strings.Contains(out, banned) {
			t.Errorf("output contains forbidden field %q:\n%s", banned, out)
		}
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
	// Keep the token cache inside the temp dir so the test never touches the real
	// per-user cache (login fails before writing, but stay hermetic regardless).
	t.Setenv("SPEEDIANCE_TOKEN_CACHE", filepath.Join(dir, ".token.json"))
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

// TestEndToEndMigratesLegacyTokenToCacheDir exercises the full wired path through
// the real command tree (NewRootCmd → resolveConfig → MigrateLegacy → token load
// → API call → token write-back) against a fake API server. It is the end-to-end
// proof of the issue #17 fix plus the CLI_CONVENTIONS.md §1 cache-dir move: a
// legacy ./.token.json is relocated up to the per-user CACHE dir (not CWD, not the
// roaming config dir), and the SAME session is reused — no forced re-login.
func TestEndToEndMigratesLegacyTokenToCacheDir(t *testing.T) {
	srv := fakeAPI(t)
	defer srv.Close()

	cwd := t.TempDir()
	t.Chdir(cwd)
	clearSpeedianceEnv(t)
	t.Setenv("SPEEDIANCE_EMAIL", "e@example.com")
	t.Setenv("SPEEDIANCE_PASSWORD", "pw")

	// Point the per-user CACHE base at a temp dir so the default token path
	// resolves there instead of the real user cache dir. Deliberately do NOT set
	// SPEEDIANCE_TOKEN_CACHE — migration only runs for the default (unset) path.
	cacheHome := t.TempDir()
	t.Setenv("LocalAppData", cacheHome)   // Windows UserCacheDir
	t.Setenv("XDG_CACHE_HOME", cacheHome) // Linux/BSD UserCacheDir
	t.Setenv("HOME", cacheHome)           // macOS derives the cache base

	// A token cached the OLD way: ./.token.json in the working directory.
	legacy := filepath.Join(cwd, ".token.json")
	if err := os.WriteFile(legacy, []byte(`{"token":"LEGACY","user_id":"99"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	baseURLOverride = srv.URL
	t.Cleanup(func() { baseURLOverride = "" })

	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"workouts", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("workouts failed: %v\nstderr: %s", err, errb.String())
	}

	// 1) The credential no longer sits in the working directory.
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy ./.token.json still present after run (stat err = %v); migration must remove it", err)
	}

	// 2) It now lives under the per-user CACHE base (non-roaming), carrying the
	//    SAME token — proof the session was preserved, not re-logged-in.
	cacheBase, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	migrated := filepath.Join(cacheBase, "speediance", "token.json")
	data, err := os.ReadFile(migrated)
	if err != nil {
		t.Fatalf("token not found at cache-dir location %s: %v", migrated, err)
	}
	if !strings.Contains(string(data), "LEGACY") {
		t.Errorf("migrated token cache = %s, want it to preserve the LEGACY session token", data)
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

// TestSessionNoDataAnywhere: when neither namespace has data for the id, absence
// is preserved faithfully — kind "", info null, detail null (never fabricated).
func TestSessionNoDataAnywhere(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "cttTrainingInfoDetail"),
			strings.Contains(r.URL.Path, "freeTrainingDetail"):
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		default: // cttTrainingInfo / freeTraining: no session here
			_, _ = w.Write([]byte(`{"code":20,"message":"not found"}`))
		}
	}))
	defer srv.Close()
	out, _, err := runCLI(t, srv.URL, "session", "55", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"kind": ""`, `"info": null`, `"detail": null`} {
		if !strings.Contains(out, want) {
			t.Errorf("no-data session should contain %s:\n%s", want, out)
		}
	}
	// Human mode prints the no-detail hint (mentions freestyle + the override flags).
	hout, _, err := runCLI(t, srv.URL, "session", "55")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hout, "no detail found") {
		t.Errorf("missing no-detail hint:\n%s", hout)
	}
}

// sessionDetailServer serves the genuine issue-#23 session 940759: the real
// cttTrainingInfoDetail capture (set 1 fully telemetered, set 2 a sparse
// weights-only capture) plus a cttTrainingInfo payload carrying completionRate.
func sessionDetailServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "cttTrainingInfoDetail"):
			_, _ = w.Write([]byte(`{"code":0,"data":[
				{"actionLibraryName":"Standing Dual-Handle Hammer Curl","maxWeight":15.0,
				 "maxWeightCount":5,"score":16,"completionScore":5,"forceControlScore":4,
				 "bilateralBalanceScore":4,"amplitudeStableScore":3,"actionRating":3,"totalCapacity":554.0,
				 "finishedReps":[
					{"finishedCount":14,"targetCount":14,"capacity":330.0,"leftRight":0,"breakTime":60,
					 "trainingInfoDetail":{
						"weights":[15,15,15,15,15,10,10,10,10,10,10,10,10,10],
						"leftWeights":[15,15,15,15,15,10,10,10,10,10,10,10,10,10],
						"rightWeights":[15,15,15,15,15,10,10,10,10,10,10,10,10,10],
						"leftWatts":[41.65,51.84,49.19,44.87,41.33,35.67,28.9,27.35,25.21,35.9,29.44,28.5,32.58,27.5],
						"rightWatts":[26.28,55.05,54.9,47.86,39.97,37.79,33.65,28.38,24.25,28.6,29.96,29.07,33.35,27.62],
						"leftAmplitudes":[0.46,0.68,0.65,0.71,0.69,0.65,0.62,0.67,0.7,0.73,0.74,0.72,0.71,0.73],
						"rightAmplitudes":[0.46,0.66,0.67,0.7,0.67,0.76,0.66,0.65,0.73,0.78,0.7,0.73,0.73,0.73],
						"leftRopeSpeeds":[0.66,0.8,0.76,0.71,0.65,0.83,0.67,0.63,0.58,0.86,0.7,0.68,0.77,0.64],
						"leftFinishedTimes":[1.13,4.69,2.79,2.94,3.16,2.17,2.82,2.59,2.96,1.26,2.95,3.03,3.01,2.87],
						"leftBreakTimes":[1.23,0.42,0.14,0.07,0.35,0.7,0,0.14,1.61,0,0.28,0.14,1.96,0.14],
						"leftTimestamps":[1781815035511]}},
					{"finishedCount":14,"targetCount":14,"capacity":224.0,"leftRight":0,
					 "trainingInfoDetail":{"weights":[8,8,8,8,8,8,8,8,8,8,8,8,8,8]}}]}]}`))
		case strings.Contains(r.URL.Path, "cttTrainingInfo"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"completionRate":0.95,"trainingId":940759,"totalCapacity":554.0}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		}
	}))
}

// TestSessionFaithfulPassthroughJSON is the CLI-level guard for the faithful
// contract (issue #23, refined): session --json is a verbatim, lossless dump of
// both Speediance endpoints — every raw key present and unrenamed, sparse data
// preserved, nothing derived — and there is no --telemetry flag to "unlock" data.
func TestSessionFaithfulPassthroughJSON(t *testing.T) {
	srv := sessionDetailServer(t)
	defer srv.Close()

	out, _, err := runCLI(t, srv.URL, "session", "940759", "--json")
	if err != nil {
		t.Fatal(err)
	}

	// Both endpoints emitted under one document.
	var doc struct {
		TrainingID int             `json:"training_id"`
		Kind       string          `json:"kind"`
		Info       json.RawMessage `json:"info"`
		Detail     []struct {
			FinishedReps []struct {
				Detail map[string]json.RawMessage `json:"trainingInfoDetail"`
			} `json:"finishedReps"`
		} `json:"detail"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if doc.TrainingID != 940759 {
		t.Errorf("training_id = %d, want 940759", doc.TrainingID)
	}
	if doc.Kind != "program" {
		t.Errorf("kind = %q, want program (cttTrainingInfo answered)", doc.Kind)
	}
	if !strings.Contains(string(doc.Info), `"completionRate": 0.95`) {
		t.Errorf("info (cttTrainingInfo) not emitted verbatim — completionRate lost:\n%s", doc.Info)
	}

	// Every Speediance key flows through verbatim and unrenamed.
	for _, key := range []string{
		`"forceControlScore"`, `"bilateralBalanceScore"`, `"amplitudeStableScore"`,
		`"completionScore"`, `"actionRating"`, `"maxWeight"`, `"maxWeightCount"`,
		`"weights"`, `"leftWeights"`, `"rightWeights"`, `"leftWatts"`, `"rightWatts"`,
		`"leftAmplitudes"`, `"rightAmplitudes"`, `"leftRopeSpeeds"`,
		`"leftFinishedTimes"`, `"leftBreakTimes"`, `"leftTimestamps"`,
	} {
		if !strings.Contains(out, key) {
			t.Errorf("verbatim Speediance key %s missing from session --json", key)
		}
	}

	// No derived / renamed fields from the superseded telemetry design.
	for _, banned := range []string{
		"weight_source", "weight_avg_per_handle", "derived_avg", "reps_detail",
		"left_watts", "right_watts", "max_hr", "completion_rate", `"exercises"`,
	} {
		if strings.Contains(out, banned) {
			t.Errorf("session --json contains forbidden derived/renamed field %q:\n%s", banned, out)
		}
	}

	// The sparse set 2 keeps only what Speediance returned (weights) — no gap-fill.
	sparse := doc.Detail[0].FinishedReps[1].Detail
	if _, ok := sparse["weights"]; !ok || len(sparse) != 1 {
		t.Errorf("sparse set not preserved faithfully, want only weights, got %v", sparse)
	}

	// There is no --telemetry flag: requesting it is a usage error.
	if _, _, err := runCLI(t, srv.URL, "session", "940759", "--json", "--telemetry"); err == nil {
		t.Error("--telemetry should no longer exist (faithful-by-default), but the flag was accepted")
	}
}

// dispatchServer models the live namespace topology: program id 100 (type 5) and
// free id 200 (type 1, rowing) both dated 2026-06-18, plus collision id 300 that
// is a valid — but DIFFERENT — session in BOTH namespaces. Detail endpoints are
// keyed by id; unknown ids get code 20 (info) or [] (detail), matching the API.
func dispatchServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := time.Date(2026, 6, 18, 12, 0, 0, 0, time.Local).Unix()
	in := func(id string, set ...string) bool {
		for _, s := range set {
			if s == id {
				return true
			}
		}
		return false
	}
	programInfo := func(id string) string {
		return `{"code":0,"data":{"completionRate":100.0,"type":5,"name":"Program ` + id + `","trainingId":` + id + `}}`
	}
	programDetail := `{"code":0,"data":[{"actionLibraryName":"Row","forceControlScore":4,` +
		`"finishedReps":[{"finishedCount":12,"trainingInfoDetail":{"weights":[20,20],"leftBreakTimes":[1,0]}}]}]}`
	freeInfo := func(id string) string {
		return `{"code":0,"data":{"id":` + id + `,"type":1,"totalCapacity":0.0,"totalEnergy":65098.22,` +
			`"totalDistance":1505.5,"existBoatingSkiDataGraph":true,"trainingCount":15}}`
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		id := p[strings.LastIndex(p, "/")+1:]
		switch {
		case strings.Contains(p, "userTrainingDataRecord"):
			_, _ = fmt.Fprintf(w, `{"code":0,"data":[
				{"trainingId":100,"id":900,"title":"Pull Program","type":5,"startTimestamp":%d,"trainingTime":1731,"calorie":205,"totalCapacity":8380.0},
				{"trainingId":200,"id":901,"title":"Free Lift","type":1,"startTimestamp":%d,"trainingTime":904,"calorie":77,"totalCapacity":0.0}]}`, ts, ts)
		case strings.Contains(p, "cttTrainingInfoDetail"):
			if in(id, "100", "300") {
				_, _ = w.Write([]byte(programDetail))
			} else {
				_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
			}
		case strings.Contains(p, "cttTrainingInfo"):
			if in(id, "100", "300") {
				_, _ = w.Write([]byte(programInfo(id)))
			} else {
				_, _ = w.Write([]byte(`{"code":20,"message":"no program"}`))
			}
		case strings.Contains(p, "freeTrainingDetail"):
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		case strings.Contains(p, "freeTraining"):
			if in(id, "200", "300") {
				_, _ = w.Write([]byte(freeInfo(id)))
			} else {
				_, _ = w.Write([]byte(`{"code":20,"message":"no free"}`))
			}
		default:
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		}
	}))
}

type sessionDoc struct {
	TrainingID int             `json:"training_id"`
	Kind       string          `json:"kind"`
	Info       json.RawMessage `json:"info"`
	Detail     json.RawMessage `json:"detail"`
}

// TestSessionAutoDetectsFree: given only an id for a free (rowing) session, the
// tool probes program (empty), falls back to free, and returns kind=free with the
// freeTraining payload — no flag or type knowledge from the caller.
func TestSessionAutoDetectsFree(t *testing.T) {
	srv := dispatchServer(t)
	defer srv.Close()
	out, _, err := runCLI(t, srv.URL, "session", "200", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var doc sessionDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if doc.Kind != "free" {
		t.Errorf("kind = %q, want free (auto-detected)", doc.Kind)
	}
	if !strings.Contains(string(doc.Info), `"totalDistance"`) {
		t.Errorf("free info (freeTraining payload) not emitted:\n%s", doc.Info)
	}
	if strings.TrimSpace(string(doc.Detail)) != "[]" {
		t.Errorf("free detail = %s, want [] (freeTrainingDetail)", doc.Detail)
	}
}

// TestTodayResolvesEverySessionType: one call returns every session on the day,
// each resolved to the correct namespace by its list type — no caller knowledge.
func TestTodayResolvesEverySessionType(t *testing.T) {
	srv := dispatchServer(t)
	defer srv.Close()
	out, _, err := runCLI(t, srv.URL, "today", "--date", "2026-06-18", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var arr []sessionDoc
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if len(arr) != 2 {
		t.Fatalf("today returned %d sessions, want 2\n%s", len(arr), out)
	}
	got := map[int]string{}
	for _, s := range arr {
		got[s.TrainingID] = s.Kind
	}
	if got[100] != "program" {
		t.Errorf("session 100 kind = %q, want program", got[100])
	}
	if got[200] != "free" {
		t.Errorf("session 200 kind = %q, want free", got[200])
	}
}

// TestSessionCollisionOverride: when an id is valid in both namespaces, the
// default prefers program, and --free / --program force the intended one.
func TestSessionCollisionOverride(t *testing.T) {
	srv := dispatchServer(t)
	defer srv.Close()
	get := func(args ...string) sessionDoc {
		t.Helper()
		out, _, err := runCLI(t, srv.URL, append([]string{"session", "300", "--json"}, args...)...)
		if err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		var d sessionDoc
		if err := json.Unmarshal([]byte(out), &d); err != nil {
			t.Fatalf("bad json: %v\n%s", err, out)
		}
		return d
	}
	if d := get(); d.Kind != "program" {
		t.Errorf("default kind = %q, want program (program-first on collision)", d.Kind)
	}
	if d := get("--free"); d.Kind != "free" || !strings.Contains(string(d.Info), "totalDistance") {
		t.Errorf("--free did not force the free session: kind=%q info=%s", d.Kind, d.Info)
	}
	if d := get("--program"); d.Kind != "program" {
		t.Errorf("--program kind = %q, want program", d.Kind)
	}
}

// TestSessionMutuallyExclusiveFlags: --free and --program together is a usage error.
func TestSessionMutuallyExclusiveFlags(t *testing.T) {
	srv := dispatchServer(t)
	defer srv.Close()
	if _, _, err := runCLI(t, srv.URL, "session", "100", "--json", "--free", "--program"); err == nil {
		t.Error("--free --program together should be a usage error")
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
