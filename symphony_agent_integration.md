# Symphony Go ↔ Mini-Agent Integration Guide

This guide documents how Symphony Go orchestrates work using **Mini-Agent**
([shaunie2fly/Mini-Agent](https://github.com/shaunie2fly/Mini-Agent)) as its agent
backend. Mini-Agent is an ACP-compatible agent powered by the **MiniMax M2.5** model.

---

## Overview

Symphony Go supports three agent backends. Mini-Agent is the `mini_agent` backend:

| Backend | Config key | Protocol | Model |
|---------|-----------|---------|-------|
| Gemini CLI | `backend: gemini` | ACP / JSON-RPC over stdio | Gemini |
| Claude Code | `backend: claude` | NDJSON stream | Claude |
| **Mini-Agent** | **`backend: mini_agent`** | **ACP / JSON-RPC over stdio** | **MiniMax M2.5** |

Both Gemini and Mini-Agent use the **Agent Communication Protocol (ACP)** — a
JSON-RPC 2.0 framing over subprocess stdin/stdout. The Go `ACPClient` in
`internal/agent/acp.go` handles both.

---

## Prerequisites

### 1. Install Mini-Agent

```bash
# Install via uv (recommended)
uv tool install git+https://github.com/shaunie2fly/Mini-Agent.git

# Verify
mini-agent-acp --help   # should print usage
mini-agent --version
```

This installs two entry points:
- `mini-agent` — interactive / non-interactive CLI
- `mini-agent-acp` — **ACP stdio server** (used by Symphony)

### 2. Get a MiniMax API Key

1. Register at [https://platform.minimax.io](https://platform.minimax.io) (global)
   or [https://platform.minimaxi.com](https://platform.minimaxi.com) (China)
2. Go to **Account Management → API Keys → Create New Key**
3. Copy the key (it is only shown once)

Set it as an environment variable:

```bash
export MINIMAX_API_KEY="your-key-here"
```

Or configure it directly in your `WORKFLOW.md` (see below).

---

## Quick Start

Minimal `WORKFLOW.md` for Mini-Agent:

```yaml
---
backend: mini_agent
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: my-project-abc123
mini_agent:
  api_key: $MINIMAX_API_KEY
workspace:
  root: ~/symphony_workspaces
hooks:
  after_create: |
    git clone https://github.com/your-org/your-repo.git .
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.
...
```

Run Symphony:

```bash
symphony WORKFLOW.md
```

---

## Configuration Reference

All Mini-Agent settings live under the `mini_agent:` key in `WORKFLOW.md`:

```yaml
mini_agent:
  command: "mini-agent-acp"       # ACP entry point (default)
  model: "MiniMax-M2.5"           # model passed to Mini-Agent's config
  api_key: $MINIMAX_API_KEY       # MiniMax API key (supports $VAR)
  api_base: "https://api.minimax.io"  # API endpoint (global default)
  turn_timeout_ms: 3600000        # per-turn timeout (default: 60 min)
  read_timeout_ms: 5000           # ACP handshake read timeout (default: 5 s)
  stall_timeout_ms: 300000        # stall detection (default: 5 min)
```

| Field | Default | Description |
|-------|---------|-------------|
| `command` | `mini-agent-acp` | The ACP server binary. Use `mini-agent-acp` when installed via `uv tool install`, or `./mini_agent/acp/server.py` in development mode. |
| `model` | `MiniMax-M2.5` | MiniMax model name written into the workspace config. |
| `api_key` | _(none)_ | MiniMax API key. When set, Symphony writes a `mini_agent/config/config.yaml` into the workspace. Supports `$VAR` env-var substitution. |
| `api_base` | `https://api.minimax.io` | API endpoint. Use `https://api.minimaxi.com` for the China platform. |
| `turn_timeout_ms` | `3600000` | Maximum milliseconds per turn (hard deadline). |
| `read_timeout_ms` | `5000` | Milliseconds to wait for the ACP handshake (`initialize`). |
| `stall_timeout_ms` | `300000` | Stall detection handled by orchestrator reconciliation. |

### China platform

```yaml
mini_agent:
  api_key: $MINIMAX_API_KEY
  api_base: "https://api.minimaxi.com"
```

---

## How the Integration Works

### Launch sequence

```
orchestrator.dispatchIssue()
  │
  └─ MiniAgentRunner.Launch()
       │
       ├─ 1. WorkspaceMgr.CreateForIssue()   — create/reuse workspace dir
       ├─ 2. ValidateWorkspacePath()          — safety check
       ├─ 3. WorkspaceMgr.RunBeforeRun()      — before_run hook
       │
       ├─ 4. writeMiniAgentConfig()           — write mini_agent/config/config.yaml
       │      (only when mini_agent.api_key is configured)
       │
       ├─ 5. writeSymphonyContext()           — write .symphony-context.json
       │      { issue: {id, identifier, title, state},
       │        callback_url: "http://localhost:<port>/api/internal/agent/callback" }
       │
       ├─ 6. NewACPClient("mini-agent-acp", ws.Path, extraEnv)
       │      — spawns subprocess via bash -lc
       │
       ├─ 7. client.Initialize()             — ACP handshake
       ├─ 8. client.SessionNew(ws.Path)      — creates session, sets cwd=workspace
       │
       └─ 9. Turn loop (up to agent.max_turns):
              ├─ buildTurnPrompt()           — render issue prompt
              ├─ client.SessionPrompt()      — send prompt, stream updates
              └─ check stopReason:
                   "end_turn"          → continue (check issue state, next turn)
                   "refusal"           → stop (non-continuable)
                   "cancelled"         → stop (non-continuable)
                   "max_turn_requests" → stop (Mini-Agent hit its step limit)
```

### Config file injection

When `mini_agent.api_key` is set in `WORKFLOW.md`, Symphony writes:

```
<workspace>/mini_agent/config/config.yaml
```

This is the **highest-priority** config location Mini-Agent searches when launched
from the workspace directory. Content:

```yaml
api_key: "your-key"
api_base: "https://api.minimax.io"
model: "MiniMax-M2.5"
```

If `api_key` is **not** set in `WORKFLOW.md`, Mini-Agent falls back to the user's
global config at `~/.mini-agent/config/config.yaml`.

### Symphony context file

Symphony writes `.symphony-context.json` to the workspace root before each launch:

```json
{
  "issue": {
    "id": "...",
    "identifier": "PROJ-42",
    "title": "Fix the widget",
    "state": "In Progress"
  },
  "callback_url": "http://localhost:8080/api/internal/agent/callback"
}
```

Mini-Agent's **`SymphonyStatusUpdateTool`** reads this file (via `--context-file`
in CLI mode) to:
1. Inject the issue identifier into its system prompt
2. POST a completion callback to Symphony when the task is done

> **ACP mode note:** `mini-agent-acp` does not automatically read `--context-file`
> in its current version. The context file is primarily used when invoking Mini-Agent
> in CLI mode (`mini-agent --task "..." --context-file .symphony-context.json`).
> The ACP integration still benefits from the turn-based completion signals
> (`stopReason: "end_turn"`).

### Agent callback endpoint

Mini-Agent's `SymphonyStatusUpdateTool` POSTs to the URL stored in `.symphony-context.json`. Symphony sets this automatically from the configured server port:

```
POST http://localhost:<server.port>/api/internal/agent/callback
Content-Type: application/json

{"status": "completed", "message": "Created PR #42 and moved issue to Human Review"}
```

Symphony's HTTP server accepts this at `/api/internal/agent/callback` and triggers
an immediate reconcile tick so the orchestrator re-checks issue states without
waiting for the next poll interval.

**Status values:**

| Status | Meaning |
|--------|---------|
| `completed` | Agent finished the task successfully |
| `failed` | Agent encountered an unrecoverable error |
| `blocked` | Agent is blocked by missing secrets or permissions |

---

## ACP Protocol Details

Mini-Agent implements the same ACP protocol as Gemini CLI. Symphony's `ACPClient`
handles both transparently.

### Messages Symphony sends

| Method | When |
|--------|------|
| `initialize` | On startup — protocol handshake |
| `session/new` | Once per run — creates agent session with `cwd=<workspace>` |
| `session/prompt` | Once per turn — delivers the issue prompt |
| `session/cancel` | On context cancellation |

### Messages Symphony receives

| Method | When |
|--------|------|
| `session/update` | During a turn — tool calls, thinking, agent messages |

### Stop reasons Mini-Agent returns

| `stopReason` | Symphony action |
|-------------|----------------|
| `end_turn` | Continue (check issue state, then next turn or stop) |
| `refusal` | Stop immediately, no retry |
| `cancelled` | Stop immediately, no retry |
| `max_turn_requests` | Stop immediately (Mini-Agent hit its `max_steps` limit) |

---

## Example WORKFLOW.md Files

### Minimal (relies on global Mini-Agent config)

```yaml
---
backend: mini_agent
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: my-project-abc123
agent:
  max_concurrent_agents: 2
  max_turns: 15
workspace:
  root: ~/symphony_workspaces
hooks:
  after_create: |
    git clone https://github.com/your-org/your-repo.git .
server:
  port: 8080
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

...
```

### Full (Symphony manages Mini-Agent credentials)

```yaml
---
backend: mini_agent
tracker:
  kind: linear
  api_key: $LINEAR_API_KEY
  project_slug: my-project-abc123
  active_states:
    - Todo
    - In Progress
    - Rework
mini_agent:
  command: "mini-agent-acp"
  model: "MiniMax-M2.5"
  api_key: $MINIMAX_API_KEY
  api_base: "https://api.minimax.io"
  turn_timeout_ms: 1800000    # 30 min per turn
  read_timeout_ms: 10000
agent:
  max_concurrent_agents: 3
  max_turns: 20
workspace:
  root: ~/symphony_workspaces
hooks:
  after_create: |
    git clone https://github.com/your-org/your-repo.git .
  before_run: |
    git fetch origin main 2>/dev/null
    CURRENT_BRANCH=$(git branch --show-current 2>/dev/null)
    if [ "$CURRENT_BRANCH" = "main" ] || [ -z "$CURRENT_BRANCH" ]; then
      git checkout main && git pull
    fi
  timeout_ms: 120000
server:
  port: 8080
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

## Instructions
...
```

---

## Troubleshooting

### `mini-agent-acp: command not found`

Mini-Agent is not installed or not on `PATH`. Install it:

```bash
uv tool install git+https://github.com/shaunie2fly/Mini-Agent.git
# Ensure uv tool bin directory is on your PATH:
export PATH="$HOME/.local/bin:$PATH"
```

### `ACP initialize failed`

Likely causes:
- `mini-agent-acp` exited immediately because the config file is missing or has
  an invalid API key. Check `~/.mini-agent/config/config.yaml` or set
  `mini_agent.api_key` in `WORKFLOW.md`.
- The binary path is wrong. Test manually: `mini-agent-acp` in the terminal.

### `Please configure a valid API Key`

Mini-Agent found a config file but `api_key` is `YOUR_API_KEY_HERE`. Either:
- Set `mini_agent.api_key: $MINIMAX_API_KEY` in `WORKFLOW.md` (Symphony will
  write the config), or
- Edit `~/.mini-agent/config/config.yaml` with your real key.

### Agent hits `max_turn_requests`

Mini-Agent reached its internal `max_steps` limit before completing the task.
Increase `max_steps` in Mini-Agent's config:

```yaml
# ~/.mini-agent/config/config.yaml
max_steps: 200
```

Or break the task into smaller sub-issues.

### Callback not received

The Symphony callback endpoint (`/api/internal/agent/callback`) is only called
when Mini-Agent runs with `--context-file` (CLI mode). In ACP mode, it is not
currently invoked automatically. This is expected; task completion is signalled
via `stopReason: "end_turn"` instead.

---

## Related Documentation

- [Mini-Agent README](https://github.com/shaunie2fly/Mini-Agent/blob/main/README.md)
- [MiniMax API](https://platform.minimax.io/docs)
- [Symphony Go README](./README.md)
- [Upgrade Plan](./upgrade-plan.md)
