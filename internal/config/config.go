// Package config resolves runtime configuration and credential discovery.
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

// Environment variable names, a frozen external contract.
const (
	EnvEmail      = "SPEEDIANCE_EMAIL"
	EnvPassword   = "SPEEDIANCE_PASSWORD"
	EnvRegion     = "SPEEDIANCE_REGION"
	EnvDeviceType = "SPEEDIANCE_DEVICE_TYPE"
	EnvConfig     = "SPEEDIANCE_CONFIG"
	EnvTokenCache = "SPEEDIANCE_TOKEN_CACHE"
)

// Built-in defaults.
const (
	DefaultRegion     = "Global"
	DefaultDeviceType = 1

	defaultConfigName = "config.json"

	appUserSubdir   = "speediance"
	userTokenName   = "token.json"
	legacyTokenName = ".token.json"
)

// ErrMissingCredentials reports that email or password could not be resolved.
var ErrMissingCredentials = errors.New("missing credentials")

// Config is the fully resolved configuration handed to commands.
type Config struct {
	Email      string
	Password   string
	Region     string
	DeviceType int

	ConfigPath          string
	ConfigExists        bool
	TokenCachePath      string
	TokenCacheIsDefault bool

	ConfigMayChoosePaths bool
	Warnings             []string
}

type configSource int

const (
	sourceWorkingDir configSource = iota
	sourceExplicit
	sourceUserDir
)

func (s configSource) mayChoosePaths() bool { return s != sourceWorkingDir }

type fileConfig struct {
	Email          *string `json:"email"`
	Password       *string `json:"password"`
	Region         *string `json:"region"`
	DeviceType     *int    `json:"device_type"`
	TokenCachePath *string `json:"token_cache_path"`
}

// Options carries values the caller already knows from flags.
type Options struct {
	ConfigPath string
}

// Load resolves configuration from defaults, config.json and the environment.
func Load(opts Options) (*Config, error) {
	dotenv, _ := godotenv.Read()

	cfg := &Config{
		Region:     DefaultRegion,
		DeviceType: DefaultDeviceType,
	}

	path, source := discoverConfigPath(opts.ConfigPath)
	cfg.ConfigPath = path
	cfg.ConfigMayChoosePaths = source.mayChoosePaths()
	cfg.Warnings = ignoredDotenvPathKeys(dotenv)

	fc, exists, err := readFileConfig(path)
	if err != nil {
		return nil, err
	}
	cfg.ConfigExists = exists
	applyFile(cfg, fc)

	applyEnv(cfg, dotenv)

	resolveTokenCachePath(cfg, fc, source)
	return cfg, nil
}

func resolveTokenCachePath(cfg *Config, fc fileConfig, source configSource) {
	if v, ok := exportedOnly(EnvTokenCache); ok && v != "" {
		cfg.TokenCachePath = v
		return
	}
	if fc.TokenCachePath != nil && *fc.TokenCachePath != "" {
		if source.mayChoosePaths() {
			cfg.TokenCachePath = *fc.TokenCachePath
			return
		}
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf(
			"token_cache_path in %s is ignored: a working-directory config.json can't choose where the session token is written; "+
				"pass --config, export %s or %s, or move the file to <user-config-dir>/%s/%s",
			cfg.ConfigPath, EnvConfig, EnvTokenCache, appUserSubdir, defaultConfigName))
	}
	cfg.TokenCachePath = defaultTokenCachePath()
	cfg.TokenCacheIsDefault = true
}

func ignoredDotenvPathKeys(dotenv map[string]string) []string {
	var warnings []string
	for _, key := range []string{EnvConfig, EnvTokenCache} {
		if _, inDotenv := dotenv[key]; !inDotenv {
			continue
		}
		if _, exported := exportedOnly(key); exported {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("%s in .env is ignored: path settings must be exported", key))
	}
	return warnings
}

func defaultTokenCachePath() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, appUserSubdir, userTokenName)
	}
	return legacyTokenName
}

// LegacyTokenCachePath is the pre-#17 token cache location, kept for migration.
func LegacyTokenCachePath() string { return legacyTokenName }

func discoverConfigPath(flagPath string) (string, configSource) {
	if flagPath != "" {
		return flagPath, sourceExplicit
	}
	if v, ok := exportedOnly(EnvConfig); ok && v != "" {
		return v, sourceExplicit
	}
	if _, err := os.Stat(defaultConfigName); err == nil {
		return defaultConfigName, sourceWorkingDir
	}
	if dir, err := os.UserConfigDir(); err == nil {
		alt := filepath.Join(dir, appUserSubdir, defaultConfigName)
		if _, err := os.Stat(alt); err == nil {
			return alt, sourceUserDir
		}
	}
	return defaultConfigName, sourceWorkingDir
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
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DeviceType = n
		}
	}
}

func envLayer(dotenv map[string]string, key string) (string, bool) {
	if v, ok := os.LookupEnv(key); ok {
		return v, true
	}
	if v, ok := dotenv[key]; ok {
		return v, true
	}
	return "", false
}

func exportedOnly(key string) (string, bool) {
	return os.LookupEnv(key)
}

// RequireCredentials returns ErrMissingCredentials naming every source it checked.
func (c *Config) RequireCredentials() error {
	if c.Email != "" && c.Password != "" {
		return nil
	}
	return fmt.Errorf(
		"%w: set %s and %s (exported env vars, or a .env in the working directory), "+
			"or add \"email\"/\"password\" to config.json. config.json is resolved from "+
			"--config / %s, then ./%s, then <user-config-dir>/%s/%s (resolved this run: %s)",
		ErrMissingCredentials, EnvEmail, EnvPassword,
		EnvConfig, defaultConfigName, appUserSubdir, defaultConfigName, c.ConfigPath,
	)
}

// DeviceWarning returns a warning when the configured device is untested.
func (c *Config) DeviceWarning() string {
	if c.DeviceType != DefaultDeviceType {
		return fmt.Sprintf("warning: device_type=%d is untested (only Gym Monster 1 / device_type=1 is verified)", c.DeviceType)
	}
	return ""
}
