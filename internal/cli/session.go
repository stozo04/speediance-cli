package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/workout"
)

// newSessionCmd implements `session <training_id> [--json] [--telemetry]`
// (GOAL.md §9.3). --telemetry adds the real per-rep, per-side telemetry (weights,
// power, ROM, tempo, timestamps) and per-exercise form scores the API returns
// (issue #23); without it, the lean per-set view is emitted.
func newSessionCmd(app *App) *cobra.Command {
	var asJSON, telemetry bool
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

			out := w.SessionOutput(telemetry)
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
				line := "  " + ex.Name + ": " + strings.Join(parts, ", ")
				// --telemetry annotates the human line with the form scores; the
				// full per-rep arrays are reserved for --json (too verbose for the
				// terminal view).
				if telemetry && ex.Scores != nil {
					line += "  " + formatScores(ex.Scores)
				}
				fprintf(stdout, "%s\n", line)
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.Flags().BoolVar(&telemetry, "telemetry", false,
		"include per-rep telemetry (power, ROM, tempo) and per-exercise form scores")
	return cmd
}

// formatScores renders the per-exercise form scores as a compact bracketed
// annotation for the human session view.
func formatScores(s *workout.Scores) string {
	return fmt.Sprintf("[score %d · completion %d · force %d · balance %d · amplitude %d · rating %d]",
		s.Total, s.Completion, s.ForceControl, s.BilateralBalance, s.AmplitudeStable, s.Rating)
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
