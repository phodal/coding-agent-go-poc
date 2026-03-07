package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"agent-platform/core/permission"
	"agent-platform/core/types"
)

type ReadFileParams struct {
	Path string `json:"path"`
}

type WriteFileParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type BashParams struct {
	Command string `json:"command"`
}

type AskUserParams struct {
	Question string `json:"question"`
}

func BuiltinReadFile() Tool {
	return Spec[ReadFileParams]{
		Info: Descriptor{
			Name:        "read_file",
			Description: "Read a file from workspace",
			Category:    "file",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "description": "Relative or absolute file path"}}, "required": []string{"path"}},
		},
		Parse: ParseJSON[ReadFileParams],
		Permission: func(_ context.Context, params ReadFileParams, _ ExecutionEnv) (permission.Request, error) {
			return permission.Request{ToolName: "read_file", RiskLevel: types.RiskLow, Summary: "Read file", FileTargets: []string{params.Path}}, nil
		},
		Handler: func(_ context.Context, params ReadFileParams, env ExecutionEnv) (ExecutionResult, error) {
			target := resolvePath(env.Workspace, params.Path)
			content, err := os.ReadFile(target)
			if err != nil {
				return ExecutionResult{Success: false, Error: err.Error()}, err
			}
			return ExecutionResult{Success: true, Content: []types.ContentPart{{Type: "text", Text: string(content)}}, Metadata: map[string]any{"path": target}}, nil
		},
	}
}

func BuiltinWriteFile() Tool {
	return Spec[WriteFileParams]{
		Info: Descriptor{
			Name:        "write_file",
			Description: "Write a file into workspace",
			Category:    "file",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string", "description": "Relative or absolute file path"}, "content": map[string]any{"type": "string", "description": "Full file content to write"}}, "required": []string{"path", "content"}},
		},
		Parse: ParseJSON[WriteFileParams],
		Permission: func(_ context.Context, params WriteFileParams, _ ExecutionEnv) (permission.Request, error) {
			return permission.Request{ToolName: "write_file", RiskLevel: types.RiskHigh, Summary: "Write file", FileTargets: []string{params.Path}}, nil
		},
		Handler: func(_ context.Context, params WriteFileParams, env ExecutionEnv) (ExecutionResult, error) {
			target := resolvePath(env.Workspace, params.Path)
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return ExecutionResult{Success: false, Error: err.Error()}, err
			}
			if err := os.WriteFile(target, []byte(params.Content), 0o644); err != nil {
				return ExecutionResult{Success: false, Error: err.Error()}, err
			}
			return ExecutionResult{Success: true, Content: []types.ContentPart{{Type: "text", Text: fmt.Sprintf("Wrote %s", target)}}, Metadata: map[string]any{"path": target}}, nil
		},
	}
}

func BuiltinBash() Tool {
	return Spec[BashParams]{
		Info: Descriptor{
			Name:        "bash",
			Description: "Run a shell command",
			Category:    "terminal",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string", "description": "Command string executed with zsh -lc"}}, "required": []string{"command"}},
		},
		Parse: ParseJSON[BashParams],
		Permission: func(_ context.Context, params BashParams, _ ExecutionEnv) (permission.Request, error) {
			return permission.Request{ToolName: "bash", RiskLevel: types.RiskHigh, Summary: "Execute shell command", CommandPreview: params.Command}, nil
		},
		Handler: func(ctx context.Context, params BashParams, env ExecutionEnv) (ExecutionResult, error) {
			cmd := exec.CommandContext(ctx, "zsh", "-lc", params.Command)
			cmd.Dir = env.Workspace
			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output
			err := cmd.Run()
			if err != nil {
				return ExecutionResult{Success: false, Error: err.Error(), Content: []types.ContentPart{{Type: "text", Text: output.String()}}}, err
			}
			return ExecutionResult{Success: true, Content: []types.ContentPart{{Type: "text", Text: output.String()}}}, nil
		},
	}
}

func BuiltinAskUser() Tool {
	return Spec[AskUserParams]{
		Info: Descriptor{
			Name:        "ask_user",
			Description: "Request user input",
			Category:    "interaction",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"question": map[string]any{"type": "string", "description": "Question to show to the user"}}, "required": []string{"question"}},
		},
		Parse: ParseJSON[AskUserParams],
		Permission: func(_ context.Context, params AskUserParams, _ ExecutionEnv) (permission.Request, error) {
			return permission.Request{ToolName: "ask_user", RiskLevel: types.RiskLow, Summary: params.Question}, nil
		},
		Handler: func(_ context.Context, params AskUserParams, _ ExecutionEnv) (ExecutionResult, error) {
			return ExecutionResult{Success: true, Content: []types.ContentPart{{Type: "text", Text: params.Question}}}, nil
		},
	}
}

func resolvePath(workspace, input string) string {
	if filepath.IsAbs(input) {
		return input
	}
	return filepath.Join(workspace, input)
}
