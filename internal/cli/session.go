package cli

import (
	"encoding/json"
	"strconv"

	"github.com/spf13/cobra"
)

// newSessionCmd implements `session <training_id> [--json]` (GOAL.md §9.3). The
// session is a faithful, complete passthrough: it fetches both Speediance session
// endpoints and emits their verbatim data payloads under --json — no derived
// fields, no renaming, no flag to "unlock" data (the endpoints return it, so the
// CLI returns it). The human view is a minimal, display-only listing of the
// exercises; --json is the authoritative output.
func newSessionCmd(app *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "session <training_id>",
		Short: "Full, verbatim Speediance detail for one session",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return withCode(ExitUsage, err)
			}

			client, cfg, err := app.apiClient(true)
			if err != nil {
				return err
			}
			sd, err := client.FetchSessionDetail(cmd.Context(), id)
			if err != nil {
				return err
			}
			app.saveToken(cfg, client)

			stdout := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(stdout, sd)
			}
			names := exerciseNames(sd.Detail)
			if len(names) == 0 {
				fprintf(stdout, "No per-set detail for training %d "+
					"(freestyle 'Free Lift' sessions return none).\n", id)
				return nil
			}
			for _, name := range names {
				fprintf(stdout, "  %s\n", name)
			}
			fprintf(cmd.ErrOrStderr(),
				"Run with --json for the full, verbatim session data.\n")
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

// exerciseNames loosely decodes just the exercise names from the raw detail
// payload for the human listing. This is a display-only convenience and touches
// only the human path; the --json output is the raw, unparsed payload.
func exerciseNames(detail json.RawMessage) []string {
	if len(detail) == 0 {
		return nil
	}
	var exs []struct {
		ActionLibraryName string `json:"actionLibraryName"`
	}
	if err := json.Unmarshal(detail, &exs); err != nil {
		return nil
	}
	names := make([]string, 0, len(exs))
	for _, e := range exs {
		names = append(names, e.ActionLibraryName)
	}
	return names
}
