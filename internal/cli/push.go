package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/v2/internal/template"
)

// pushResult is the --json output of every push that reaches the account.
// ScheduledDate is null unless --date reserved the program on the calendar.
type pushResult struct {
	TemplateCode  string  `json:"template_code"`
	ScheduledDate *string `json:"scheduled_date"`
}

// newPushCmd implements `push <plan.json> [--dry-run] [--date today|YYYY-MM-DD] [--json]` (GOAL.md §9.5).
func newPushCmd(app *App) *cobra.Command {
	var (
		dryRun bool
		asJSON bool
		date   string
	)
	cmd := &cobra.Command{
		Use:   "push <plan.json>",
		Short: "Create a Speediance program from a plan JSON",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			plan, err := template.LoadPlan(args[0])
			if err != nil {
				return withCode(ExitUnresolved, err)
			}
			targetDate, err := parseScheduleDate(date, time.Now())
			if err != nil {
				return err
			}

			client, cfg, err := app.apiClient(true)
			if err != nil {
				return err
			}

			nSets := 0
			for _, e := range plan.Exercises {
				nSets += len(e.Sets)
			}
			fprintf(cmd.ErrOrStderr(), "Plan: %s - %d exercises, %d sets\n",
				plan.Name, len(plan.Exercises), nSets)

			stdout := cmd.OutOrStdout()
			if dryRun {
				payload, err := template.BuildPayload(cmd.Context(), client, plan.Name, plan.Exercises, cfg.DeviceType)
				if err != nil {
					return withCode(ExitUnresolved, err)
				}
				app.saveToken(cfg, client)
				if asJSON {
					if targetDate != "" {
						return writeJSON(stdout, map[string]any{
							"payload":        payload,
							"scheduled_date": targetDate,
						})
					}
					return writeJSON(stdout, payload)
				}
				fprintf(stdout, "[dry-run] totalCapacity=%s; not sent.\n",
					strconv.FormatFloat(float64(payload.TotalCapacity), 'f', 0, 64))
				for _, a := range payload.ActionLibraryList {
					fprintf(stdout, "    groupId %d: reps %s | weights %s\n",
						a.GroupID, a.SetsAndReps, a.Weights)
				}
				if targetDate != "" {
					fprintf(stdout, "[dry-run] would schedule for %s.\n", targetDate)
				}
				return nil
			}

			data, err := template.CreateTemplate(cmd.Context(), client, plan.Name, plan.Exercises, cfg.DeviceType)
			if err != nil {
				return withCode(ExitUnresolved, err)
			}
			app.saveToken(cfg, client)
			code, err := templateCodeFrom(data)
			if err != nil {
				return fmt.Errorf("created '%s' but %w", plan.Name, err)
			}
			result := pushResult{TemplateCode: code}

			if targetDate != "" {
				if err := client.ReserveTemplate(cmd.Context(), code, targetDate, cfg.DeviceType); err != nil {
					if asJSON {
						_ = writeJSON(stdout, result)
					}
					return withCode(ExitScheduleFailed, fmt.Errorf(
						"created '%s' (template code %s) but could not schedule it for %s: %w",
						plan.Name, code, targetDate, err))
				}
				result.ScheduledDate = &targetDate
			}

			if asJSON {
				return writeJSON(stdout, result)
			}
			if targetDate != "" {
				fprintf(stdout, "Created '%s' and scheduled it for %s on your Speediance calendar.\n", plan.Name, targetDate)
				return nil
			}
			fprintf(stdout, "Created '%s' on your Speediance account. Open the app to run it.\n", plan.Name)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "build payload, do not send")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print payload/result as JSON")
	cmd.Flags().StringVar(&date, "date", "",
		"also reserve the program on the machine calendar for this day: today | YYYY-MM-DD")
	return cmd
}

// parseScheduleDate resolves --date to a YYYY-MM-DD day, or "" when the flag is
// unset. The calendar only takes today or later, so a past day is a usage error.
func parseScheduleDate(s string, now time.Time) (string, error) {
	if s == "" {
		return "", nil
	}
	day, err := parseDay(s, now)
	if err != nil {
		return "", withCode(ExitUsage, err)
	}
	y, m, d := now.Date()
	if day.Before(time.Date(y, m, d, 0, 0, 0, 0, time.Local)) {
		return "", withCode(ExitUsage, fmt.Errorf(
			"--date %s is in the past; schedule today or later", day.Format(time.DateOnly)))
	}
	return day.Format(time.DateOnly), nil
}

// templateCodeFrom reads the template code from a create response, whose data
// is a bare JSON string such as "6afe53cee4b06ab9dced9026".
func templateCodeFrom(data json.RawMessage) (string, error) {
	var code string
	if err := json.Unmarshal(data, &code); err != nil || code == "" {
		return "", fmt.Errorf("the server returned no template code (data: %s)", data)
	}
	return code, nil
}
