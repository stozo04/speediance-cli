package template_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stozo04/speediance-cli/v2/internal/template"
)

// vitaReleaseVersionCode is the app release (4.4.0) that the live API requires
// before it lists Vita exercises; below it they are dropped with code 0.
const vitaReleaseVersionCode = 40400

// versionGatedLibraryServer serves a one-tab catalog that, like the live API,
// silently omits the Vita exercise when the client's Versioncode is too old.
func versionGatedLibraryServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/actionLibraryTab/list"):
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":1,"name":"Training","isCustom":0}]}`))
		case strings.HasSuffix(r.URL.Path, "/actionLibraryGroup/trainingPartGroup"):
			actions := []string{`{"id":116,"title":"Barbell Bench Press"}`}
			if v, err := strconv.Atoi(r.Header.Get("Versioncode")); err == nil && v >= vitaReleaseVersionCode {
				actions = append(actions, `{"id":455213218267137,"title":"Vita Twist"}`)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[{"actionLibraryGroupList":[` + strings.Join(actions, ",") + `]}]}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
		}
	}))
}

func TestFetchLibraryIncludesVersionGatedExercises(t *testing.T) {
	srv := versionGatedLibraryServer(t)
	defer srv.Close()

	got, err := template.FetchLibrary(context.Background(), newTestClient(t, srv.URL), 1)
	if err != nil {
		t.Fatalf("FetchLibrary: %v", err)
	}
	names := make([]string, len(got))
	for i, ex := range got {
		names[i] = ex.Name
	}
	if len(got) != 2 || got[1].Name != "Vita Twist" {
		t.Errorf("catalog = %v, want Barbell Bench Press and Vita Twist (Versioncode below %d hides Vita)",
			names, vitaReleaseVersionCode)
	}
}
