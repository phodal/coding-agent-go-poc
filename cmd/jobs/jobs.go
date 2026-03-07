package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	jobcore "agent-platform/core/jobs"
	"github.com/spf13/cobra"
)

func New(_ context.Context) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "List local jobs",
		RunE: func(_ *cobra.Command, _ []string) error {
			workspace, err := os.Getwd()
			if err != nil {
				return err
			}
			manager := jobcore.NewManager(workspace)
			jobs, err := manager.List()
			if err != nil {
				return err
			}
			if output == "json" {
				content, err := json.MarshalIndent(jobs, "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(content))
				return nil
			}
			if len(jobs) == 0 {
				fmt.Println("no jobs found")
				return nil
			}
			for _, job := range jobs {
				fmt.Printf("%s\t%s\t%s\n", job.ID, job.Status, job.SessionID)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "text", "Output format: text or json")
	return cmd
}
