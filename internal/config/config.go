// Package config resolves runtime configuration and credential discovery.
//
// Precedence (GOAL.md §7), lowest to highest: built-in defaults < config.json <
// environment variables < command flags. The names of every env var and every
// JSON key are part of the external contract and must not be renamed — existing
// users and the ClawHub skill depend on them.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// Environment variable names (frozen — GOAL.md §7).
const (
	EnvEmail      = "SPEEDIANCE_EMAIL"
	EnvPassword   = "SPEEDIANCE_PASSWORD"
	EnvRegion     = "SPEEDIANCE_REGION"
	EnvDeviceType = "SPEEDIANCE_DEVICE_TYPE"
	EnvConfig     = "SPEEDIANCE_CONFIG"
	EnvTokenCache = "SPEEDIANCE_TOKEN_CACHE"
)

// Defaults (GOAL.md §7).
const (
	DefaultRegion     = "Global"
	DefaultDeviceType = 1 // Gym Monster 1 — the only tested device.

	defaultConfigName = "config.json"

	// appUserSubdir is the per-user application directory name, shared by both
	// per-user locations but placed under the purpose-appropriate base: the token
	// cache under os.UserCacheDir (non-roaming), config under os.UserConfigDir.
	// Either way it keeps state out of the — frequently version-controlled —
	// working directory. (CLI_CONVENTIONS.md §1, §2.)
	appUserSubdir = "speediance"
	// userTokenName is the token cache filename inside the per-user app dir.
	userTokenName = "token.json"
	// legacyTokenName is the pre-#17 default: a ".token.json" dropped in the
	// current working directory. That default leaked a live credential into
	// whatever repo the tool ran from (a stray `git add -A` could commit it), so
	// it is no longer the default — it is kept only so an existing cache there
	// can be migrated up to the per-user location instead of forcing a re-login.
	legacyTokenName = ".token.json"
)

// ErrMissingCredentials is returned by RequireCredentials when email or
// password could not be resolved from any source.
var ErrMissingCredentials = errors.New("missing credentials")

// Config is the fully resolved configuration handed to commands.
type Config struct {
	Email      string
	Password   string
	Region     string
	DeviceType int

	// ConfigPath is where config.json was loaded from, or where it would be
	// written if it does not yet exist.
	ConfigPath string
	// ConfigExists reports whether ConfigPath was present on disk at load time.
	ConfigExists bool
	// TokenCachePath is the resolved location of the session token cache.
	TokenCachePath string
	// TokenCacheIsDefault reports that TokenCachePath came from the built-in
	// per-user default rather than an explicit override (SPEEDIANCE_TOKEN_CACHE
	// or the token_cache_path config key). It guards the one-time migration of a
	// legacy working-directory .token.json (issue #17): an explicit override is
	// always honored verbatim and never triggers migration.
	TokenCacheIsDefault bool
}

// fileConfig mirrors config.json. Pointer fields distinguish "key present" from
// "key absent" so an absent key falls through to the default rather than
// overwriting it with a zero value.
type fileConfig struct {
	Email      *string `json:"email"`
	Password   *string `json:"password"`
	Region     *string `json:"region"`
	DeviceType *int    `json:"device_type"`
	// TokenCachePath maps the token_cache_path key. It is resolved separately
	// from applyFile (in Load) because the token cache obeys env-over-file
	// precedence; previously this key was advertised by `config show` but
	// silently ignored by the loader (issue #17).
	TokenCachePath *string `json:"token_cache_path"`
}

// Options carries inputs the caller already knows from flags, so config
// resolution can honor flag precedence without importing cobra.
type Options struct {
	// ConfigPath is the value of the --config flag (empty if not set). It wins
	// over SPEEDIANCE_CONFIG when choosing which file to read.
	ConfigPath string
}

