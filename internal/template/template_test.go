package template_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stozo04/speediance-cli/internal/api"
	"github.com/stozo04/speediance-cli/internal/auth"
	"github.com/stozo04/speediance-cli/internal/template"
)

// fakeServer mirrors the rules used to generate the golden payloads
// (testdata/golden/*.payload.json), so byte-equality is a meaningful check:
//   - variant id = group_id * 10
//   - unilateral = (group_id % 2 == 1)
func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	idsRe := regexp.MustCompile(`ids=(\d+)`)
	uniRe := regexp.MustCompile(`actionLibraryGroup/(\d+)\?isDisplay=1`)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		full := r.URL.Path
		if r.URL.RawQuery != "" {
			full += "?" + r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(full, "actionLibraryGroup/list?"):
			matches := idsRe.FindAllStringSubmatch(r.URL.RawQuery, -1)
			var rows []string
			for _, m := range matches {
				id := m[1]
				v, _ := strconv.ParseInt(id, 10, 64)
				rows = append(rows, `{"id":`+id+`,"actionLibraryList":[{"id":`+strconv.FormatInt(v*10, 10)+`}]}`)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[` + strings.Join(rows, ",") + `]}`))
		case uniRe.MatchString(full):
			m := uniRe.FindStringSubmatch(full)
			gid, _ := strconv.ParseInt(m[1], 10, 64)
			lr := 0
			if gid%2 == 1 {
				lr = 1
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"isLeftRight":` + strconv.Itoa(lr) + `}}`))
		case strings.Contains(full, "customTrainingTemplate"):
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":999,"name":"x"}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		}
	}))
}

func newTestClient(t *testing.T, baseURL string) *api.Client {
	t.Helper()
	return api.New(api.Config{
		BaseURL: baseURL,
		Email:   "e", Password: "p",
		Token: auth.Token{Token: "tok", UserID: "1"}, // preset so no login happens.
	})
}

// marshalLikeCLI reproduces the cli.writeJSON encoder (HTML escaping off,
// two-space indent, trailing newline) so the bytes match `push --dry-run --json`
// stdout exactly.
func marshalLikeCLI(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestBuildPayloadGoldenParity(t *testing.T) {
	srv := fakeServer(t)
	defer srv.Close()

	cases := []string{"example-push", "week-01-legs"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			planPath := filepath.Join("..", "..", "testdata", "plans", name+".json")
			plan, err := template.LoadPlan(planPath)
			if err != nil {
				t.Fatalf("load plan: %v", err)
			}
			client := newTestClient(t, srv.URL)
			payload, err := template.BuildPayload(context.Background(), client, plan.Name, plan.Exercises, 1)
			if err != nil {
				t.Fatalf("build payload: %v", err)
			}
			got := marshalLikeCLI(t, payload)

			goldenPath := filepath.Join("..", "..", "testdata", "golden", name+".payload.json")
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("payload for %s is not byte-identical to the Python golden.\n--- got ---\n%s\n--- want ---\n%s",
					name, got, want)
			}
		})
	}
}

// TestBuildPayloadUnits ports tests/test_templates.py's assertions: the kg->API
// weight conversion and CSV encodings.
func TestBuildPayloadUnits(t *testing.T) {
	srv := fakeServer(t)
	defer srv.Close()

	mode, rest := 1, 75
	plan := []template.PlanExercise{{
		ID: "304",
		Sets: []template.PlanSet{
			{Reps: 15, Weight: 20, Mode: &mode, Rest: &rest},
			{Reps: 12, Weight: 22.5, Mode: &mode, Rest: &rest},
		},
	}}
	client := newTestClient(t, srv.URL)
	payload, err := template.BuildPayload(context.Background(), client, "Test Push", plan, 1)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}
	a0 := payload.ActionLibraryList[0]
	if a0.GroupID != 304 || a0.ActionLibraryID != 3040 {
		t.Errorf("ids: got groupId=%d actionLibraryId=%d", a0.GroupID, a0.ActionLibraryID)
	}
	if a0.SetsAndReps != "15,12" {
		t.Errorf("setsAndReps=%q want 15,12", a0.SetsAndReps)
	}
	if a0.Weights != "44.0,49.5" { // kg * 2.2
		t.Errorf("weights=%q want 44.0,49.5", a0.Weights)
	}
	if got := len(strings.Split(a0.Weights, ",")); got != len(strings.Split(a0.SetsAndReps, ",")) {
		t.Errorf("weights/reps length mismatch")
	}
	if a0.LeftRight != "0,0" { // id 304 is even => bilateral
		t.Errorf("leftRight=%q want 0,0", a0.LeftRight)
	}
}

func TestLoadPlanRequiresNameAndExercises(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"name":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := template.LoadPlan(bad); err == nil {
		t.Error("expected error for plan missing 'exercises'")
	}
}
