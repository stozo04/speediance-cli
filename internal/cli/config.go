package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

// configView is the stable shape emitted by `config show --json`. The password
// is always blanked here — this view is a convenience, not a secret dump
// (GOAL.md §9.7).
type configView struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	Region         string `json:"region"`
	DeviceType     int    `json:"device_type"`
	ConfigPath     string `json:"config_path"`
	TokenCachePath string `json:"token_cache_path"`
}

// validConfigKeys are the keys `config set` accepts (the config.json schema).
var validConfigKeys = map[string]bool{
	"email": true, "password": true, "region": true,
	"device_type": true,
}

// newConfigCmd implements `config [show|set|path]` (GOAL.md §9.7), a convenience
// for managing config.json without hand-editing.
func newConfigCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration (show/set/path)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newConfigShowCmd(app), newConfigSetCmd(app), newConfigPathCmd(app))
	return cmd
}

func newConfigShowCmd(app *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the resolved effective config (password masked)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := app.resolveConfig()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				view := configView{
					Email:          cfg.Email,
					Password:       "", // never emit the secret in JSON.
					Region:         cfg.Region,
					DeviceType:     cfg.DeviceType,
					ConfigPath:     cfg.ConfigPath,
					TokenCachePath: cfg.TokenCachePath,
				}
				return writeJSON(out, view)
			}
			pw := ""
			if cfg.Password != "" {
				pw = "****"
			}
			fprintf(out, "email:            %s\n", cfg.Email)
			fprintf(out, "password:         %s\n", pw)
			fprintf(out, "region:           %s\n", cfg.Region)
			fprintf(out, "device_type:      %d\n", cfg.DeviceType)
			fprintf(out, "config_path:      %s\n", cfg.ConfigPath)
			fprintf(out, "token_cache_path: %s\n", cfg.TokenCachePath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

func newConfigSetCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a key in config.json",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			if !validConfigKeys[key] {
				return withCode(ExitUsage, fmt.Errorf("unknown config key %q (valid: email, password, region, device_type)", key))
			}
			cfg, err := app.resolveConfig()
			if err != nil {
				return err
			}
			if err := setConfigKey(cfg.ConfigPath, key, value); err != nil {
				return withCode(ExitConfig, err)
			}
			fprintf(cmd.ErrOrStderr(), "Set %s in %s\n", key, cfg.ConfigPath)
			return nil
		},
	}
}

func newConfigPathCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print resolved config and token-cache paths",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := app.resolveConfig()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fprintf(out, "config:      %s\n", cfg.ConfigPath)
			fprintf(out, "token cache: %s\n", cfg.TokenCachePath)
			return nil
		},
	}
}

// setConfigKey reads config.json (if present), updates one key, and writes it
// back with owner-only (0600) permissions since the file may hold a password
// (GOAL.md §9.7).
func setConfigKey(path, key, value string) error {
	obj := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &obj); err != nil {
			return fmt.Errorf("parse existing config %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read config %s: %w", path, err)
	}

	if key == "device_type" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("device_type must be an integer: %w", err)
		}
		obj[key] = json.RawMessage(strconv.Itoa(n))
	} else {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		obj[key] = encoded
	}

	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}
	// Owner-only because config.json may store the password (GOAL.md §7, §9.7).
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open config %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	_ = f.Chmod(0o600) // best-effort re-tighten; no-op/ignored on Windows.
	if _, err := f.Write(out); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}
