package cli

import (
	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/workout"
)

// newWorkoutsCmd implements `workouts [--days N] [--json]` (GOAL.md §9.2).
func newWorkoutsCmd(app *App) *cobra.Command {
	var (
		days   int
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "workouts",
		Short: "List recent completed sessions",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			client, cfg, err := app.apiClient(true)
			if err != nil {
				return err
			}
			ws, err := client.FetchWorkouts(cmd.Context(), days)
			if err != nil {
				return err
			}
			app.saveToken(cfg, client)

			// Build a non-nil slice so an empty result encodes as [] (not null),
			// matching Python's json.dumps([]).
			rows := make([]workout.Summary, 0, len(ws))
			for _, w := range ws {
				rows = append(rows, w.Summary())
			}

			out := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(out, rows)
			}
			if len(rows) == 0 {
				fprintf(out, "No completed workouts in the last %d day(s).\n", days)
				return nil
			}
			fprintf(out, "Found %d session(s) in the last %d day(s):\n\n", len(rows), days)
			for _, r := range rows {
				date := "None"
				if r.Date != nil {
					date = *r.Date
				}
				fprintf(out, "  - %s  %s  -  %d min, %d kcal  (id %d)\n",
					date, r.Title, r.DurationSecs/60, r.Calories, r.TrainingID)
			}
			return nil
		}),
	}
	cmd.Flags().IntVar(&days, "days", 3, "look back this many days")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	return cmd
}
