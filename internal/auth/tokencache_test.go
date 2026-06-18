package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".token.json")
	want := Token{Token: "abc123", UserID: "42"}
	if err := Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok || got != want {
		t.Errorf("round-trip = %+v ok=%v, want %+v", got, ok, want)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, ok, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || ok {
		t.Errorf("missing file: ok=%v err=%v, want false,nil", ok, err)
	}
}

func TestLoadCorruptFileIsNotFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".token.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, ok, err := Load(path)
	if err != nil || ok || !tok.Empty() {
		t.Errorf("corrupt file: tok=%+v ok=%v err=%v, want empty,false,nil", tok, ok, err)
	}
}

// TestMigrateLegacyMovesToken is the regression guard for issue #17: a token
// cached in the working-directory legacy path is relocated to the per-user
// location AND removed from the working directory (so it can't be committed).
func TestMigrateLegacyMovesToken(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".token.json")
	resolved := filepath.Join(dir, "userdir", "token.json")
	want := Token{Token: "abc123", UserID: "7"}
	if err := Save(legacy, want); err != nil {
		t.Fatal(err)
	}

	migrated, err := MigrateLegacy(resolved, legacy)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !migrated {
		t.Fatal("migrated = false, want true")
	}
	// The credential must no longer sit in the (committable) legacy location.
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy token still present after migration (stat err = %v); the leak must be removed", err)
	}
	got, ok, err := Load(resolved)
	if err != nil || !ok || got != want {
		t.Errorf("relocated token = %+v ok=%v err=%v, want %+v", got, ok, err, want)
	}
}

func TestMigrateLegacyNoLegacyFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	migrated, err := MigrateLegacy(filepath.Join(dir, "token.json"), filepath.Join(dir, ".token.json"))
	if err != nil || migrated {
		t.Errorf("missing legacy file: migrated=%v err=%v, want false,nil", migrated, err)
	}
}

// TestMigrateLegacyDoesNotClobberExisting: a token already at the destination is
// never overwritten, and the legacy file is left untouched in that case.
func TestMigrateLegacyDoesNotClobberExisting(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, ".token.json")
	resolved := filepath.Join(dir, "token.json")
	if err := Save(legacy, Token{Token: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := Save(resolved, Token{Token: "new"}); err != nil {
		t.Fatal(err)
	}

	migrated, err := MigrateLegacy(resolved, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if migrated {
		t.Error("migrated = true; must not migrate over an existing destination")
	}
	got, _, _ := Load(resolved)
	if got.Token != "new" {
		t.Errorf("destination clobbered: %+v", got)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Errorf("legacy file should be left as-is when destination exists: %v", err)
	}
}

func TestMigrateLegacySamePathIsNoop(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".token.json")
	if err := Save(p, Token{Token: "x"}); err != nil {
		t.Fatal(err)
	}
	migrated, err := MigrateLegacy(p, p)
	if err != nil || migrated {
		t.Errorf("same path: migrated=%v err=%v, want false,nil", migrated, err)
	}
}

func TestSavePermsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are advisory on Windows")
	}
	path := filepath.Join(t.TempDir(), ".token.json")
	if err := Save(path, Token{Token: "x"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}
}