// Load resolves configuration from defaults, config.json, and environment
// variables. Per-command flag overrides, when present, are applied by the
// caller afterward, since those flags are not known here.
func Load(opts Options) (*Config, error) {
	// Parse a .env file from the working directory WITHOUT mutating the process
	// environment, then consult only our own SPEEDIANCE_* keys (see envLayer).
	//
	// This is deliberate. godotenv.Load — the obvious alternative — exports EVERY
	// key in .env into the live process environment, including unrelated and
	// security-sensitive ones (PATH, LD_PRELOAD, HTTP_PROXY, …). Because a CLI's
	// working directory is attacker-influenceable (an agent may run it anywhere),
	// a stray or hostile .env could escalate into the tool's runtime. godotenv.Read
	// returns the file as a map and changes no global state, so unknown keys are
	// never applied. The documented feature is preserved: SPEEDIANCE_* values in
	// .env still act as the env layer. A missing/unreadable .env yields a nil map
	// and is a silent no-op.
	dotenv, _ := godotenv.Read()

	cfg := &Config{
		Region:     DefaultRegion,
		DeviceType: DefaultDeviceType,
	}

	// 1. Locate and read config.json (if any).
	path, err := discoverConfigPath(opts.ConfigPath, dotenv)
	if err != nil {
		return nil, err
	}
	cfg.ConfigPath = path

	fc, exists, err := readFileConfig(path)
	if err != nil {
		return nil, err
	}
	cfg.ConfigExists = exists
	applyFile(cfg, fc)

	// 2. Environment overrides. A real exported var beats a .env entry, which in
	// turn beats config.json — precedence stays flags > env > .env > file >
	// defaults. (envLayer distinguishes set-empty from unset, like LookupEnv.)
	applyEnv(cfg, dotenv)

	// 3. Token cache location. Precedence mirrors the global contract: an explicit
	// SPEEDIANCE_TOKEN_CACHE (real env or .env) wins, then the token_cache_path
	// config key, then the per-user default. The default lives in the non-roaming
	// per-user cache dir, OUTSIDE the working directory: a token cached in CWD is a
	// live credential a routine `git add -A` could commit (issue #17), and a token
	// in the roaming config dir can sync across machines (CLI_CONVENTIONS.md §1).
	if v, ok := envLayer(dotenv, EnvTokenCache); ok {
		cfg.TokenCachePath = v
	} else if fc.TokenCachePath != nil && *fc.TokenCachePath != "" {
		cfg.TokenCachePath = *fc.TokenCachePath
	} else {
		cfg.TokenCachePath = defaultTokenCachePath()
		cfg.TokenCacheIsDefault = true
	}

	return cfg, nil
}

// defaultTokenCachePath returns the per-user token cache location,
// <os.UserCacheDir>/speediance/token.json. The cache base is deliberate: a token
// is regenerable state, not config, and os.UserCacheDir is non-roaming
// (Windows %LocalAppData%) — unlike os.UserConfigDir (%AppData%\Roaming), which
// can sync a live credential across machines (CLI_CONVENTIONS.md §1). It also
// resolves OUTSIDE the working directory so the credential can't be swept into a
// commit (issue #17). If the cache base can't be determined (a rare, HOME-less
// environment), it falls back to the legacy working-directory path so the tool
// still functions.
func defaultTokenCachePath() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, appUserSubdir, userTokenName)
	}
	return legacyTokenName
}

// LegacyTokenCachePath is the pre-#17 default token cache location (.token.json
// in the working directory). It is exported only so the CLI can find and migrate
// an existing cache there up to the per-user default.
func LegacyTokenCachePath() string { return legacyTokenName }

