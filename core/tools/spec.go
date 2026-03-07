package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"agent-platform/core/permission"
	"agent-platform/core/types"
)

type ExecutionEnv struct {
	Workspace string
	SessionID string
}

type Descriptor struct {
	Name        string
	Description string
	Category    string
	InputSchema map[string]any
}

type ExecutionResult struct {
	Success    bool
	Content    []types.ContentPart
	Metadata   map[string]any
	DurationMs int64
	Error      string
	Retryable  bool
}

type Tool interface {
	Descriptor() Descriptor
	EvaluatePermission(ctx context.Context, rawArgs []byte, env ExecutionEnv) (permission.Request, error)
	ExecuteRaw(ctx context.Context, rawArgs []byte, env ExecutionEnv) (ExecutionResult, error)
}

type Spec[P any] struct {
	Info       Descriptor
	Parse      func([]byte) (P, error)
	Permission func(context.Context, P, ExecutionEnv) (permission.Request, error)
	Handler    func(context.Context, P, ExecutionEnv) (ExecutionResult, error)
}

func (s Spec[P]) Descriptor() Descriptor {
	return s.Info
}

func (s Spec[P]) EvaluatePermission(ctx context.Context, rawArgs []byte, env ExecutionEnv) (permission.Request, error) {
	params, err := s.Parse(rawArgs)
	if err != nil {
		return permission.Request{}, err
	}
	return s.Permission(ctx, params, env)
}

func (s Spec[P]) ExecuteRaw(ctx context.Context, rawArgs []byte, env ExecutionEnv) (ExecutionResult, error) {
	started := time.Now()
	params, err := s.Parse(rawArgs)
	if err != nil {
		return ExecutionResult{Success: false, Error: err.Error()}, err
	}
	result, err := s.Handler(ctx, params, env)
	result.DurationMs = time.Since(started).Milliseconds()
	return result, err
}

func ParseJSON[P any](raw []byte) (P, error) {
	var params P
	if len(raw) == 0 {
		return params, nil
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("parse tool params: %w", err)
	}
	return params, nil
}
