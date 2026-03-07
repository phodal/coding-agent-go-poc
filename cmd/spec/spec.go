package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"agent-platform/core/specengine"
	"github.com/spf13/cobra"
)

type options struct {
	workspace string
	prompt    string
	agent     string
	provider  string
	json      bool
}

func New(ctx context.Context) *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Generate staged specification documents",
		RunE: func(_ *cobra.Command, _ []string) error {
			service, err := specengine.New(absPath(opts.workspace))
			if err != nil {
				return err
			}
			result, err := service.Generate(ctx, specengine.GenerateRequest{
				Workspace:    absPath(opts.workspace),
				Prompt:       opts.prompt,
				AgentName:    opts.agent,
				ProviderName: opts.provider,
			})
			if err != nil {
				return err
			}
			if opts.json {
				content, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(content))
				return nil
			}
			fmt.Printf("spec: %s\n", result.SpecID)
			fmt.Printf("dir: %s\n", result.Directory)
			for name, path := range result.Files {
				fmt.Printf("%s: %s\n", name, path)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&opts.workspace, "workspace", "w", ".", "Workspace directory")
	cmd.Flags().StringVarP(&opts.prompt, "prompt", "p", "", "Specification request")
	cmd.Flags().StringVar(&opts.agent, "agent", "", "Agent name to use")
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider name")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print JSON output")
	_ = cmd.MarkFlagRequired("prompt")
	return cmd
}

func absPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}
