package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // no config.json present, no env.
	clearEnv(t)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Region != "Global" || cfg.DeviceType != 1 {
		t.Errorf("defaults wrong: %+v", cfg)
	}
	if cfg.TokenCachePath != ".token.json" {
		t.Errorf("token cache = %q, want .token.json", cfg.TokenCachePath)
	}
}

func TestFileOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)
	writeConfig(t, dir, `{"email":"a@b.com","password":"pw","region":"EU","device_type":2}`)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email != "a@b.com" || cfg.Region != "EU" || cfg.DeviceType != 2 {
		t.Errorf("file not applied: %+v", cfg)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)
	writeConfig(t, dir, `{"email":"file@b.com","region":"EU","device_type":2}`)
	t.Setenv(EnvEmail, "env@b.com")
	t.Setenv(EnvRegion, "Global")
	t.Setenv(EnvDeviceType, "5")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email != "env@b.com" {
		t.Errorf("email = %q, want env@b.com", cfg.Email)
	}
	if cfg.Region != "Global" {
		t.Errorf("region = %q, want Global", cfg.Region)
	}
	if cfg.DeviceType != 5 {
		t.Errorf("device_type = %d, want 5", cfg.DeviceType)
	}
}

func TestExplicitConfigPathWins(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)
	custom := filepath.Join(dir, "custom.json")
	if err := os.WriteFile(custom, []byte(`{"email":"custom@b.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Options{ConfigPath: custom})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email != "custom@b.com" {
		t.Errorf("explicit --config not used: %q", cfg.Email)
	}
	if cfg.ConfigPath != custom {
		t.Errorf("config path = %q, want %q", cfg.ConfigPath, custom)
	}
}

func TestRequireCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)

	cfg, _ := Load(Options{})
	if err := cfg.RequireCredentials(); err == nil {
		t.Error("expected missing-credentials error")
	}
	cfg.Email, cfg.Password = "a", "b"
	if err := cfg.RequireCredentials(); err != nil {
		t.Errorf("unexpected error with creds set: %v", err)
	}
}

func TestDotEnvLoadedAsEnvLayer(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t) // registers SPEEDIANCE_* for restoration before godotenv sets them.
	// A .env in CWD is auto-loaded and beats config.json (it's the env layer).
	writeConfig(t, dir, `{"email":"file@b.com","region":"EU"}`)
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("SPEEDIANCE_EMAIL=dotenv@b.com\nSPEEDIANCE_REGION=Global\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email != "dotenv@b.com" {
		t.Errorf("email = %q, want dotenv@b.com (.env over file)", cfg.Email)
	}
	if cfg.Region != "Global" {
		t.Errorf("region = %q, want Global (.env over file)", cfg.Region)
	}
}

func TestRealEnvBeatsDotEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("SPEEDIANCE_EMAIL=dotenv@b.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvEmail, "real@b.com") // a real exported var must win over .env.

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Email != "real@b.com" {
		t.Errorf("email = %q, want real@b.com (exported env over .env)", cfg.Email)
	}
}

// clearEnv unsets all SPEEDIANCE_* vars for a clean, isolated test (t.Setenv
// restores them afterward).
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvEmail, EnvPassword, EnvRegion, EnvDeviceType,
		EnvConfig, EnvTokenCache,
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}
