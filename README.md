# git-agent

An interactive terminal assistant for GitLab. It manages merge requests,
pipelines and tickets, reviews code with AI, and keeps a local record of what you
did.

```text
$ ./bin/gitlab-ai

  git-agent — Interactive Mode
  Type 'help' for commands, or ask anything in plain English.
  Tab completes · ↑/↓ history · Ctrl+R search · auto-exit after 1h0m0s idle

  ✓ Authenticated as Jane Doe (@jane)
  ✓ Environment: work
  ✓ Team: cnips
  ✓ Session started — ready for commands

git-agent[work/cnips]> mr-review backend 42
git-agent[work/cnips]> show me failed pipelines in frontend
git-agent[work/cnips]> stats week
```

## Contents

- [Features](#features)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Configuration](#configuration)
- [Usage](#usage)
- [Command reference](#command-reference)
- [Stats and dashboard](#stats-and-dashboard)
- [Files and locations](#files-and-locations)
- [Security](#security)
- [Development](#development)
- [Releasing](#releasing)

## Features

**GitLab workflow**
- The full merge request lifecycle: list, open, review, comment, approve, rebase,
  merge, close, reopen and check pipelines.
- Pipelines: list, inspect jobs, read logs, retry and cancel.
- Tickets: create, update, search, close and reopen, plus open-ticket reports.
- Repository tools: compare tags or branches, clean up merged branches, and
  check release status across projects.
- `ship`: ticket → folders → commit → push → MR → update ticket, in one command.

**AI assistance** (Anthropic Claude, Google Gemini or NVIDIA NIM)
- Structured MR reviews using a configurable template, saved as Markdown and
  optionally posted to the MR.
- AI triage of failed pipelines.
- Ticket, epic and MR descriptions drafted from diffs.
- Plain-English input is routed to the right command ("show open MRs in
  backend" runs `mr-status backend`). Anything else is answered as a question.
- An optional fallback model per provider, and a configurable API `base_url` for
  proxies and gateways.

**Day-to-day ergonomics**
- Tab completion for commands, project names and environments. Short project
  names resolve to full paths (`backend` → `team/backend`).
- "Did you mean …?" for mistyped commands, before anything is sent to the AI.
- Confirmation before merging, closing or cancelling.
- One-shot mode for scripts: `gitlab-ai mr-status backend` (exit status 1 on
  error).
- Environment profiles (for example work and personal GitLab) that you can switch
  without restarting.
- `doctor` shows where every setting comes from and checks access.

**Activity tracking**
- Every command is recorded locally with its outcome and duration. AI calls also
  record the model, latency and token usage.
- `stats` shows your success rate, trend, top commands and projects, model usage
  and how often AI output was accepted. It can also export to CSV or JSON.
- `stats dashboard` opens a self-contained web dashboard that works offline.

## Requirements

- Go 1.25 or newer, to build from source
- A GitLab instance (self-hosted or gitlab.com) and a personal access token with
  `api` scope
- An API key for at least one AI provider:
  [Anthropic](https://console.anthropic.com/settings/keys),
  [Google Gemini](https://aistudio.google.com/apikey) or
  [NVIDIA](https://build.nvidia.com/)

## Installation

```bash
make build            # → bin/gitlab-ai
make install          # → $GOPATH/bin
make run              # build and start
```

Cross-compilation:

```bash
make build-linux      # linux/amd64
make build-darwin     # darwin/amd64 and darwin/arm64
make build-windows    # windows/amd64
make build-all        # all of the above
```

## Quick start

```bash
mkdir -p ~/.config/git-agent
cp configs/default.yaml ~/.config/git-agent/config.yaml
chmod 600 ~/.config/git-agent/config.yaml
$EDITOR ~/.config/git-agent/config.yaml    # set gitlab.base_url and teams

export GITLAB_TOKEN=glpat-...
export ANTHROPIC_API_KEY=sk-ant-...

./bin/gitlab-ai doctor                     # verify the setup
./bin/gitlab-ai                            # start the REPL
```

## Configuration

### Where config is read from

git-agent uses the first config file it finds:

1. `--config <path>`
2. `./config.yaml`
3. `./configs/config.yaml`
4. `~/.config/git-agent/config.yaml` (honours `$XDG_CONFIG_HOME`)

Keep your config in `~/.config/git-agent/` so git-agent behaves the same from any
directory. Relative output directories in a config file are resolved against that
file's location. `config.yaml` is gitignored; never commit it.

### Minimal config

```yaml
gitlab:
  base_url: "https://gitlab.example.com/"

ai:
  provider: anthropic          # anthropic | gemini | nvidia

teams:
  - my-team                    # GitLab group prefix that scopes the session
```

### GitLab token

The token is taken from the first of these that is set:

1. The env var named by `gitlab.token_env` (default `GITLAB_TOKEN`)
2. `GITLAB_TOKEN`
3. `gitlab.token` in the config file
4. A `~/.netrc` entry for the GitLab host

### AI providers

| Provider      | `ai.provider` | Key env var         | Default model                                   |
|---------------|---------------|---------------------|-------------------------------------------------|
| Anthropic     | `anthropic`   | `ANTHROPIC_API_KEY` | `claude-sonnet-4-20250514`                      |
| Google Gemini | `gemini`      | `GEMINI_API_KEY`    | `gemini-2.5-flash`                              |
| NVIDIA NIM    | `nvidia`      | `NVIDIA_API_KEY`    | `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning` |

Each provider section (`ai.anthropic`, `ai.gemini`, `ai.nvidia`) accepts:

| Key                         | Description                                                           |
|-----------------------------|-----------------------------------------------------------------------|
| `api_key`                   | The key. Leave empty to use the env var instead.                      |
| `api_key_env`               | Which env var to read the key from.                                   |
| `model`, `fallback_model`   | The primary model, and the model to retry on if the primary fails.    |
| `max_tokens`, `temperature` | Generation limits.                                                    |
| `base_url`                  | API host override for a proxy or gateway. See [Security](#security). |

NVIDIA also accepts `top_p`, `enable_thinking` and `reasoning_budget`.

### Environment profiles

Keep several setups in one file. Each entry under `environments:` is an overlay:
the keys it sets replace the base config, and everything else is inherited.

```yaml
gitlab:
  base_url: "https://gitlab.work.example/"
teams: [cnips]

default_environment: work
environments:
  work: {}
  personal:
    gitlab:
      base_url: "https://gitlab.com/"
      token_env: PERSONAL_GITLAB_TOKEN
    ai:
      provider: gemini
    teams: [me]
```

Select a profile with `--env personal`, `GITLAB_AI_DEFAULT_ENVIRONMENT`, or
`switch-env` inside the REPL. The prompt shows the active environment and team,
for example `git-agent[personal/me]>`.

### Teams

`teams` scopes the project list to GitLab group prefixes. A team can carry its own
settings:

```yaml
teams:
  - name: cnips
    parent_folder: /home/me/src/cnips   # local clones, used by ship
    projects: [cnips/issues]            # where tickets live
    ignored_projects: [cnips/sandbox]   # excluded from ticket reports
```

Plain strings (`teams: [cnips]`) still work. Use `switch-team` to change the
active team.

### Environment variables

Any setting can be overridden with a `GITLAB_AI_` env var. Replace dots with
underscores: `GITLAB_AI_GITLAB_BASE_URL`, `GITLAB_AI_AI_PROVIDER`,
`GITLAB_AI_CLI_IDLE_TIMEOUT_MINUTES`. Set `NO_COLOR` to disable colour.

### Config reference

| Key                                     | Default                 | Description                                                    |
|-----------------------------------------|-------------------------|----------------------------------------------------------------|
| `platform`                              | `gitlab`                | Code-hosting platform (only GitLab is implemented)             |
| `gitlab.base_url`                       | —                       | GitLab instance URL (required)                                 |
| `gitlab.token_env`                      | `GITLAB_TOKEN`          | Env var holding the token                                      |
| `gitlab.parent_folder`                  | —                       | Folder of local clones, used by `ship`                         |
| `gitlab.default_project`                | —                       | Default project for ticket commands                            |
| `ai.provider`                           | `anthropic`             | Active AI provider                                             |
| `ai.timeout_seconds`                    | `180`                   | Timeout for AI API calls                                       |
| `review.template.sections`              | six sections            | Review sections (name, prompt, required)                       |
| `review.output.directory`               | `./.data`               | Where reviews are saved                                        |
| `review.filters.exclude_files`          | `*.lock`, `vendor/*`, … | Files left out of AI reviews                                   |
| `issues.output.directory`               | `./.data`               | Where ticket reports are saved                                 |
| `other.directory`                       | `./.data`               | Where other generated files and exports go                     |
| `merge.squash`                          | `false`                 | Default squash setting for `mr-merge`                          |
| `merge.remove_source_branch`            | `true`                  | Default remove-branch setting for `mr-merge`                   |
| `ticket_content.template`               | built-in                | Markdown template for AI ticket content                        |
| `epic_content.template`                 | built-in                | Markdown template for AI epic content                          |
| `cli.color_output`                      | `true`                  | Coloured output                                                |
| `cli.verbose`                           | `false`                 | Debug logging                                                  |
| `cli.confirm_before_post`               | `true`                  | Ask before posting comments                                    |
| `cli.auto_confirm`                      | `false`                 | Answer yes to confirmations, including merge, close and cancel |
| `cli.idle_timeout_minutes`              | `60`                    | Exit the REPL after this many idle minutes                     |
| `cli.open_in_browser`                   | `true`                  | Open created MRs, tickets and the dashboard in the browser     |
| `cli.theme`                             | `default`               | Colour theme                                                   |
| `teams`, `projects`, `ignored_projects` | `[]`                    | Session scope (see [Teams](#teams))                            |
| `environments`, `default_environment`   | —                       | Profiles (see [Environment profiles](#environment-profiles))   |

## Usage

### Interactive

```bash
./bin/gitlab-ai [--config path] [--env name]
```

The session starts automatically. Commands that take a project accept a full path
or a short name, and offer a picker when you leave it out. Any input that isn't a
command is classified by the AI: it either runs the matching command or answers
your question.

### One-shot

```bash
./bin/gitlab-ai mr-status cnips/backend
./bin/gitlab-ai --env personal stats week
./bin/gitlab-ai stats export csv month || echo "export failed"
```

This runs one command and exits, with status 1 if the command reported an error.
It prints no banner and no session summary, and records to the same audit trail.

## Command reference

### Merge requests

| Command                               | Description                                     |
|---------------------------------------|-------------------------------------------------|
| `mr-status <project>`                 | List open MRs                                   |
| `mr-review <project> [mr]`            | AI review, saved as Markdown, optionally posted |
| `mr-comment <project> [mr]`           | Post the last review as an MR comment           |
| `mr-open <project> [branch] [target]` | Create an MR with an AI-drafted description     |
| `mr-merge <project> <mr>`             | Merge an MR (asks first)                        |
| `mr-approve <project> <mr>`           | Approve an MR                                   |
| `mr-unapprove <project> <mr>`         | Remove your approval                            |
| `mr-rebase <project> <mr>`            | Rebase the source branch onto the target        |
| `mr-update <project> <mr>`            | Update the title, description or labels         |
| `mr-close <project> <mr>`             | Close an MR (asks first)                        |
| `mr-reopen <project> <mr>`            | Reopen a closed MR                              |
| `mr-checks <project> <mr>`            | Pipeline status for an MR                       |

### Pipelines / CI

| Command                            | Description                            |
|------------------------------------|----------------------------------------|
| `pipeline <project>`               | List recent pipelines                  |
| `pipeline-view <project> <id>`     | Pipeline details and jobs              |
| `pipeline-logs <project> <job-id>` | Job log output                         |
| `pipeline-retry <project> <id>`    | Retry a failed pipeline                |
| `pipeline-cancel <project> <id>`   | Cancel a running pipeline (asks first) |
| `pipeline-triage <project> [id]`   | AI triage of a failed pipeline         |

### Tickets / issues

| Command                            | Description                                  |
|------------------------------------|----------------------------------------------|
| `ticket-open [project]`            | Create a ticket interactively                |
| `ticket-open-empty`                | Quick-create an empty ticket assigned to you |
| `ticket-close <project> <number>`  | Close a ticket (asks first)                  |
| `ticket-reopen <project> <number>` | Reopen a ticket                              |
| `ticket-update <project> <number>` | Update ticket metadata                       |
| `ticket-search <project>`          | Search tickets                               |
| `tickets`                          | Generate an open-tickets report              |
| `tickets-black`                    | Generate a malformed-tickets report          |

### Repository

| Command                    | Description                         |
|----------------------------|-------------------------------------|
| `list` / `projects`        | List team projects                  |
| `diff <project>`           | Compare tags or branches            |
| `branch-cleanup <project>` | Delete merged branches (asks first) |
| `release`                  | Release status across projects      |

### Content generation (AI)

| Command                           | Description                        |
|-----------------------------------|------------------------------------|
| `create-ticket-content [project]` | Ticket content from a branch diff  |
| `create-ticket-desc [project]`    | Ticket description from linked MRs |
| `create-epic-content [project]`   | Epic content from a branch diff    |

### Workflow

| Command | Description                                           |
|---------|-------------------------------------------------------|
| `ship`  | Ticket → folders → commit → push → MR → update ticket |

### Stats

| Command                                             | Description                                |
|-----------------------------------------------------|--------------------------------------------|
| `stats [today\|week\|month [YYYY-MM]\|year [YYYY]]` | Activity summary (default: last 30 days)   |
| `stats export csv\|json [range]`                    | Export raw events to `other.directory`     |
| `stats dashboard`                                   | Open the local web dashboard               |

### Session

| Command             | Description                                                   |
|---------------------|---------------------------------------------------------------|
| `start`             | Start a session (runs automatically)                          |
| `switch-team`       | Switch the active team                                        |
| `switch-env [name]` | Switch environment profile and reconnect                      |
| `doctor`            | Show where settings come from; check GitLab access and AI key |
| `config`            | Show the current configuration                                |
| `help`              | Show the command reference                                    |
| `exit`              | End the session                                               |

Keys: <kbd>Tab</kbd> completes, <kbd>↑</kbd>/<kbd>↓</kbd> browse history,
<kbd>Ctrl</kbd>+<kbd>R</kbd> searches history. In menus, number keys pick an
option and <kbd>Esc</kbd> cancels.

## Stats and dashboard

Every command is recorded in a local SQLite audit trail. Nothing leaves your
machine.

`stats` reports:
- commands run, failures and success rate
- MRs reviewed, created and posted; tickets created; files written
- a daily activity trend line
- top commands (with failure counts) and top projects
- per-model AI usage, latency (average, min, max) and input/output tokens
- AI output quality: accepted, edited or discarded
- your busiest days

`stats dashboard` serves the same data at a local address. It only accepts
loopback requests, stops when you press Enter, and needs no internet access.

## Files and locations

| What                       | Where                                                       |
|----------------------------|-------------------------------------------------------------|
| Config                     | See [Where config is read from](#where-config-is-read-from) |
| Command history            | `~/.config/git-agent/history`                               |
| Audit trail (`stats`)      | `~/.config/git-agent/audit/audit.db`                        |
| Reviews, reports, exports  | `./.data/` (configurable, relative to the config file)      |
| Review context per project | `./.context/`                                               |

An audit trail in the old `./.git-agent/audit/` location is moved to the config
directory on first run. It is moved only if it is a regular file that you own.

## Security

- **Secrets in config.** Startup and `doctor` warn when a config file holds a
  token or API key and others can read it. Run `chmod 600` on the file, or keep
  secrets in env vars.
- **AI `base_url`.** This setting decides where your API key is sent, so it is
  only honoured from `~/.config/git-agent/config.yaml`, a file passed with
  `--config`, or `GITLAB_AI_AI_<PROVIDER>_BASE_URL`. A `config.yaml` found in the
  current directory (for example in a cloned repository) cannot redirect it.
  `base_url` must use https; plain http is allowed only for localhost.
- **Dashboard.** It binds to 127.0.0.1 and rejects requests with a non-loopback
  `Host` header. It loads no external resources.
- **Gemini.** The key is sent in a request header, never in the URL.

## Development

```bash
make test       # all tests with the race detector and coverage
make coverage   # HTML coverage report → coverage.html
make lint       # golangci-lint
make fmt        # gofmt
make vet        # go vet
make tidy       # go mod tidy
```

The end-to-end suite in `tests/e2e/` builds the real binary and drives it against
fake GitLab and Anthropic servers. It needs no network access and no real
credentials. To also catch data races in the binary, build it with the race
detector:

```bash
GOFLAGS=-race go test -race ./...
```

Commands are defined once, in `cmd/commands.go`. That table drives dispatch,
`help`, tab completion and the AI prompts. A test fails if this README stops
documenting a command.

## Releasing

Releases are automated with semantic-release from conventional commits on
`master`. See [RELEASING.md](RELEASING.md).
