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
	clearEnv(t) // ensures no real SPEEDIANCE_* var masks the .env-supplied value.
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

// TestDotEnvDoesNotInjectForeignEnv is the regression guard for the ClawHub
// "Privilege Escalation / Credential Access" finding. A .env in the working
// directory must influence ONLY the documented SPEEDIANCE_* settings; it must
// never be able to export an unrelated, security-sensitive variable (PATH,
// LD_PRELOAD, an arbitrary sentinel) into the live process environment.
//
// This pins the use of godotenv.Read (map, no global mutation) over
// godotenv.Load (exports every key). If someone reintroduces Load, the foreign
// keys below would appear in the process environment and this test fails.
func TestDotEnvDoesNotInjectForeignEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)

	// Record the pre-Load state of two real, dangerous knobs so we can prove
	// Load left them exactly as they were rather than adopting the .env values.
	const ldKey, pathProbe = "LD_PRELOAD", "DOTENV_FOREIGN_SENTINEL_SHOULD_BE_IGNORED"
	ldBefore, ldSet := os.LookupEnv(ldKey)

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(
		"SPEEDIANCE_EMAIL=ok@b.com\n"+
			"LD_PRELOAD=/tmp/evil.so\n"+
			"PATH=/evil/bin\n"+
			pathProbe+"=injected\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}

	// The documented feature still works: SPEEDIANCE_* keys are applied.
	if cfg.Email != "ok@b.com" {
		t.Errorf("email = %q, want ok@b.com (.env SPEEDIANCE_* must still load)", cfg.Email)
	}

	// The guard: none of the foreign keys leaked into the process environment.
	if v, ok := os.LookupEnv(pathProbe); ok {
		t.Errorf("foreign key %s leaked into process env as %q; .env must not mutate the environment", pathProbe, v)
	}
	if ldAfter, ldStillSet := os.LookupEnv(ldKey); ldStillSet != ldSet || ldAfter != ldBefore {
		t.Errorf("LD_PRELOAD changed by .env load: before=(%q,%v) after=(%q,%v)", ldBefore, ldSet, ldAfter, ldStillSet)
	}
}

// TestDotEnvNeverMutatesProcessEnv locks in the stronger invariant behind the
// fix: Load resolves .env values through a map and writes nothing back to the
// environment — not even for keys it DOES consume. A SPEEDIANCE_* value sourced
// from .env must reach the Config without ever appearing in os.Environ.
func TestDotEnvNeverMutatesProcessEnv(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("SPEEDIANCE_REGION=EU\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Region != "EU" {
		t.Fatalf("region = %q, want EU (.env value should be consumed)", cfg.Region)
	}
	// Value flowed via the map, so the real environment is still untouched.
	if v, ok := os.LookupEnv(EnvRegion); ok {
		t.Errorf("%s present in process env as %q after Load; .env consumption must not Setenv", EnvRegion, v)
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
