package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/api"
	"github.com/stozo04/speediance-cli/internal/auth"
	"github.com/stozo04/speediance-cli/internal/config"
)

// baseURLOverride, when non-empty, points the API client at a specific base URL
// instead of deriving one from the region. It exists only so in-package tests
// can redirect the client at an httptest server; production never sets it.
var baseURLOverride string

// apiClient resolves config, validates credentials, and builds an API client.
// When useCache is true the cached token is loaded so a user with a valid token
// isn't forced to re-login (GOAL.md §7); login passes false to force a fresh
// authentication.
func (a *App) apiClient(useCache bool) (*api.Client, *config.Config, error) {
	cfg, err := a.resolveConfig()
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.RequireCredentials(); err != nil {
		return nil, nil, withCode(ExitConfig, err)
	}
	if w := cfg.DeviceWarning(); w != "" {
		a.logger.Warn(w)
	}

	var tok auth.Token
	if useCache {
		t, _, err := auth.Load(cfg.TokenCachePath)
		if err != nil {
			// A bad cache is non-fatal: warn to stderr and log in fresh.
			a.logger.Warn("token cache unreadable", "path", cfg.TokenCachePath, "err", err)
		} else {
			tok = t
		}
	}

	client := api.New(api.Config{
		Region:   cfg.Region,
		Email:    cfg.Email,
		Password: cfg.Password,
		Token:    tok,
		Logger:   a.logger,
		BaseURL:  baseURLOverride, // empty in production.
	})
	return client, cfg, nil
}

// saveToken writes the client's (possibly refreshed) token back to the cache.
// Failure is non-fatal — a command that already produced its output shouldn't
// fail because the cache couldn't be written (GOAL.md §7).
func (a *App) saveToken(cfg *config.Config, client *api.Client) {
	if err := auth.Save(cfg.TokenCachePath, client.Token()); err != nil {
		a.logger.Warn("could not write token cache", "path", cfg.TokenCachePath, "err", err)
	}
}

// classify maps an API auth failure to exit code 2 (GOAL.md §12). Other errors
// pass through for main to map to the generic failure code.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var ae *api.AuthError
	if errors.As(err, &ae) {
		return &ExitError{Code: ExitAuth, Err: err}
	}
	return err
}

// runE wraps a command body so any auth error is classified to exit 2 before it
// reaches main.
func runE(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return classify(fn(cmd, args))
	}
}
