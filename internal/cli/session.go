package cli

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// newSessionCmd implements `session <training_id> [--json]` (GOAL.md §9.3).
func newSessionCmd(app *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "session <training_id>",
		Short: "Full per-set detail for one session",
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
			w, err := client.FetchDetail(cmd.Context(), id)
			if err != nil {
				return err
			}
			app.saveToken(cfg, client)

			out := w.SessionOutput()
			stdout := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(stdout, out)
			}
			if len(out.Exercises) == 0 {
				fprintf(stdout, "No per-set detail for training %d "+
					"(freestyle 'Free Lift' sessions return none).\n", id)
				return nil
			}
			for _, ex := range out.Exercises {
				parts := make([]string, 0, len(ex.Sets))
				for _, s := range ex.Sets {
					parts = append(parts, formatG(s.Weight)+"x"+strconv.Itoa(s.Reps))
				}
				fprintf(stdout, "  %s: %s\n", ex.Name, strings.Join(parts, ", "))
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}

// formatG renders a json.Number the way Python's "%g" does for the human session
// view: shortest form, trailing zeros dropped (e.g. 20.0 -> "20", 22.5 -> "22.5").
func formatG(n json.Number) string {
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil {
		return string(n)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}
