package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	if want := defaultTokenCachePath(); cfg.TokenCachePath != want {
		t.Errorf("token cache = %q, want %q (per-user default)", cfg.TokenCachePath, want)
	}
	if !cfg.TokenCacheIsDefault {
		t.Error("TokenCacheIsDefault = false, want true when no override is set")
	}
}

// setUserBaseDirs points BOTH os.UserConfigDir and os.UserCacheDir at dir on
// every platform, so the per-user defaults are deterministic in tests. The token
// default uses the cache base (CLI_CONVENTIONS.md §1); config uses the config
// base. UserConfigDir reads %AppData% / $XDG_CONFIG_HOME / $HOME/Library/Application Support;
// UserCacheDir reads %LocalAppData% / $XDG_CACHE_HOME / $HOME/Library/Caches —
// setting all of them (plus $HOME for macOS, which derives both) covers each.
func setUserBaseDirs(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("AppData", dir)         // Windows UserConfigDir
	t.Setenv("LocalAppData", dir)    // Windows UserCacheDir
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux/BSD UserConfigDir
	t.Setenv("XDG_CACHE_HOME", dir)  // Linux/BSD UserCacheDir
	t.Setenv("HOME", dir)            // macOS derives both; Linux fallback
}

// TestTokenCacheDefaultNotInWorkingDir is the immutable regression guard for
// issue #17: with no override, the token cache must NOT resolve inside the
// current working directory, where a routine `git add -A` would commit the live
// session token. Restoring the old CWD ".token.json" default fails this.
func TestTokenCacheDefaultNotInWorkingDir(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	clearEnv(t)
	setUserBaseDirs(t, t.TempDir()) // a per-user dir distinct from cwd.

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.TokenCacheIsDefault {
		t.Fatal("TokenCacheIsDefault = false, want true for the built-in default")
	}
	if cfg.TokenCachePath == legacyTokenName {
		t.Fatalf("token cache defaults to %q in the working directory — issue #17 leak", cfg.TokenCachePath)
	}
	if !filepath.IsAbs(cfg.TokenCachePath) {
		t.Fatalf("default token cache %q is not absolute; it must live in a per-user dir, not CWD", cfg.TokenCachePath)
	}
	if base := filepath.Base(cfg.TokenCachePath); base != userTokenName {
		t.Errorf("token cache filename = %q, want %q", base, userTokenName)
	}
	if parent := filepath.Base(filepath.Dir(cfg.TokenCachePath)); parent != appUserSubdir {
		t.Errorf("token cache parent dir = %q, want %q", parent, appUserSubdir)
	}
	// Prove it is not nested under the working directory at all.
	if rel, err := filepath.Rel(cwd, cfg.TokenCachePath); err == nil &&
		rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("default token cache %q resolves inside the working directory %q — issue #17 leak", cfg.TokenCachePath, cwd)
	}
}

// TestTokenCacheDefaultIsNotRoaming is the regression guard for CLI_CONVENTIONS.md
// §1: the token default must resolve under the non-roaming cache base
// (os.UserCacheDir → %LocalAppData% on Windows), NOT the config base
// (os.UserConfigDir → %AppData%\Roaming), which can sync a live credential across
// machines. Asserting the negative — token is NOT under the roaming config base —
// fails loudly if anyone moves the default back to os.UserConfigDir.
func TestTokenCacheDefaultIsNotRoaming(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	clearEnv(t)
	// Point the config base and the cache base at DISTINCT dirs so "under cache,
	// not under config" is a meaningful, hermetic assertion on every platform.
	configDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("AppData", configDir)         // Windows UserConfigDir
	t.Setenv("XDG_CONFIG_HOME", configDir) // Linux/BSD UserConfigDir
	t.Setenv("LocalAppData", cacheDir)     // Windows UserCacheDir
	t.Setenv("XDG_CACHE_HOME", cacheDir)   // Linux/BSD UserCacheDir
	t.Setenv("HOME", cacheDir)             // macOS derives both (distinct subdirs)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}

	cacheBase, errCache := os.UserCacheDir()
	configBase, errConfig := os.UserConfigDir()
	if errCache != nil || errConfig != nil {
		t.Skip("per-user base dirs unavailable in this environment")
	}
	sep := string(filepath.Separator)
	if !strings.HasPrefix(cfg.TokenCachePath, cacheBase+sep) {
		t.Errorf("token default %q is not under the cache base %q (CLI_CONVENTIONS.md §1)", cfg.TokenCachePath, cacheBase)
	}
	if strings.HasPrefix(cfg.TokenCachePath, configBase+sep) {
		t.Errorf("token default %q is under the ROAMING config base %q; it must use the non-roaming cache base (CLI_CONVENTIONS.md §1)", cfg.TokenCachePath, configBase)
	}
}

// TestTokenCacheEnvOverridesDefault: SPEEDIANCE_TOKEN_CACHE wins and is taken
// verbatim, and such an explicit choice is never flagged as the default (so it
// never triggers legacy migration).
func TestTokenCacheEnvOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)
	custom := filepath.Join(dir, "explicit-token.json")
	t.Setenv(EnvTokenCache, custom)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenCachePath != custom {
		t.Errorf("token cache = %q, want %q (env override)", cfg.TokenCachePath, custom)
	}
	if cfg.TokenCacheIsDefault {
		t.Error("TokenCacheIsDefault = true for an explicit override; want false")
	}
}

// TestTokenCacheConfigKeyHonored is the regression guard for issue #17's
// secondary bug: token_cache_path in config.json was printed by `config show`
// but silently ignored by the loader. It must now be applied.
func TestTokenCacheConfigKeyHonored(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)
	want := filepath.Join(dir, "from-config.json")
	writeConfig(t, dir, `{"token_cache_path":`+strconv.Quote(want)+`}`)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenCachePath != want {
		t.Errorf("token cache = %q, want %q (token_cache_path config key must be honored)", cfg.TokenCachePath, want)
	}
	if cfg.TokenCacheIsDefault {
		t.Error("TokenCacheIsDefault = true with token_cache_path set; want false")
	}
}

// TestTokenCacheEnvBeatsConfigKey: env layer outranks the config-file key, per
// the global precedence contract (flags > env > .env > file > defaults).
func TestTokenCacheEnvBeatsConfigKey(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	clearEnv(t)
	fromFile := filepath.Join(dir, "from-file.json")
	fromEnv := filepath.Join(dir, "from-env.json")
	writeConfig(t, dir, `{"token_cache_path":`+strconv.Quote(fromFile)+`}`)
	t.Setenv(EnvTokenCache, fromEnv)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenCachePath != fromEnv {
		t.Errorf("token cache = %q, want %q (env must beat config key)", cfg.TokenCachePath, fromEnv)
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
