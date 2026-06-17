package cli

import (
	"github.com/spf13/cobra"

	"github.com/stozo04/speediance-cli/internal/version"
)

// newVersionCmd implements `version [--json]` (GOAL.md §9.8). The root also
// supports a `--version` flag for the short string; this subcommand adds the
// structured form.
func newVersionCmd(app *App) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print build version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.Info()
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), info)
			}
			fprintln(cmd.OutOrStdout(), info.String())
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit machine-readable JSON")
	_ = app // reserved for future use; keeps the constructor signature uniform.
	return cmd
}
