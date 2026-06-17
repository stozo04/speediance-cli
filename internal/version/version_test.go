package version

import (
	"strings"
	"testing"
)

func TestSetOverridesDefaults(t *testing.T) {
	// Save and restore package state so tests stay independent.
	t.Cleanup(func() { version, commit, date = "dev", "none", "unknown" })

	Set("v1.2.3", "abc1234", "2026-06-16")
	got := Info()
	if got.Version != "v1.2.3" || got.Commit != "abc1234" || got.Date != "2026-06-16" {
		t.Fatalf("Set not applied: %+v", got)
	}
	if got.Go == "" {
		t.Fatal("Go runtime version should never be empty")
	}
}

func TestSetIgnoresEmptyArgs(t *testing.T) {
	t.Cleanup(func() { version, commit, date = "dev", "none", "unknown" })

	Set("v9", "", "")
	got := Info()
	if got.Version != "v9" {
		t.Fatalf("version override lost: %q", got.Version)
	}
	// Empty commit/date must not blank the existing values.
	if got.Commit == "" || got.Date == "" {
		t.Fatalf("empty args clobbered fields: %+v", got)
	}
}

func TestStringIncludesAllFields(t *testing.T) {
	b := Build{Version: "v1", Commit: "c1", Date: "d1", Go: "go1.x"}
	s := b.String()
	for _, want := range []string{"v1", "c1", "d1", "go1.x"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() %q missing %q", s, want)
		}
	}
}
