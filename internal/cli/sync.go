package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/sheet"
)

// newSyncCmd implements `sync [--date D] [--days N] [--weeks-dir DIR]`
// (GOAL.md §9.6, §11). It writes a completed session into a Markdown week sheet.
// No --json.
func newSyncCmd(app *App) *cobra.Command {
	var (
		dateArg  string
		days     int
		weeksDir string
	)
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "(optional) Write a session into a Markdown week sheet",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			cfg, err := app.resolveConfig()
			if err != nil {
				return err
			}
			// Flag overrides env/config when provided (flags > env > file).
			if weeksDir != "" {
				cfg.SetWeeksDir(weeksDir)
			}
			if err := cfg.RequireWeeksDir(); err != nil {
				return withCode(ExitConfig, err)
			}

			target, err := resolveDate(dateArg)
			if err != nil {
				return withCode(ExitUsage, err)
			}

			client, _, err := app.apiClient(true)
			if err != nil {
				return err
			}
			lookback := days
			if lookback < 1 {
				lookback = 1
			}
			workouts, err := client.FetchWorkouts(cmd.Context(), lookback)
			if err != nil {
				return err
			}
			app.saveToken(cfg, client)

			targetStr := target.Format("2006-01-02")
			var todays []int
			for i, w := range workouts {
				if d := w.Date(); d != nil && *d == targetStr {
					todays = append(todays, i)
				}
			}

			stdout := cmd.OutOrStdout()
			if len(todays) == 0 {
				fprintf(stdout, "No completed Speediance session found for %s.\n", targetStr)
				return nil
			}

			sheetPath, err := sheet.FindWeekSheet(cfg.WeeksDir, target)
			if err != nil {
				return err
			}
			if sheetPath == "" {
				fprintf(stdout, "No Week-XX.md sheet found in %s\n", cfg.WeeksDir)
				return nil
			}

			fprintf(stdout, "Syncing %d session(s) for %s -> %s\n\n", len(todays), targetStr, sheetPath)
			for _, idx := range todays {
				w := &workouts[idx]
				if err := client.PopulateDetail(cmd.Context(), w); err != nil {
					return err
				}
				res, err := sheet.WriteSession(sheetPath, w, target, cfg.Unit)
				if err != nil {
					return err
				}
				fprintf(stdout, "%s: matched %d/%d exercises\n", w.Title, len(res.Matched), res.ExerciseCount)
				if len(res.Unmatched) > 0 {
					fprintf(stdout, "    not matched (logged in notes): %s\n", strings.Join(res.Unmatched, ", "))
				}
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&dateArg, "date", "today", "today | yesterday | YYYY-MM-DD")
	cmd.Flags().IntVar(&days, "days", 3, "look back this many days")
	cmd.Flags().StringVar(&weeksDir, "weeks-dir", "", "folder with Week-XX.md sheets")
	return cmd
}

// resolveDate parses the --date flag: "today" (or empty), "yesterday", or an
// explicit YYYY-MM-DD (GOAL.md §11). The result is a local-midnight date.
func resolveDate(s string) (time.Time, error) {
	now := time.Now()
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	switch s {
	case "", "today":
		return today, nil
	case "yesterday":
		return today.AddDate(0, 0, -1), nil
	default:
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid --date %q: want today | yesterday | YYYY-MM-DD", s)
		}
		return t, nil
	}
}
