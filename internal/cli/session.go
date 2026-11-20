package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/v2/internal/api"
	"github.com/stozo04/speediance-cli/v2/internal/workout"
)

// newSessionCmd implements `session <training_id> [--json] [--free|--program|--course]`
// (GOAL.md §9.3). The session is a faithful, complete passthrough AND autonomous:
// given only an id it figures out what the session was — a program/Coach session,
// an official course, a free weight lift, or a rowing/ski free session — by
// probing the program, course and free namespaces in turn, then emits the verbatim
// payloads under a uniform {training_id, kind, info, detail} shape. The agent does
// not need to know the type. --free / --program / --course are strict overrides for the rare
// case where a trainingId is valid in multiple namespaces (it identifies different
// sessions in each), letting the caller force the intended one.
func newSessionCmd(app *App) *cobra.Command {
	var asJSON, free, program, course bool
	cmd := &cobra.Command{
		Use:   "session <training_id>",
		Short: "Full, verbatim Speediance detail for one session (any type)",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return withCode(ExitUsage, err)
			}
			mode, err := dispatchMode(free, program, course)
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
	cmd.Flags().BoolVar(&course, "course", false,
		"force the official course namespace; overrides auto-detection")
	return cmd
}

// dispatchMode maps the --free/--program/--course flags to a resolver mode. The flags
// are mutually exclusive; none specified means autonomous (program/course/free fallback).
func dispatchMode(free, program, course bool) (api.DispatchMode, error) {
	count := 0
	if free {
		count++
	}
	if program {
		count++
	}
	if course {
		count++
	}
	if count > 1 {
		return api.Auto, withCode(ExitUsage,
			fmt.Errorf("--free, --program, and --course are mutually exclusive"))
	}
	if free {
		return api.FreeOnly, nil
	}
	if program {
		return api.ProgramOnly, nil
	}
	if course {
		return api.CourseOnly, nil
	}
	return api.Auto, nil
}

// humanSession renders the one-line human view for a resolved session. The full,
// authoritative data is in --json; this is a display-only convenience.
func humanSession(sd *workout.SessionDetail) string {
	switch sd.Kind {
	case "program", "course":
		names := exerciseNames(sd.Detail)
		if len(names) == 0 {
			return fmt.Sprintf("training %d: %s session, no per-set detail", sd.TrainingID, sd.Kind)
		}
		return fmt.Sprintf("training %d (%s): %s", sd.TrainingID, sd.Kind, strings.Join(names, ", "))
	case "free":
		return fmt.Sprintf("training %d (free): %s", sd.TrainingID, freeSummary(sd.Info))
	default:
		return fmt.Sprintf("training %d: no detail found "+
			"(freestyle 'Free Lift' sessions can return none; try --free/--program/--course if the id is ambiguous)",
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
