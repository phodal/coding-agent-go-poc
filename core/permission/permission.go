package permission

import (
	"context"
	"fmt"

	"agent-platform/core/types"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

type Request struct {
	ID             string          `json:"id"`
	ToolName       string          `json:"toolName"`
	RiskLevel      types.RiskLevel `json:"riskLevel"`
	Summary        string          `json:"summary"`
	CommandPreview string          `json:"commandPreview,omitempty"`
	FileTargets    []string        `json:"fileTargets,omitempty"`
	Reason         string          `json:"reason,omitempty"`
}

type Response struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason,omitempty"`
}

type Handler func(context.Context, Request) (Response, error)

func DefaultHandler(_ context.Context, req Request) (Response, error) {
	if req.RiskLevel == types.RiskLow {
		return Response{Decision: DecisionAllow}, nil
	}
	return Response{Decision: DecisionDeny, Reason: fmt.Sprintf("tool %s requires explicit approval", req.ToolName)}, nil
}
