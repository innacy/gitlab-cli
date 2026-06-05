# git-agent

An interactive REPL for GitLab — manage merge requests, pipelines, and tickets from your terminal, with AI assistance baked in.

```
  git-agent — Command Reference

  Merge Requests
    mr-status <project>                  List open MRs
    mr-review <project> [mr]             AI-powered MR review
    ...

  Any unrecognized input is sent to AI as a question.
  Tab = auto-complete | Up/Down = history | Ctrl+R = search history
```

---

## Features

- **AI-powered MR review** — structured reviews (summary, quality, security, performance, suggestions) using Claude, Gemini, or NVIDIA models
- **Full MR lifecycle** — open, merge, approve, rebase, close, reopen
- **Pipeline management** — list, inspect, tail logs, retry, cancel
- **Ticket/issue workflow** — create, update, search, close; generate descriptions from diffs
- **Content generation** — AI-drafted ticket content, epic descriptions, and MR descriptions from git diffs
- **All-in-one workflow** — ticket → folder → commit → push → MR in one command
- **Free-form AI chat** — any input that isn't a command is forwarded to the AI as a question
- **Tab completion** — project names and commands auto-complete
- **History** — Up/Down navigation, Ctrl+R reverse search

---

## Requirements

- Go 1.21+
- A GitLab instance (self-hosted or gitlab.com)
- An API key for at least one AI provider: [Anthropic](https://console.anthropic.com/settings/keys), [Google Gemini](https://aistudio.google.com/apikey), or [NVIDIA](https://build.nvidia.com/)

---

## Installation

```bash
# Build binary into bin/
make build

# Or install to $GOPATH/bin
make install

# Run directly
make run
```

The binary is placed at `bin/gitlab-ai`.

### Cross-compilation

```bash
make build-linux    # linux/amd64
make build-darwin   # darwin/amd64 + darwin/arm64
make build-windows  # windows/amd64
make build-all      # all of the above
```

---

## Configuration

Copy the default config and fill in your values:

```bash
cp configs/default.yaml config.yaml
```

`config.yaml` is gitignored — never commit it.

### Minimal setup

```yaml
gitlab:
  base_url: "https://gitlab.example.com/"

ai:
  provider: "anthropic"   # or "gemini" or "nvidia"
  anthropic:
    api_key: ""           # or set ANTHROPIC_API_KEY env var

teams:
  - "my-team"
```

### AI providers

| Provider | `provider` value | Key env var | Models |
|----------|-----------------|-------------|--------|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` | `claude-sonnet-4-20250514` |
| Google Gemini | `gemini` | `GEMINI_API_KEY` | `gemini-2.5-flash` |
| NVIDIA | `nvidia` | `NVIDIA_API_KEY` | `meta/llama-3.3-70b-instruct`, `qwen/qwen2.5-coder-32b-instruct` |

API keys can be set in `config.yaml` or via environment variables — env vars take precedence.

---

## Usage

```bash
./bin/gitlab-ai
```

The REPL starts immediately. Type `start` to begin a session, `help` to see all commands, `exit` to quit.

```
> start
> mr-status cnips/backend
> mr-review cnips/backend 42
> What does the decorator pattern mean in this codebase?
```

---

## Command Reference

### Merge Requests

| Command | Description |
|---------|-------------|
| `mr-status <project>` | List open MRs |
| `mr-review <project> [mr]` | AI-powered review, saved to `reviews/` |
| `mr-comment <project> [mr]` | Post the last review as an MR comment |
| `mr-open <project> [branch] [target]` | Create a new MR |
| `mr-merge <project> <mr>` | Merge an MR |
| `mr-approve <project> <mr>` | Approve an MR |
| `mr-unapprove <project> <mr>` | Remove your approval |
| `mr-rebase <project> <mr>` | Rebase source branch onto target |
| `mr-update <project> <mr>` | Update MR title, description, labels |
| `mr-close <project> <mr>` | Close an MR |
| `mr-reopen <project> <mr>` | Reopen a closed MR |
| `mr-checks <project> <mr>` | Show pipeline status for an MR |

### Pipelines / CI

| Command | Description |
|---------|-------------|
| `pipeline <project>` | List recent pipelines |
| `pipeline-view <project> <id>` | Pipeline details and job list |
| `pipeline-logs <project> <job-id>` | Stream job log output |
| `pipeline-retry <project> <id>` | Retry a failed pipeline |
| `pipeline-cancel <project> <id>` | Cancel a running pipeline |

### Tickets / Issues

| Command | Description |
|---------|-------------|
| `ticket-open [project]` | Create a new ticket interactively |
| `ticket-open-empty` | Quick-create an empty ticket assigned to you |
| `ticket-close <project> <number>` | Close a ticket |
| `ticket-reopen <project> <number>` | Reopen a ticket |
| `ticket-update <project> <number>` | Update ticket metadata |
| `ticket-search <project>` | Search tickets by keyword |
| `tickets` | Generate a tickets report (saved to `tickets/`) |
| `tickets-black` | Tickets report excluding ignored projects |

### Repository

| Command | Description |
|---------|-------------|
| `list` / `projects` | List team projects |
| `diff <project>` | Compare tags or branches |
| `branch-cleanup <project>` | Remove stale merged branches |
| `release` | Check release status across projects |

### Content Generation (AI)

| Command | Description |
|---------|-------------|
| `create-ticket-content [project]` | Generate ticket content from current diff |
| `create-ticket-desc [project]` | Generate ticket description from linked MRs |
| `create-epic-content [project]` | Generate epic content from current diff |

### Workflow

| Command | Description |
|---------|-------------|
| `all-in-one` | Full flow: ticket → folder → commit → push → MR |

### Session

| Command | Description |
|---------|-------------|
| `start` | Begin a session (fetches projects in background) |
| `config` | Show current configuration |
| `help` | Print command reference |
| `exit` | End session |

---

## Review output

`mr-review` saves structured Markdown reviews to `./reviews/` using the pattern `{project}_{mr_number}.md`. Reviews contain sections configured in `config.yaml` under `review.template.sections` — by default: Summary, Code Quality, Security Concerns, Performance Impact, Suggestions, and Approval Recommendation.

Use `mr-comment` after `mr-review` to post the review directly as a GitLab MR comment.

---

## Development

```bash
make test       # run tests with race detector
make coverage   # HTML coverage report at coverage.html
make lint       # golangci-lint
make fmt        # gofmt
make vet        # go vet
make tidy       # go mod tidy
```

---

## Config reference

| Key | Default | Description |
|-----|---------|-------------|
| `gitlab.base_url` | — | GitLab instance URL |
| `gitlab.parent_folder` | — | Local folder containing cloned repos (for `all-in-one`) |
| `ai.provider` | `anthropic` | Active AI provider |
| `ai.timeout_seconds` | `180` | HTTP timeout for AI API calls |
| `cli.color_output` | `true` | Colorized output (disable with `NO_COLOR` env var) |
| `cli.verbose` | `false` | Debug logging |
| `cli.confirm_before_post` | `true` | Prompt before posting comments |
| `cli.idle_timeout_minutes` | `60` | Auto-exit after N minutes of inactivity |
| `cli.output_format` | `text` | `text` or `json` for machine-readable output |
| `teams` | `[]` | GitLab group prefixes to scope the session |
| `ignored_projects` | `[]` | Projects excluded from ticket reports |
