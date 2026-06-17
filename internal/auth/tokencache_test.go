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
