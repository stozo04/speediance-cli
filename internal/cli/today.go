package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// newTodayCmd implements `today [--date today|yesterday|YYYY-MM-DD] [--json]`. It
// is the one-shot, agent-friendly entry point: "give me the client's workout(s)
// for today." The agent supplies only a day — it need not know whether the client
// ran a program, free-lifted, or rowed. The tool discovers each session on that
// day from the record list (which carries the authoritative type) and returns each
// one fully resolved to its type-correct detail, as an array of the same uniform
// {training_id, kind, info, detail} shape that `session` emits.
func newTodayCmd(app *App) *cobra.Command {
	var (
		asJSON bool
		date   string
	)
	cmd := &cobra.Command{
		Use:   "today",
		Short: "Full detail for every session on a day (auto-detects program/free/rowing)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			day, err := parseDay(date, time.Now())
			if err != nil {
				return withCode(ExitUsage, err)
			}

			client, cfg, err := app.apiClient(true)
			if err != nil {
				return err
			}
			sessions, err := client.FetchDay(cmd.Context(), day)
			if err != nil {
				return err
			}
			app.saveToken(cfg, client)

			stdout := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(stdout, sessions)
			}
			if len(sessions) == 0 {
				fprintf(stdout, "No workouts on %s.\n", day.Format("2006-01-02"))
				return nil
			}
			fprintf(stdout, "%d session(s) on %s:\n", len(sessions), day.Format("2006-01-02"))
			for i := range sessions {
				fprintf(stdout, "  - %s\n", humanSession(&sessions[i]))
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.Flags().StringVar(&date, "date", "today",
		"day to fetch: today | yesterday | YYYY-MM-DD")
	return cmd
}

// parseDay resolves the --date value (today | yesterday | YYYY-MM-DD) against the
// supplied clock, in local time.
func parseDay(s string, now time.Time) (time.Time, error) {
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "today":
		return today, nil
	case "yesterday":
		return today.AddDate(0, 0, -1), nil
	default:
		t, err := time.ParseInLocation("2006-01-02", s, time.Local)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid --date %q: want today, yesterday, or YYYY-MM-DD", s)
		}
		return t, nil
	}
}
