# Symphony Go

A Go implementation of the [Symphony specification](../SPEC.md) — a long-running automation service that reads work from issue trackers, creates isolated workspaces, and runs AI coding agents for each issue.

Symphony supports two agent backends (**Gemini CLI** and **Claude Code**) and three issue trackers (**Linear**, **Jira Cloud**, and **GitHub Issues**). Mix and match per workflow.

## Architecture

![Symphony Go Architecture](symphony-go-architecture.png)

## Agent Backends

Symphony supports two agent backends. Set the `backend` field in your WORKFLOW.md to choose:

| | Gemini CLI | Claude Code |
|---|---|---|
| **Config key** | `backend: gemini` (default) | `backend: claude` |
| **Protocol** | ACP — JSON-RPC 2.0 over stdio (long-running process) | NDJSON stream — one CLI invocation per turn |
| **Session model** | Single process, session persists in-memory | `--resume <session_id>` across invocations, persisted to `.symphony-session-id` |
| **Tool access** | Client-side injection (ACP fs/terminal requests) | MCP servers (configured externally via `.mcp.json` or user config) |
| **Permission handling** | ACP `session/request_permission` auto-approve | `--permission-mode bypassPermissions` flag |
| **Default model** | `gemini-3.1-pro-preview` | `claude-sonnet-4-6` |
| **TTY requirement** | None | Requires pseudo-TTY (`script -q /dev/null` wrapper, handled automatically) |

### Gemini CLI Setup

```bash
npm install -g @google/gemini-cli
gemini auth login
```

For Linear integration, install the MCP extension:
```bash
gemini extensions install @google/mcp-linear
```

### Claude Code Setup

```bash
npm install -g @anthropic-ai/claude-code
```

For Linear integration, add the MCP server globally:
```bash
claude mcp add -s user --transport http linear-server https://mcp.linear.app/mcp
```

This makes the Linear MCP server available in all workspaces. Alternatively, write a `.mcp.json` file in the workspace via the `after_create` hook for a self-contained setup.

## Issue Trackers

Symphony supports **Linear**, **Jira Cloud**, and **GitHub Issues** as issue trackers. Set `tracker.kind` in your WORKFLOW.md.

### Why does Symphony need an API key?

Symphony has two separate connections to your tracker:

1. **Symphony's orchestrator (polling)** — The orchestrator polls the tracker API directly every 30 seconds to discover new issues, check which state running issues are in, and decide what to dispatch. This requires an **API key** because the orchestrator makes direct HTTP calls to the tracker's API.

2. **The agent (MCP tools)** — During its work session, the agent uses MCP tools to read/write issues (transitions, comments, etc.). The agent authenticates to the MCP server separately — this is configured once via the MCP setup commands and doesn't require the API key in WORKFLOW.md.

**Both are required.** The API key powers the orchestrator's polling loop. The MCP tools power the agent's interactions.

> **Recommendation:** Set your API keys as environment variables rather than hardcoding them in WORKFLOW.md. This keeps secrets out of version control and makes it easy to share workflow files across a team.

| | Linear | Jira Cloud | GitHub Issues |
|---|---|---|---|
| **Config key** | `tracker.kind: linear` | `tracker.kind: jira` | `tracker.kind: github` |
| **API** | GraphQL | REST API v3 | REST API v3 |
| **Auth** | API key | API token + email | Personal access token |
| **Project filter** | `tracker.project_slug` (slug ID from URL) | `tracker.project_slug` (Jira project key, e.g., `PROJ`) | `tracker.project_slug` (in `owner/repo` format) |
| **Endpoint** | Default: `https://api.linear.app/graphql` | Required (e.g., `https://mycompany.atlassian.net`) | Default: `https://api.github.com` |
| **Default active states** | `To Do`, `In Progress` | `To Do`, `In Progress` | `open` |
| **Default terminal states** | `Closed`, `Cancelled`, `Canceled`, `Duplicate`, `Done` | `Done` | `closed` |