// discoverConfigPath implements GOAL.md §7 file discovery:
//  1. --config flag or SPEEDIANCE_CONFIG env (explicit path; flag wins).
//  2. config.json in the current working directory (default, preserves today's
//     behavior and the skill docs).
//  3. optional fallback: <UserConfigDir>/speediance/config.json, but only if it
//     actually exists — never required.
func discoverConfigPath(flagPath string, dotenv map[string]string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	if v, ok := envLayer(dotenv, EnvConfig); ok && v != "" {
		return v, nil
	}

	if _, err := os.Stat(defaultConfigName); err == nil {
		return defaultConfigName, nil
	}

	// Optional modern fallback: only adopt it if the file is actually there, so
	// the CWD default stays authoritative for existing agents.
	if dir, err := os.UserConfigDir(); err == nil {
		alt := filepath.Join(dir, appUserSubdir, defaultConfigName)
		if _, err := os.Stat(alt); err == nil {
			return alt, nil
		}
	}

	// Nothing on disk yet: keep the CWD path so `config set` writes there.
	return defaultConfigName, nil
}

func readFileConfig(path string) (fileConfig, bool, error) {
	var fc fileConfig
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fc, false, nil
		}
		return fc, false, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return fc, true, fmt.Errorf("parse config %s: %w", path, err)
	}
	return fc, true, nil
}

func applyFile(cfg *Config, fc fileConfig) {
	if fc.Email != nil {
		cfg.Email = *fc.Email
	}
	if fc.Password != nil {
		cfg.Password = *fc.Password
	}
	if fc.Region != nil {
		cfg.Region = *fc.Region
	}
	if fc.DeviceType != nil {
		cfg.DeviceType = *fc.DeviceType
	}
}

func applyEnv(cfg *Config, dotenv map[string]string) {
	if v, ok := envLayer(dotenv, EnvEmail); ok {
		cfg.Email = v
	}
	if v, ok := envLayer(dotenv, EnvPassword); ok {
		cfg.Password = v
	}
	if v, ok := envLayer(dotenv, EnvRegion); ok {
		cfg.Region = v
	}
	if v, ok := envLayer(dotenv, EnvDeviceType); ok {
		// Parse leniently: a malformed value falls back to the current value
		// rather than failing the whole command, mirroring the Python tool's
		// permissiveness.
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DeviceType = n
		}
	}
}

// envLayer resolves one SPEEDIANCE_* setting from the environment layer: a real
// exported variable wins; otherwise the value parsed from .env (if any) is used.
// Only the explicit key passed in is ever consulted, so foreign keys present in
// a .env file (PATH, LD_PRELOAD, …) are never read and can never reach the
// process — this is the privilege-escalation guard. Like os.LookupEnv, a key
// that is set-but-empty returns ("", true) and still overrides lower layers.
func envLayer(dotenv map[string]string, key string) (string, bool) {
	if v, ok := os.LookupEnv(key); ok {
		return v, true
	}
	if v, ok := dotenv[key]; ok {
		return v, true
	}
	return "", false
}

// RequireCredentials returns a friendly error (to be shown on stderr) when
// email or password is missing after resolution. GOAL.md §7.
func (c *Config) RequireCredentials() error {
	if c.Email != "" && c.Password != "" {
		return nil
	}
	// Name where we looked, in precedence order, so the caller can fix it without
	// guessing — never a bare "missing credentials" (CLI_CONVENTIONS.md §5). No
	// secret values are echoed.
	return fmt.Errorf(
		"%w: set %s and %s (exported env vars, or a .env in the working directory), "+
			"or add \"email\"/\"password\" to config.json. config.json is resolved from "+
			"--config / %s, then ./%s, then <user-config-dir>/%s/%s (resolved this run: %s)",
		ErrMissingCredentials, EnvEmail, EnvPassword,
		EnvConfig, defaultConfigName, appUserSubdir, defaultConfigName, c.ConfigPath)
}

// DeviceWarning returns a non-empty warning string when a non-GM1 device is
// configured. GM2 (device_type 2) is untested; the warning is carried forward
// verbatim from the Python tool (GOAL.md §3, §7).
func (c *Config) DeviceWarning() string {
	if c.DeviceType != DefaultDeviceType {
		return fmt.Sprintf("warning: device_type=%d is untested (only Gym Monster 1 / device_type=1 is verified)", c.DeviceType)
	}
	return ""
}
