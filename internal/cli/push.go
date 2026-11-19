package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/v2/internal/template"
)

// newPushCmd implements `push <plan.json> [--dry-run] [--date today|yesterday|YYYY-MM-DD] [--json]` (GOAL.md §9.5).
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

			var targetDate string
			if date != "" {
				if strings.ToLower(date) == "yesterday" {
					return withCode(ExitUsage, fmt.Errorf("cannot schedule a workout for yesterday"))
				}
				day, err := parseDay(date, time.Now())
				if err != nil {
					return withCode(ExitUsage, err)
				}
				targetDate = day.Format("2006-01-02")
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

			var templateCode string
			_ = json.Unmarshal(data, &templateCode)
			if templateCode == "" {
				templateCode = strings.Trim(string(data), `"`)
			}

			if targetDate != "" {
				if templateCode == "" {
					return fmt.Errorf("template '%s' was created but server returned no template code for scheduling", plan.Name)
				}
				if err := client.ReserveTemplate(cmd.Context(), templateCode, targetDate, cfg.DeviceType); err != nil {
					return fmt.Errorf("created template '%s' (code %s) but scheduling for %s failed: %w", plan.Name, templateCode, targetDate, err)
				}
				if asJSON {
					return writeJSON(stdout, map[string]any{
						"template_code":  templateCode,
						"scheduled_date": targetDate,
						"scheduled":      true,
					})
				}
				fprintf(stdout, "Created '%s' and scheduled for %s on your Speediance calendar.\n", plan.Name, targetDate)
				return nil
			}

			if asJSON {
				return writeJSON(stdout, data)
			}
			fprintf(stdout, "Created '%s' on your Speediance account. Open the app to run it.\n", plan.Name)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "build payload, do not send")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print payload/result as JSON")
	cmd.Flags().StringVar(&date, "date", "", "optional date to schedule on the machine calendar: today | yesterday | YYYY-MM-DD")
	return cmd
}