### Linear Setup

1. Create an API key at **Linear Settings > API > Personal API keys**
2. Find your project slug from the URL: `https://linear.app/yourteam/project/my-project-abc123` → `my-project-abc123`
3. Set the environment variable (add to your `~/.zshrc` or `~/.bashrc` to persist):
   ```bash
   export LINEAR_API_KEY="lin_api_..."
   ```
4. Add MCP so the agent can interact with Linear during work:
   - **Gemini:** `gemini extensions install @google/mcp-linear`
   - **Claude Code:** `claude mcp add -s user --transport http linear-server https://mcp.linear.app/mcp`
5. In your WORKFLOW.md, reference the env var:
   ```yaml
   tracker:
     kind: linear
     api_key: $LINEAR_API_KEY
     project_slug: my-project-abc123
   ```

### Jira Setup

1. Generate an API token at https://id.atlassian.com/manage-profile/security/api-tokens
2. Find your project key from Jira (e.g., `PROJ` from issue keys like `PROJ-123`)
3. Set environment variables (add to your `~/.zshrc` or `~/.bashrc` to persist):
   ```bash
   export JIRA_API_TOKEN="your-api-token"
   export JIRA_EMAIL="your-email@company.com"
   export JIRA_ENDPOINT="https://mycompany.atlassian.net"
   ```
4. Add MCP so the agent can interact with Jira during work:
   - **Claude Code:** `claude mcp add -s user --transport http jira-server https://mcp.atlassian.com/v1/sse`
   - **Gemini:** configure a Jira MCP server in `~/.gemini/settings.json`
5. In your WORKFLOW.md, reference the env vars:
   ```yaml
   tracker:
     kind: jira
     endpoint: $JIRA_ENDPOINT
     api_key: $JIRA_API_TOKEN
     email: $JIRA_EMAIL
     project_slug: PROJ
   ```

### GitHub Issues Setup

1. Create a personal access token at **GitHub Settings > Developer settings > Personal access tokens**
   - Classic token: select the `repo` scope (or `public_repo` for public repositories only)
   - Fine-grained token: grant **Issues: Read and write** permission for the target repository
2. Set the environment variable (add to your `~/.zshrc` or `~/.bashrc` to persist):
   ```bash
   export GITHUB_TOKEN="ghp_..."
   ```
