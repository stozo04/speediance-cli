package cli

import (
	"github.com/spf13/cobra"
)

// newCompletionCmd exposes shell-completion generation (GOAL.md §6). Cobra
// ships a built-in completion command, but defining it explicitly lets us set
// the supported shells and a clear description, and keeps the help text stable.
func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: "Output a shell completion script for speediance-cli.\n\n" +
			"  bash:        source <(speediance-cli completion bash)\n" +
			"  zsh:         source <(speediance-cli completion zsh)\n" +
			"  fish:        speediance-cli completion fish | source\n" +
			"  powershell:  speediance-cli completion powershell | Out-String | Invoke-Expression",
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			}
			return nil
		},
	}
	return cmd
}
