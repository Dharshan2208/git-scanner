package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = newRootCmd()

func newRootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "git-scanner",
		Short: "A fast concurrent repo scanner for secrets and APIs",
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// ExecuteContext runs the CLI with the given context, propagating it to commands.
func ExecuteContext(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

// RunWithContext is an adapter for cobra's RunE that extracts context from
// the command and provides typed error returns instead of log.Fatal.
func RunWithContext(fn func(ctx context.Context, cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		return fn(ctx, cmd, args)
	}
}
