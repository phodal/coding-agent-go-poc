package types

import "time"

type ResourceLocation string

const (
	LocationBuiltin ResourceLocation = "builtin"
	LocationGlobal  ResourceLocation = "global"
	LocationProject ResourceLocation = "project"
	LocationRuntime ResourceLocation = "runtime"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type StopReason string

const (
	StopCompletion      StopReason = "completion"
	StopQuestionMode    StopReason = "question_mode"
	StopWaitingMode     StopReason = "waiting_mode"
	StopSummaryMode     StopReason = "summary_mode"
	StopExplanationMode StopReason = "explanation_mode"
	StopUncertaintyMode StopReason = "uncertainty_mode"
)

type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Message struct {
	ID          string            `json:"id"`
	Role        string            `json:"role"`
	Content     []ContentPart     `json:"content"`
	CreatedAt   time.Time         `json:"createdAt"`
	ToolCalls   []ToolCall        `json:"toolCalls,omitempty"`
	ToolResults []ToolResult      `json:"toolResults,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args []byte `json:"args"`
}

type ToolResult struct {
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Success    bool           `json:"success"`
	Content    []ContentPart  `json:"content"`
	Error      string         `json:"error,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type AgentDefinition struct {
	Name         string           `yaml:"name" json:"name"`
	Description  string           `yaml:"description" json:"description"`
	Model        string           `yaml:"model" json:"model"`
	Tools        []string         `yaml:"tools" json:"tools"`
	SystemPrompt string           `yaml:"systemPrompt" json:"systemPrompt"`
	Location     ResourceLocation `yaml:"-" json:"location"`
	StoragePath  string           `yaml:"-" json:"storagePath"`
	Color        string           `yaml:"color" json:"color"`
	Labels       []string         `yaml:"labels" json:"labels"`
	NeedAskUser  bool             `yaml:"ask-user" json:"needAskUser"`
}

type SkillDefinition struct {
	Name         string           `yaml:"name" json:"name"`
	Description  string           `yaml:"description" json:"description"`
	AllowedTools []string         `yaml:"allowed-tools" json:"allowedTools"`
	Labels       []string         `yaml:"labels" json:"labels"`
	Content      string           `yaml:"content" json:"content"`
	Location     ResourceLocation `yaml:"-" json:"location"`
	StoragePath  string           `yaml:"-" json:"storagePath"`
}

type AgentState struct {
	CurrentAgent      string         `json:"currentAgent"`
	CurrentMode       string         `json:"currentMode"`
	CurrentStopReason StopReason     `json:"currentStopReason"`
	Iteration         int            `json:"iteration"`
	MaxIterations     int            `json:"maxIterations"`
	BusinessStage     string         `json:"businessStage"`
	RecoveryCount     int            `json:"recoveryCount"`
	RecoveryLimit     int            `json:"recoveryLimit"`
	ToolBreakdown     map[string]int `json:"toolBreakdown"`
}

type Session struct {
	ID              string            `json:"id"`
	Workspace       string            `json:"workspace"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
	Messages        []Message         `json:"messages"`
	AgentState      AgentState        `json:"agentState"`
	CurrentPlanPath string            `json:"currentPlanPath,omitempty"`
	CurrentSpecPath string            `json:"currentSpecPath,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}
