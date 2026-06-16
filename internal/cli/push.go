package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/template"
)

// newPushCmd implements `push <plan.json> [--dry-run] [--json]` (GOAL.md §9.5).
func newPushCmd(app *App) *cobra.Command {
	var (
		dryRun bool
		asJSON bool
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
					return writeJSON(stdout, payload)
				}
				fprintf(stdout, "[dry-run] totalCapacity=%s; not sent.\n",
					strconv.FormatFloat(float64(payload.TotalCapacity), 'f', 0, 64))
				for _, a := range payload.ActionLibraryList {
					fprintf(stdout, "    groupId %d: reps %s | weights %s\n",
						a.GroupID, a.SetsAndReps, a.Weights)
				}
				return nil
			}

			data, err := template.CreateTemplate(cmd.Context(), client, plan.Name, plan.Exercises, cfg.DeviceType)
			if err != nil {
				return withCode(ExitUnresolved, err)
			}
			app.saveToken(cfg, client)
			if asJSON {
				return writeJSON(stdout, data)
			}
			fprintf(stdout, "Created '%s' on your Speediance account. Open the app to run it.\n", plan.Name)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "build payload, do not send")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print payload/result as JSON")
	return cmd
}
