package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/api"
	"github.com/stozo04/speediance-cli/internal/workout"
)

// newSessionCmd implements `session <training_id> [--json] [--free|--program]`
// (GOAL.md §9.3). The session is a faithful, complete passthrough AND autonomous:
// given only an id it figures out what the session was — a program/Coach session,
// a free weight lift, or a rowing/ski free session — by probing the program
// namespace first and falling back to the free namespace, then emits the verbatim
// payloads under a uniform {training_id, kind, info, detail} shape. The agent does
// not need to know the type. --free / --program are strict overrides for the rare
// case where a trainingId is valid in BOTH namespaces (it identifies different
// sessions in each), letting the caller force the intended one.
func newSessionCmd(app *App) *cobra.Command {
	var asJSON, free, program bool
	cmd := &cobra.Command{
		Use:   "session <training_id>",
		Short: "Full, verbatim Speediance detail for one session (any type)",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return withCode(ExitUsage, err)
			}
			mode, err := dispatchMode(free, program)
			if err != nil {
				return err
			}

			client, cfg, err := app.apiClient(true)
			if err != nil {
				return err
			}
			sd, err := client.ResolveSession(cmd.Context(), id, mode)
			if err != nil {
				return err
			}
			app.saveToken(cfg, client)

			stdout := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(stdout, sd)
			}
			fprintf(stdout, "%s\n", humanSession(sd))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")
	cmd.Flags().BoolVar(&free, "free", false,
		"force the free-lift namespace (freestyle/rowing); overrides auto-detection")
	cmd.Flags().BoolVar(&program, "program", false,
		"force the program/Coach namespace; overrides auto-detection")
	return cmd
}

// dispatchMode maps the --free/--program flags to a resolver mode. The two flags
// are mutually exclusive; neither means autonomous (program-first, free-fallback).
func dispatchMode(free, program bool) (api.DispatchMode, error) {
	switch {
	case free && program:
		return api.Auto, withCode(ExitUsage,
			fmt.Errorf("--free and --program are mutually exclusive"))
	case free:
		return api.FreeOnly, nil
	case program:
		return api.ProgramOnly, nil
	default:
		return api.Auto, nil
	}
}

// humanSession renders the one-line human view for a resolved session. The full,
// authoritative data is in --json; this is a display-only convenience.
func humanSession(sd *workout.SessionDetail) string {
	switch sd.Kind {
	case "program":
		names := exerciseNames(sd.Detail)
		if len(names) == 0 {
			return fmt.Sprintf("training %d: program session, no per-set detail", sd.TrainingID)
		}
		return fmt.Sprintf("training %d (program): %s", sd.TrainingID, strings.Join(names, ", "))
	case "free":
		return fmt.Sprintf("training %d (free): %s", sd.TrainingID, freeSummary(sd.Info))
	default:
		return fmt.Sprintf("training %d: no detail found "+
			"(freestyle 'Free Lift' sessions can return none; try --free/--program if the id is ambiguous)",
			sd.TrainingID)
	}
}

// exerciseNames loosely decodes just the exercise names from a program detail
// array for the human listing. Display-only; --json carries the raw payload.
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

// freeSummary renders a compact line from a free-lift info payload. Display-only.
func freeSummary(info json.RawMessage) string {
	if len(info) == 0 {
		return "no detail"
	}
	var f struct {
		TrainingTime  json.Number `json:"trainingTime"`
		TotalCapacity json.Number `json:"totalCapacity"`
		Calorie       json.Number `json:"calorie"`
		TotalEnergy   json.Number `json:"totalEnergy"`
		TotalDistance json.Number `json:"totalDistance"`
	}
	_ = json.Unmarshal(info, &f)
	parts := []string{}
	if f.TrainingTime != "" {
		parts = append(parts, f.TrainingTime.String()+"s")
	}
	if f.Calorie != "" {
		parts = append(parts, f.Calorie.String()+" kcal")
	}
	if f.TotalCapacity != "" && f.TotalCapacity != "0.0" && f.TotalCapacity != "0" {
		parts = append(parts, "capacity "+f.TotalCapacity.String())
	}
	if f.TotalDistance != "" {
		parts = append(parts, "distance "+f.TotalDistance.String())
	}
	if len(parts) == 0 {
		return "session-level totals only"
	}
	return strings.Join(parts, ", ")
}
