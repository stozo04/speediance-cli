package cli

import (
	"github.com/spf13/cobra"
)

// newLoginCmd implements `login` (GOAL.md §9.1): authenticate without using any
// cached token, then write .token.json. No --json. Auth failure exits 2.
func newLoginCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate and cache a session token",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			client, cfg, err := app.apiClient(false) // force a fresh login.
			if err != nil {
				return err
			}
			if err := client.Login(cmd.Context()); err != nil {
				return err
			}
			app.saveToken(cfg, client)

			tok := client.Token()
			fprintf(cmd.OutOrStdout(),
				"Logged in (user %s). Token cached to %s — keep this file private.\n",
				tok.UserID, cfg.TokenCachePath)
			fprintln(cmd.ErrOrStderr(),
				"Note: the token grants access to your Speediance account. Do not share or sync this file.")
			return nil
		}),
	}
}
