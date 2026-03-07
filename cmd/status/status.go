package status

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func New(_ context.Context) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show platform status",
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(output)
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "Output format: text or json")
	return cmd
}

func run(output string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	projectDir := filepath.Join(wd, ".agent-platform")
	homeDir := filepath.Join(home, ".agent-platform")
	status := map[string]any{
		"workspace":           wd,
		"projectConfigDir":    projectDir,
		"globalConfigDir":     homeDir,
		"projectConfigExists": exists(projectDir),
		"globalConfigExists":  exists(homeDir),
	}
	if output == "json" {
		content, marshalErr := json.MarshalIndent(status, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Println(string(content))
		return nil
	}
	fmt.Printf("workspace: %s\n", wd)
	fmt.Printf("project config: %s (exists=%t)\n", projectDir, exists(projectDir))
	fmt.Printf("global config: %s (exists=%t)\n", homeDir, exists(homeDir))
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