3. Optionally, add a GitHub MCP server so the agent can interact with issues during work:
   - Install the [GitHub MCP Server](https://github.com/github/github-mcp-server) and configure it for your agent backend
   - **Claude Code:** `claude mcp add -s user github -- github-mcp-server stdio`
   - **Gemini:** configure the GitHub MCP server in `~/.gemini/settings.json`
4. In your WORKFLOW.md, reference the env var:
   ```yaml
   tracker:
     kind: github
     api_key: $GITHUB_TOKEN
     project_slug: owner/repo
   ```

   Symphony defaults to `active_states: [open]` and `terminal_states: [closed]` for GitHub. Specify `active_states` and `terminal_states` explicitly only if you want to change these defaults or use label-based states (see below).

   > **Note:** Unlike Linear and Jira, GitHub has no automatic environment variable fallback. Always set `api_key: $GITHUB_TOKEN` explicitly in your WORKFLOW.md.

**Label-based states:** GitHub only has two native states — `open` and `closed`. Symphony lets you use GitHub **labels** as additional states to model a richer workflow. Any state name that is not `open` or `closed` is treated as a label name:

```yaml
tracker:
  kind: github
  api_key: $GITHUB_TOKEN
  project_slug: owner/repo
  active_states:
    - in-progress    # issues with the "in-progress" label are picked up
  terminal_states:
    - closed         # native GitHub closed state
    - done           # issues with the "done" label are stopped and cleaned up
```

Symphony filters issues by the configured labels server-side (via the GitHub API `labels` parameter) and resolves each issue's effective state from its labels. Terminal label states take priority over active label states if an issue carries both.

### Tracker Configuration Reference

```yaml
tracker:
  kind: jira                        # required: "linear", "jira", or "github"
  endpoint: $JIRA_ENDPOINT          # required for Jira; has default for Linear and GitHub
  api_key: $JIRA_API_TOKEN          # required: API key/token (supports $VAR)
  email: $JIRA_EMAIL                # required for Jira only (supports $VAR)
  project_slug: PROJ                # required: Linear slug ID, Jira project key, or owner/repo for GitHub
  active_states:                    # states that trigger agent work
    - To Do
    - In Progress
  terminal_states:                  # states that stop agents and clean workspaces
    - Done
```

| Field | Required | Description |
|---|---|---|
| `kind` | Always | `"linear"`, `"jira"`, or `"github"` |
| `endpoint` | Jira only | Jira Cloud base URL. Linear and GitHub have built-in defaults. |
| `api_key` | Always | API key (Linear), API token (Jira), or personal access token (GitHub). Use `$VAR` to reference env vars. Auto-fallback: `LINEAR_API_KEY` for linear, `JIRA_API_TOKEN` for jira. GitHub has no auto-fallback — always use `api_key: $GITHUB_TOKEN` explicitly. |
| `email` | Jira only | Atlassian account email for Basic Auth. Use `$VAR`. Falls back to `JIRA_EMAIL`. |
| `project_slug` | Always | Linear project slug ID, Jira project key (e.g., `PROJ`), or GitHub `owner/repo` (e.g., `myorg/myrepo`). |
| `active_states` | No | States that trigger agent work. Must match tracker exactly (case-sensitive). Defaults: Linear/Jira: `["Todo", "In Progress"]`; GitHub: `["open"]`. For GitHub, any value other than `open`/`closed` is treated as a label name. |
| `terminal_states` | No | States that stop agents and trigger workspace cleanup. Defaults: Linear: `["Closed", "Cancelled", "Canceled", "Duplicate", "Done"]`; Jira: `["Done"]`; GitHub: `["closed"]`. For GitHub, any value other than `open`/`closed` is treated as a label name. |

**State names must match your tracker exactly** (case-sensitive). Check your Linear workflow states or Jira project board settings.

**Jira state tips:**
- Jira status names often include spaces: `"To Do"`, `"In Progress"`, `"In Review"`
- Custom workflows may have different names — check your project's board columns
- Include all "done" statuses in `terminal_states` so Symphony cleans up finished work

**GitHub Issues state tips:**
- Symphony defaults to `active_states: [open]` and `terminal_states: [closed]` for GitHub. No explicit state config is needed for the standard open/closed workflow.
- **Label-based states:** Any value other than `open`/`closed` in `active_states` or `terminal_states` is treated as a GitHub **label name**. Symphony will filter issues by those labels via the GitHub API and expose the label name as the effective state. This lets you model richer workflows (e.g., `in-progress`, `review`, `done`) using labels.
- Pull requests returned by the GitHub Issues API are automatically skipped.

## Workflows & Customization

Symphony is driven by `WORKFLOW.md` files. You can create different workflow files for different strategies and backends.

### Included Workflows

| Workflow | File | Backend | Tracker | Strategy |
|---|---|---|---|---|
| **Autonomous** | `WORKFLOW.md` | Gemini | Linear | Full automation |
| **Planning First** | `WORKFLOW-PLAN.md` | Gemini | Linear | Human-in-the-loop |
| **Planning First (Claude)** | `WORKFLOW-PLAN-CLAUDE.md` | Claude Code | Linear | Human-in-the-loop |
| **Planning First (Jira)** | `WORKFLOW-PLAN-JIRA.md` | Claude Code | Jira | Human-in-the-loop |

#### Strategy Comparison

| Feature | Autonomous | Planning First |
|---|---|---|
| **Initial Action** | Moves to `In Progress` immediately | Analyzes code and creates a technical plan |
| **Approval Gate** | None — proceeds to implementation | Stops in `Plan Review` for human feedback |
| **Execution** | Continuous turn loop until PR | Only starts coding after move to `Plan Approved` |
| **Risk Profile** | High speed, less oversight | Higher quality, safe for sensitive codebases |

Both strategies work with either backend. The backend determines *which AI agent* runs. The workflow strategy determines *how* it runs (autonomous vs. human-gated).

### Creating Custom Workflows

You can tailor Symphony to any organizational need by creating a new `.md` file with a YAML header.

**Common customization ideas:**
- **Security Auditor**: A workflow that only runs security scans and reports findings to Linear comments.
- **Documentation Agent**: A workflow that focuses on updating `README` and `DOCS` based on code changes.
- **Issue Triage**: A workflow that analyzes new issues, adds labels, and suggests a priority without writing code.

To use a custom workflow:
```bash
./bin/symphony my-custom-workflow.md
```

## Prerequisites

1. **Go 1.25+** — [install](https://go.dev/dl/)

2. **An agent backend** — at least one of:
   - **Gemini CLI** — `npm install -g @google/gemini-cli && gemini auth login`
   - **Claude Code** — `npm install -g @anthropic-ai/claude-code` (requires Anthropic API key or Claude subscription)

3. **An issue tracker** — at least one of:
   - **Linear** — API key from Linear settings. Project slug from URL: `my-project-abc123`
   - **Jira Cloud** — API token + email. Project key (e.g., `PROJ`)
   - **GitHub Issues** — Personal access token with `repo` scope. Repository in `owner/repo` format.

4. **Tracker MCP** (for agent access to the tracker) — see [Issue Trackers](#issue-trackers) for setup

## Build

```bash
cd go/
make build
```

This produces `bin/symphony`.

## Configuration

All configuration lives in a single `WORKFLOW.md` file. The file has two parts:

1. **YAML front matter** (between `---` delimiters) — runtime settings
2. **Markdown body** — the prompt template sent to the agent for each issue

### Minimal WORKFLOW.md (Gemini)

```yaml
---
tracker:
  kind: linear
  project_slug: my-project-slug
gemini:
  command: "gemini --acp"
  model: gemini-3.1-pro-preview
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

{{ issue.description }}
```

### Minimal WORKFLOW.md (Claude Code)

```yaml
---
backend: claude
tracker:
  kind: linear
  project_slug: my-project-slug
claude:
  command: claude
  model: claude-sonnet-4-6
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

{{ issue.description }}
```

### Full WORKFLOW.md reference

```yaml
---
backend: gemini                           # "gemini" (default) or "claude"

tracker:
  kind: linear                          # required: "linear", "jira", or "github"
  project_slug: my-project              # required (Linear slug, Jira project key, or owner/repo for GitHub)
  endpoint: https://api.linear.app/graphql  # default for Linear; required for Jira; not needed for GitHub
  email: $JIRA_EMAIL                    # required for Jira (Basic Auth); ignored for Linear and GitHub
  active_states:                        # default: ["Todo", "In Progress"] for Linear/Jira; ["open"] for GitHub
                                        # GitHub: non-native values (not "open"/"closed") are treated as label names
    - Todo
    - In Progress
  terminal_states:                      # default: ["Closed", "Cancelled", "Canceled", "Duplicate", "Done"] for Linear; ["Done"] for Jira; ["closed"] for GitHub
                                        # GitHub: non-native values (not "open"/"closed") are treated as label names
    - Done
    - Closed
    - Cancelled

polling:
  interval_ms: 30000                    # default: 30000 (30s)

workspace:
  root: ~/symphony_workspaces           # default: <system-temp>/symphony_workspaces
                                        # supports ~ and $VAR

hooks:
  after_create: |                       # runs once when workspace dir is first created
    git clone git@github.com:org/repo.git .
  before_run: |                         # runs before each agent attempt
    git checkout main && git pull
  after_run: |                          # runs after each attempt (failures ignored)
    echo "run complete"
  before_remove: |                      # runs before workspace deletion (failures ignored)
    echo "cleaning up"
  timeout_ms: 60000                     # default: 60000 (60s), applies to all hooks

agent:
  max_concurrent_agents: 5              # default: 10
  max_turns: 10                         # default: 20, orchestrator-level turn loop
  max_retry_backoff_ms: 300000          # default: 300000 (5 min)
  max_concurrent_agents_by_state:       # optional per-state caps
    todo: 2
    in progress: 5

# --- Gemini backend config (used when backend: gemini) ---
gemini:
  command: "gemini --acp"               # default
  model: gemini-3.1-pro-preview         # default
  turn_timeout_ms: 3600000              # default: 3600000 (1 hour)
  read_timeout_ms: 5000                 # default: 5000 (5s)
  stall_timeout_ms: 300000              # default: 300000 (5 min), 0 disables

# --- Claude Code backend config (used when backend: claude) ---
claude:
  command: claude                       # default
  model: claude-sonnet-4-6              # default
  permission_mode: bypassPermissions    # default, auto-approves all tool use
  allowed_tools:                        # default: ["Read", "Write", "Edit", "Bash"]
    - Read
    - Write
    - Edit
    - Bash
    - "Bash(git *)"
  max_turns: 25                         # default: 25, per-invocation Claude turns
  turn_timeout_ms: 600000               # default: 600000 (10 min per invocation)
  stall_timeout_ms: 300000              # default: 300000 (5 min), 0 disables

server:
  port: 8080                            # optional, enables HTTP dashboard

# --- cmux visibility (macOS only) ---
cmux:
  enabled: true                         # default: false — opt-in
  workspace_name: "Symphony"            # default: "Symphony" (cosmetic, used for naming)
  close_delay_ms: 30000                 # default: 30000 (30s before closing finished tabs)
---
You are working on issue {{ issue.identifier }}: {{ issue.title }}.

{% if issue.description %}
## Description
{{ issue.description }}
{% endif %}

## Labels
{% for label in issue.labels %}- {{ label }}
{% endfor %}

{% if attempt %}
This is retry attempt {{ attempt }}. Check previous work and continue.
{% endif %}
```

### Template variables

The prompt body is rendered with [Liquid](https://shopify.github.io/liquid/) syntax. Available variables:

| Variable | Type | Description |
|---|---|---|
| `issue.id` | string | Tracker ID (Linear UUID or Jira key) |
| `issue.identifier` | string | Human-readable key (e.g., `MT-123` or `PROJ-456`) |
| `issue.title` | string | Issue title |
| `issue.description` | string | Issue description (empty if none) |
| `issue.state` | string | Current tracker state name |
| `issue.priority` | int or nil | Priority (1=urgent, 4=low, nil=none) |
| `issue.url` | string | Issue URL (Linear or Jira) |
| `issue.labels` | list of strings | Lowercase label names |
| `issue.branch_name` | string | Suggested branch name |
| `issue.blocked_by` | list of objects | Blocking issues (each has `.id`, `.identifier`, `.state`) |
| `issue.created_at` | string | ISO-8601 timestamp |
| `issue.updated_at` | string | ISO-8601 timestamp |
| `attempt` | int or nil | nil on first run, integer on retry/continuation |

### Environment variables

Symphony resolves `$VAR_NAME` references in config fields at startup. You can also rely on automatic fallbacks:

| Variable | Used by | Purpose |
|---|---|---|
| `LINEAR_API_KEY` | Linear tracker | Auto-fallback for `tracker.api_key` when kind is `linear` |
| `JIRA_API_TOKEN` | Jira tracker | Auto-fallback for `tracker.api_key` when kind is `jira` |
| `JIRA_EMAIL` | Jira tracker | Auto-fallback for `tracker.email` when kind is `jira` |

**Best practice:** Add these to your shell profile (`~/.zshrc` or `~/.bashrc`) so they persist across terminal sessions:

```bash
# Linear
export LINEAR_API_KEY="lin_api_..."

# Jira
export JIRA_API_TOKEN="your-token"
export JIRA_EMAIL="you@company.com"
export JIRA_ENDPOINT="https://mycompany.atlassian.net"

# GitHub
export GITHUB_TOKEN="ghp_..."
```

Then reference them in WORKFLOW.md with `$VAR` syntax: `api_key: $GITHUB_TOKEN`. For Linear and Jira, you can omit the field entirely and let the auto-fallback pick up the env var. For GitHub, always reference the token explicitly with `api_key: $GITHUB_TOKEN`.

## Run

```bash
# Default: looks for ./WORKFLOW.md
./bin/symphony

# Explicit workflow path
./bin/symphony WORKFLOW-PLAN.md

# With HTTP dashboard
./bin/symphony --port 8080

# CLI --port overrides server.port in WORKFLOW.md
./bin/symphony --port 9090 /path/to/WORKFLOW.md

# Version
./bin/symphony --version
```

The service runs until stopped with `Ctrl+C` (SIGINT) or `SIGTERM`.

## HTTP Dashboard & API

When a port is configured (via `--port` flag or `server.port` in config):

| Endpoint | Method | Description |
|---|---|---|
| `/` | GET | HTML dashboard (auto-refreshes every 5s) |
| `/api/v1/state` | GET | JSON system state: running sessions, retry queue, token totals |
| `/api/v1/{identifier}` | GET | JSON detail for a specific issue (e.g., `/api/v1/MT-123`) |
| `/api/v1/refresh` | POST | Trigger an immediate poll + reconciliation cycle |

The server binds to `127.0.0.1` (localhost only).

## cmux Session Visibility

When running on macOS with [cmux](https://cmux.dev), Symphony can show **live, color-coded agent output** in dedicated terminal workspaces — one per dispatched issue.

### What you see

Each active issue gets its own cmux workspace (tab) named after the issue identifier (e.g., `AIE-12`). Inside, a live stream shows what the agent is doing:

```
 16:42:33  ── ACP initialized — agent: gemini, protocol: 1 ──
 16:42:33  ── Starting turn 1 of 15 ──
 16:42:35  TOOL   src/app/page.tsx  in_progress
 16:42:35  FAIL   File not found: /Users/sascha/...
 16:42:37  THINK  Examining the Codebase...
 16:42:37  AGENT  I'll list the contents of the src/app directory...
 16:42:38  TOOL   list_directory: src/app  completed
 16:42:45  ── Turn 1 completed — end_turn ──
```

Events are color-coded: `TOOL` in yellow, `AGENT` in cyan, `THINK` in gray, `FAIL` in red, `DONE` in green. Timestamps are dim. Annotations (turn start/end, session info) appear as highlighted separator lines.

### Enabling cmux visibility

Add the `cmux` section to your WORKFLOW.md config:

```yaml
cmux:
  enabled: true
```

That's it. Symphony will:
1. Detect the cmux binary and verify connectivity on startup
2. Create a cmux workspace per dispatched issue, running `tail -f` on the agent's event log
3. Rename each workspace to the issue identifier (e.g., `AIE-12`)
4. Stream formatted events as the agent works
5. Close the workspace 30 seconds after the agent finishes (configurable via `close_delay_ms`)

### How it works under the hood

- Agent processes still run as hidden subprocesses with pipe-based I/O — cmux is a **display-only mirror**, not part of the process lifecycle
- As the runner receives protocol events (ACP JSON-RPC for Gemini, NDJSON for Claude), it writes formatted lines to `<workspace>/.symphony-agent.log`
- Each cmux workspace runs `tail -f` on that log file for real-time display
- Works uniformly for both Gemini and Claude Code backends
- If cmux is not installed or not running, Symphony continues normally with zero impact

### Configuration

| Field | Default | Description |
|---|---|---|
| `cmux.enabled` | `false` | Enable cmux visibility (opt-in) |
| `cmux.workspace_name` | `"Symphony"` | Display name (cosmetic) |
| `cmux.close_delay_ms` | `30000` | Milliseconds to keep workspace open after agent finishes |

### Requirements

- **macOS only** — cmux is a native macOS application
- cmux must be installed and running (socket at `/tmp/cmux.sock`)
- cmux binary must be in PATH or at `/Applications/cmux.app/Contents/Resources/bin/cmux`

## How it works

1. **Poll** — Every `polling.interval_ms`, fetch candidate issues from the configured tracker (Linear or Jira)
2. **Dispatch** — Sort by priority, check eligibility (concurrency, blockers), launch workers
3. **Worker** — Create workspace → run hooks → launch agent (Gemini or Claude) → multi-turn session
4. **Reconcile** — Each tick, check tracker state for running issues (stop on terminal, update on active)
5. **Retry** — Normal exit → 1s continuation retry; failure → exponential backoff (10s base, capped)
6. **Reload** — `WORKFLOW.md` changes are detected and applied without restart (backend change requires restart)

### Workspace lifecycle

```
<workspace.root>/
  MT-123/          ← one directory per issue
  MT-124/
  MT-125/
```

- Created on first dispatch, reused on retries
- `after_create` hook runs once (e.g., git clone)
- `before_run` hook runs before each attempt (e.g., git pull)
- Cleaned up when issue enters a terminal state

### Agent protocols

**Gemini CLI (ACP)** — Long-running JSON-RPC 2.0 process over stdio:

```
Symphony ──initialize──▶ Gemini CLI
         ◀──result──────
         ──session/new──▶
         ◀──sessionId───
         ──session/prompt──▶
         ◀──session/update── (streaming)
         ◀──prompt result──
```

Permission requests (`session/request_permission`) are auto-approved (high-trust mode).

**Claude Code (NDJSON)** — One CLI invocation per turn with streaming JSON output:

```
Symphony ── claude -p "<prompt>" --output-format stream-json ──▶ Claude Code
         ◀── {"type":"system","subtype":"init","session_id":"..."} ──
         ◀── {"type":"assistant","message":{...}} ──  (streaming)
         ◀── {"type":"result","subtype":"success",...} ──
         (process exits)

Next turn:
Symphony ── claude -p "<prompt>" --resume <session_id> ──▶ Claude Code
         ◀── ... ──
```

Session continuity is maintained via `--resume <session_id>`. The session ID is persisted to `.symphony-session-id` in the workspace directory.

Claude Code requires a TTY to produce `stream-json` output. Symphony wraps the process in `script -q /dev/null` to allocate a pseudo-TTY automatically.

## Writing the Prompt (Instructing the Agent)

The prompt in `WORKFLOW.md` is the **only way you control what the agent does**. Symphony is a scheduler — it picks up issues, creates workspaces, and launches the agent. Everything else is determined by your prompt.

The same prompt works with both Gemini and Claude Code. The agents have different capabilities, but both can read/write files, run shell commands, and use MCP tools.

### What to include in your prompt

1. **What it's working on** — use template variables like `{{ issue.identifier }}` and `{{ issue.title }}`
2. **Where the code is** — mention the repo so the agent understands the context
3. **What steps to follow** — be explicit about branching, committing, pushing, PR creation
4. **How to interact with the tracker** — tell the agent which MCP tools to use for state transitions and comments
5. **What to do when done** — move issue to review, create a PR, etc.

### Tracker-specific prompt guidance

The prompt template is the same regardless of tracker, but the agent instructions should reference the correct MCP tools:

**Linear workflows:**
```markdown
Use the Linear MCP tools for ALL Linear operations:
- `mcp_linear_update_issue` to transition states
- `mcp_linear_create_comment` / `mcp_linear_update_comment` for workpad
- `mcp_linear_list_comments` to find existing comments
```

**Jira workflows:**
```markdown
Use the Jira MCP tools for ALL Jira operations:
- Use the Jira MCP to transition issue status (e.g., move to "In Progress")
- Use the Jira MCP to add and update comments
- Use the Jira MCP to read issue details and existing comments
```

**GitHub Issues workflows:**
```markdown
Use the GitHub MCP tools for ALL GitHub operations:
- Use the GitHub MCP to update issue state (open/closed)
- Use the GitHub MCP to add and remove labels (e.g., add "in-progress", remove "in-progress" and add "done")
- Use the GitHub MCP to add comments to the issue
- Use the GitHub MCP to read issue details and existing comments
- The issue identifier is in owner/repo#number format (e.g., myorg/myrepo#42)
```

The template variables (`{{ issue.identifier }}`, `{{ issue.title }}`, etc.) work identically for all trackers — Symphony normalizes the data before rendering. For GitHub Issues, `{{ issue.identifier }}` produces the `owner/repo#number` format (e.g., `myorg/myrepo#42`).

### Example: Full workflow prompt

```markdown
You are working on issue {{ issue.identifier }}: {{ issue.title }}.
You are working in a checkout of https://github.com/your-org/your-repo.

{% if issue.description %}
## Description
{{ issue.description }}
{% endif %}

## Instructions
1. Make the code changes needed to resolve this issue.
2. Move the issue to `In Progress` using the tracker MCP tools.
3. Create a new branch: `git checkout -b {{ issue.identifier }}`
4. Commit your changes with a clear message referencing the issue.
5. Push the branch: `git push origin {{ issue.identifier }}`
6. Create a pull request:
   `gh pr create --title "{{ issue.identifier }}: {{ issue.title }}" --body "Resolves {{ issue.identifier }}"`
7. Add the PR link to the issue as a comment.
8. Move the issue to `Human Review`.

When you are done, do NOT leave the issue in an active state.
The issue will be picked up again if it stays active.

{% if attempt %}
This is retry attempt {{ attempt }}. Check previous work and continue.
{% endif %}
```

### Key principles

- **Be explicit.** The agent does what you tell it. If you don't say "create a PR", it won't.
- **Use tracker MCP tools.** Both backends discover MCP tools automatically — Gemini via extensions, Claude Code via user-scoped config. Tell the agent to use them for state transitions and comments.
- **Use `gh` CLI for PRs.** If `gh` is installed and authenticated on the machine, the agent can create PRs directly.
- **Handle retries.** Use `{% if attempt %}` to give different instructions on retry.
- **State names matter.** The states you reference in the prompt (e.g., "In Progress", "Human Review") must match your tracker's workflow exactly.

### Workspace location

Each issue gets its own directory under `workspace.root`:

```
~/symphony_workspaces/
  AIE-7/          ← cloned repo for issue AIE-7
  AIE-8/          ← cloned repo for issue AIE-8
```

## Development

```bash
# Run tests
make test

# Build
make build

# Run directly
make run
```

### Project structure

```
├── cmd/symphony/main.go          # CLI entrypoint
├── internal/
│   ├── config/                   # Typed config + defaults + validation
│   ├── workflow/                 # WORKFLOW.md parser + file watcher
│   ├── tracker/                  # Issue tracker clients (Linear, Jira, GitHub Issues)
│   ├── orchestrator/             # Poll loop, dispatch, reconcile, retry
│   ├── workspace/                # Directory lifecycle + hooks + safety
│   ├── agent/                    # Backend runners + protocol clients
│   │   ├── runner.go             # AgentLauncher interface + factory
│   │   ├── acp.go                # Gemini ACP client (JSON-RPC over stdio)
│   │   ├── claude_runner.go      # Claude Code runner (NDJSON streaming)
│   │   ├── ndjson.go             # NDJSON line-accumulator parser
│   │   └── events.go             # Event types for orchestrator
│   ├── cmux/                     # cmux session visibility (macOS)
│   ├── prompt/                   # Liquid template rendering
│   ├── server/                   # HTTP dashboard + JSON API
│   └── logging/                  # slog JSON setup
├── Makefile
├── go.mod
└── go.sum
```
