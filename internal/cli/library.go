package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/template"
)

// newLibraryCmd implements `library [--search X] [--out FILE] [--json]`
// (GOAL.md §9.4). It always writes the full catalog to --out; --search filters
// the stdout view only.
func newLibraryCmd(app *App) *cobra.Command {
	var (
		out    string
		search string
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "library",
		Short: "Fetch the exercise catalog (id/name/muscle)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, _ []string) error {
			client, cfg, err := app.apiClient(true)
			if err != nil {
				return err
			}
			lib, err := template.FetchLibrary(cmd.Context(), client, cfg.DeviceType)
			if err != nil {
				return err
			}
			app.saveToken(cfg, client)

			// Always persist the full catalog (GOAL.md §9.4).
			if err := writeJSONFile(out, lib); err != nil {
				return err
			}
			fprintf(cmd.ErrOrStderr(), "Saved %d exercises to %s\n", len(lib), out)

			// Filter for the stdout view only.
			hits := lib
			if search != "" {
				q := strings.ToLower(search)
				hits = make([]template.Exercise, 0, len(lib))
				for _, e := range lib {
					if strings.Contains(strings.ToLower(e.Name), q) ||
						strings.Contains(strings.ToLower(e.Muscle), q) {
						hits = append(hits, e)
					}
				}
			}

			stdout := cmd.OutOrStdout()
			switch {
			case asJSON:
				return writeJSON(stdout, hits)
			case search != "":
				fprintf(stdout, "%d match '%s':\n", len(hits), search)
				for i, e := range hits {
					if i >= 60 {
						break
					}
					fprintf(stdout, "  [%d] %s (%s)\n", e.ID, e.Name, e.Muscle)
				}
			}
			// Human mode without --search prints nothing on stdout (only the
			// stderr save line above).
			return nil
		}),
	}
	cmd.Flags().StringVar(&out, "out", "library.json", "write the full catalog to this file")
	cmd.Flags().StringVar(&search, "search", "", "filter by name/muscle substring")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print results as JSON")
	return cmd
}
