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
	defaultTokenName  = ".token.json"
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
	// TokenCachePath is the resolved location of .token.json.
	TokenCachePath string
}

// fileConfig mirrors config.json. Pointer fields distinguish "key present" from
// "key absent" so an absent key falls through to the default rather than
// overwriting it with a zero value.
type fileConfig struct {
	Email      *string `json:"email"`
	Password   *string `json:"password"`
	Region     *string `json:"region"`
	DeviceType *int    `json:"device_type"`
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
	// Load a .env file from the working directory (if present) into the process
	// environment so SPEEDIANCE_* values in .env participate as the env layer.
	// godotenv.Load does NOT override already-set variables, so a real exported
	// env var still wins — precedence stays flags > env (.env) > config file >
	// defaults. A missing .env is a silent no-op.
	_ = godotenv.Load()

	cfg := &Config{
		Region:     DefaultRegion,
		DeviceType: DefaultDeviceType,
	}

	// 1. Locate and read config.json (if any).
	path, err := discoverConfigPath(opts.ConfigPath)
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

	// 2. Environment overrides (LookupEnv distinguishes set-empty from unset).
	applyEnv(cfg)

	// 3. Token cache location.
	if v, ok := os.LookupEnv(EnvTokenCache); ok {
		cfg.TokenCachePath = v
	} else {
		cfg.TokenCachePath = defaultTokenName // .token.json in CWD (Python parity).
	}

	return cfg, nil
}

// discoverConfigPath implements GOAL.md §7 file discovery:
//  1. --config flag or SPEEDIANCE_CONFIG env (explicit path; flag wins).
//  2. config.json in the current working directory (default, preserves today's
//     behavior and the skill docs).
//  3. optional fallback: <UserConfigDir>/speediance/config.json, but only if it
//     actually exists — never required.
func discoverConfigPath(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	if v, ok := os.LookupEnv(EnvConfig); ok && v != "" {
		return v, nil
	}

	if _, err := os.Stat(defaultConfigName); err == nil {
		return defaultConfigName, nil
	}

	// Optional modern fallback: only adopt it if the file is actually there, so
	// the CWD default stays authoritative for existing agents.
	if dir, err := os.UserConfigDir(); err == nil {
		alt := filepath.Join(dir, "speediance", defaultConfigName)
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

func applyEnv(cfg *Config) {
	if v, ok := os.LookupEnv(EnvEmail); ok {
		cfg.Email = v
	}
	if v, ok := os.LookupEnv(EnvPassword); ok {
		cfg.Password = v
	}
	if v, ok := os.LookupEnv(EnvRegion); ok {
		cfg.Region = v
	}
	if v, ok := os.LookupEnv(EnvDeviceType); ok {
		// Parse leniently: a malformed value falls back to the current value
		// rather than failing the whole command, mirroring the Python tool's
		// permissiveness.
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DeviceType = n
		}
	}
}

// RequireCredentials returns a friendly error (to be shown on stderr) when
// email or password is missing after resolution. GOAL.md §7.
func (c *Config) RequireCredentials() error {
	if c.Email == "" || c.Password == "" {
		return fmt.Errorf("%w: set %s and %s (or add \"email\"/\"password\" to %s)",
			ErrMissingCredentials, EnvEmail, EnvPassword, c.ConfigPath)
	}
	return nil
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
