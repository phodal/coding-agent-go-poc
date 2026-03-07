package root

import (
	"context"

	"agent-platform/cmd/jobs"
	"agent-platform/cmd/mcp"
	"agent-platform/cmd/spec"
	"agent-platform/cmd/start"
	"agent-platform/cmd/status"

	"github.com/spf13/cobra"
)

func New(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent-platform",
		Short: "Local-first Agent Runtime platform",
		Long:  "A minimal Agent platform runtime with sessions, resources, tools, permissions, and provider abstraction.",
	}
	cmd.AddCommand(jobs.New(ctx))
	cmd.AddCommand(mcp.New(ctx))
	cmd.AddCommand(spec.New(ctx))
	cmd.AddCommand(start.New(ctx))
	cmd.AddCommand(status.New(ctx))
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		return start.RunInteractive(ctx)
	}
	return cmd
}
