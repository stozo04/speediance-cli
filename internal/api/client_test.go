package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stozo04/speediance-cli/internal/auth"
)

func testClient(baseURL string, tok auth.Token) *Client {
	return New(Config{
		BaseURL:  baseURL,
		Email:    "user@example.com",
		Password: "secret",
		Token:    tok,
		Now:      func() time.Time { return time.Unix(1_700_000_000, 0) },
	})
}

func TestLoginTwoStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/v2/login/verifyIdentity":
			_, _ = w.Write([]byte(`{"code":0,"data":{"isExist":true,"hasPwd":true}}`))
		case "/app/v2/login/byPass":
			_, _ = w.Write([]byte(`{"code":0,"data":{"token":"TOK","appUserId":42}}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := testClient(srv.URL, auth.Token{})
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
	got := c.Token()
	if got.Token != "TOK" || got.UserID != "42" {
		t.Errorf("token = %+v, want {TOK 42}", got)
	}
}

func TestLoginAuthErrors(t *testing.T) {
	cases := []struct {
		name   string
		verify string
		bypass string
	}{
		{"verify-fails", `{"code":1,"message":"nope"}`, ``},
		{"not-exist", `{"code":0,"data":{"isExist":false}}`, ``},
		{"no-password", `{"code":0,"data":{"isExist":true,"hasPwd":false}}`, ``},
		{"bypass-fails", `{"code":0,"data":{"isExist":true,"hasPwd":true}}`, `{"code":1,"message":"bad pw"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/app/v2/login/verifyIdentity" {
					_, _ = w.Write([]byte(tc.verify))
					return
				}
				_, _ = w.Write([]byte(tc.bypass))
			}))
			defer srv.Close()

			c := testClient(srv.URL, auth.Token{})
			err := c.Login(context.Background())
			var ae *AuthError
			if !errors.As(err, &ae) {
				t.Fatalf("want AuthError, got %v", err)
			}
		})
	}
}

func TestCode91TriggersReLogin(t *testing.T) {
	var logins, dataCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app/v2/login/verifyIdentity":
			_, _ = w.Write([]byte(`{"code":0,"data":{"isExist":true,"hasPwd":true}}`))
		case "/app/v2/login/byPass":
			atomic.AddInt32(&logins, 1)
			_, _ = w.Write([]byte(`{"code":0,"data":{"token":"TOK","appUserId":7}}`))
		default:
			// First data call: expired (code 91); second: success.
			if atomic.AddInt32(&dataCalls, 1) == 1 {
				_, _ = w.Write([]byte(`{"code":91,"message":"expired"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"ok":true}}`))
		}
	}))
	defer srv.Close()

	// Start with a preset token so the first request skips login and hits 91.
	c := testClient(srv.URL, auth.Token{Token: "OLD", UserID: "7"})
	env, err := c.GetJSON(context.Background(), "/some/data")
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if env.Code != 0 {
		t.Errorf("final code = %d, want 0", env.Code)
	}
	if logins != 1 {
		t.Errorf("re-login count = %d, want 1", logins)
	}
	if dataCalls != 2 {
		t.Errorf("data calls = %d, want 2 (91 then retry)", dataCalls)
	}
}

func TestFrozenHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		got.Set("__host__", r.Host)
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer srv.Close()

	c := testClient(srv.URL, auth.Token{Token: "THE_TOKEN", UserID: "99"})
	if _, err := c.GetJSON(context.Background(), "/x"); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"User-Agent":      "Dart/3.9 (dart:io)",
		"Content-Type":    "application/json",
		"Utc_offset":      "+0000",
		"Timezone":        "GMT",
		"Versioncode":     "40304",
		"Accept-Language": "en",
		"App_type":        "SOFTWARE",
		"Mobiledevices":   mobileDevices,
		"Token":           "THE_TOKEN",
		"App_user_id":     "99",
	}
	for k, v := range want {
		if g := got.Get(k); g != v {
			t.Errorf("header %s = %q, want %q", k, g, v)
		}
	}
	// Timestamp is epoch ms (injected clock => 1700000000000).
	if got.Get("Timestamp") != "1700000000000" {
		t.Errorf("Timestamp = %q, want 1700000000000", got.Get("Timestamp"))
	}
}

func TestRegionBaseURL(t *testing.T) {
	cases := map[string]string{
		"Global":  "https://api2.speediance.com/api",
		"EU":      "https://euapi.speediance.com/api",
		"Unknown": "https://api2.speediance.com/api", // falls back to Global.
		"":        "https://api2.speediance.com/api",
	}
	for region, want := range cases {
		if got := baseURLFor(region); got != want {
			t.Errorf("baseURLFor(%q) = %q, want %q", region, got, want)
		}
	}
}

func TestFetchWorkoutsParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":[
			{"trainingId":123,"title":"Upper Body","courseTypeStr":"Strength",
			 "startTimestamp":1718400000,"trainingTime":2700,"calorie":320,"totalCapacity":4200.0}
		]}`))
	}))
	defer srv.Close()

	c := testClient(srv.URL, auth.Token{Token: "T", UserID: "1"})
	ws, err := c.FetchWorkouts(context.Background(), 3)
	if err != nil {
		t.Fatalf("FetchWorkouts: %v", err)
	}
	if len(ws) != 1 {
		t.Fatalf("got %d workouts, want 1", len(ws))
	}
	s := ws[0].Summary()
	if s.TrainingID != 123 || s.Title != "Upper Body" || s.Type != "Strength" {
		t.Errorf("summary basics wrong: %+v", s)
	}
	if s.DurationSecs != 2700 || s.Calories != 320 {
		t.Errorf("duration/calories wrong: %+v", s)
	}
	// volume must round-trip the float form verbatim.
	if s.Volume != json.Number("4200.0") {
		t.Errorf("volume = %q, want 4200.0", s.Volume)
	}
	wantDate := time.Unix(1718400000, 0).Format("2006-01-02")
	if s.Date == nil || *s.Date != wantDate {
		t.Errorf("date = %v, want %s", s.Date, wantDate)
	}
}
