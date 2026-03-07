# Agent Platform MVP

This directory contains a Go MVP implementation of the Agent Platform design derived from the qodercli analysis.

## Implemented

- Cobra-based CLI entry points
- Runtime loop with provider abstraction
- OpenAI-compatible chat completions provider with function tool-calling
- Typed tool registry with permission checks
- Built-in tools: `read_file`, `write_file`, `bash`, `ask_user`
- Project/global resource loading for agents and skills
- Session persistence under `.agent-platform/sessions`
- Session memory summaries under `.agent-platform/sessions/<id>/session_memory.md`
- MCP server registry plus mcp-go tool bridge
- Local job registry
- Spec engine that emits `requirements.md`, `design.md`, `tasks.md`, and `checkpoint.json`
- Interactive permission approval flow

## Commands

```bash
go run .
go run . start -p "read README.md"
go run . start -p 'write notes.txt::hello world' --yes
go run . start -p 'bash pwd' --yes
go run . start -p 'Summarize README.md' --agent reviewer
go run . status
go run . jobs
go run . mcp add demo http://localhost:8000/sse -t sse
go run . mcp list
go run . spec -p 'Design a tool-calling CLI agent'
```

## OpenAI-Compatible Provider

Set these environment variables before using a remote model:

```bash
export AGENT_PLATFORM_OPENAI_BASE_URL=http://127.0.0.1:8000/v1
export AGENT_PLATFORM_OPENAI_API_KEY=your-api-key
export AGENT_PLATFORM_OPENAI_MODEL=glm-4.7
```

If an agent `model` is not `local`, the CLI will default to the `openai` provider automatically.

## Resource Directories

Global:

- `~/.agent-platform/agents`
- `~/.agent-platform/skills`

Project:

- `.agent-platform/agents`
- `.agent-platform/skills`

## Example Agent

```yaml
name: builder
description: Local build agent
model: local
tools:
  - read_file
  - write_file
  - bash
systemPrompt: You are a local build agent.
```

Remote model example:

```yaml
name: reviewer
description: Remote review agent
model: glm-4.7
tools:
  - read_file
systemPrompt: You are a precise code review agent.
```
