package start

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-platform/core/permission"
	platformruntime "agent-platform/core/runtime"
	"agent-platform/core/types"
	"github.com/spf13/cobra"
)

type options struct {
	workspace string
	prompt    string
	sessionID string
	agent     string
	provider  string
	json      bool
	yes       bool
}

func New(ctx context.Context) *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start an agent session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(ctx, cmd, *opts)
		},
	}
	cmd.Flags().StringVarP(&opts.workspace, "workspace", "w", mustGetwd(), "Workspace directory")
	cmd.Flags().StringVarP(&opts.prompt, "prompt", "p", "", "Run a single prompt and exit")
	cmd.Flags().StringVar(&opts.sessionID, "session", "", "Resume an existing session")
	cmd.Flags().StringVar(&opts.agent, "agent", "", "Agent name to run")
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Provider name (default: infer from agent model)")
	cmd.Flags().BoolVar(&opts.json, "json", false, "Print JSON output")
	cmd.Flags().BoolVar(&opts.yes, "yes", false, "Auto-approve all permission requests")
	return cmd
}

func RunInteractive(ctx context.Context) error {
	return run(ctx, nil, options{workspace: mustGetwd()})
}

func run(ctx context.Context, cmd *cobra.Command, opts options) error {
	workspace, err := filepath.Abs(opts.workspace)
	if err != nil {
		return err
	}
	runtime, err := platformruntime.New(workspace)
	if err != nil {
		return err
	}
	permissionHandler := interactivePermissionHandler(opts.yes)

	if strings.TrimSpace(opts.prompt) != "" {
		return runOnce(ctx, runtime, opts, permissionHandler)
	}

	reader := bufio.NewReader(os.Stdin)
	currentSessionID := opts.sessionID
	for {
		fmt.Print("agent> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return nil
		}
		turnResult, err := runtime.Start(ctx, platformruntime.StartRequest{
			Workspace:         workspace,
			Prompt:            line,
			SessionID:         currentSessionID,
			AgentName:         opts.agent,
			ProviderName:      opts.provider,
			PermissionHandler: permissionHandler,
		})
		if err != nil {
			return err
		}
		currentSessionID = turnResult.Session.ID
		printMessage(turnResult.Output)
		if cmd != nil && opts.json {
			break
		}
	}
	return nil
}

func runOnce(ctx context.Context, runtime *platformruntime.Runtime, opts options, permissionHandler permission.Handler) error {
	result, err := runtime.Start(ctx, platformruntime.StartRequest{
		Workspace:         opts.workspace,
		Prompt:            opts.prompt,
		SessionID:         opts.sessionID,
		AgentName:         opts.agent,
		ProviderName:      opts.provider,
		PermissionHandler: permissionHandler,
	})
	if err != nil {
		return err
	}
	if opts.json {
		payload := struct {
			SessionID   string           `json:"sessionId"`
			StopReason  types.StopReason `json:"stopReason"`
			Message     types.Message    `json:"message"`
			ToolSummary map[string]int   `json:"toolBreakdown"`
		}{
			SessionID:   result.Session.ID,
			StopReason:  result.Session.AgentState.CurrentStopReason,
			Message:     result.Output,
			ToolSummary: result.Session.AgentState.ToolBreakdown,
		}
		content, marshalErr := json.MarshalIndent(payload, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Println(string(content))
		return nil
	}
	printMessage(result.Output)
	fmt.Printf("\nsession: %s\n", result.Session.ID)
	return nil
}

func interactivePermissionHandler(autoApprove bool) permission.Handler {
	if autoApprove {
		return func(_ context.Context, _ permission.Request) (permission.Response, error) {
			return permission.Response{Decision: permission.DecisionAllow, Reason: "auto approved by --yes"}, nil
		}
	}
	reader := bufio.NewReader(os.Stdin)
	return func(_ context.Context, req permission.Request) (permission.Response, error) {
		if req.RiskLevel == types.RiskLow {
			return permission.Response{Decision: permission.DecisionAllow}, nil
		}
		fmt.Printf("\nPermission request\n")
		fmt.Printf("tool: %s\nrisk: %s\nsummary: %s\n", req.ToolName, req.RiskLevel, req.Summary)
		if req.CommandPreview != "" {
			fmt.Printf("command: %s\n", req.CommandPreview)
		}
		if len(req.FileTargets) > 0 {
			fmt.Printf("files: %s\n", strings.Join(req.FileTargets, ", "))
		}
		fmt.Print("allow? [y/N]: ")
		answer, err := reader.ReadString('\n')
		if err != nil {
			return permission.Response{}, err
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer == "y" || answer == "yes" {
			return permission.Response{Decision: permission.DecisionAllow}, nil
		}
		return permission.Response{Decision: permission.DecisionDeny, Reason: "denied by user"}, nil
	}
}

func printMessage(msg types.Message) {
	if len(msg.ToolResults) > 0 {
		for _, result := range msg.ToolResults {
			if result.Success {
				fmt.Printf("[tool:%s]\n%s\n", result.ToolName, joinContent(result.Content))
			} else {
				fmt.Printf("[tool:%s] failed: %s\n", result.ToolName, result.Error)
			}
		}
		return
	}
	if len(msg.ToolCalls) > 0 {
		fmt.Printf("[tool request] %s\n", msg.ToolCalls[0].Name)
	}
	if len(msg.Content) > 0 {
		fmt.Println(joinContent(msg.Content))
	}
}

func joinContent(parts []types.ContentPart) string {
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		items = append(items, part.Text)
	}
	return strings.Join(items, "\n")
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
