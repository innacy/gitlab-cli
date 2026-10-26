package config

import "github.com/spf13/viper"

func setDefaultsOn(v *viper.Viper) {
	v.SetDefault("platform", "gitlab")

	v.SetDefault("gitlab.base_url", "https://gitlab.example.com/")
	v.SetDefault("gitlab.api_version", "v4")
	v.SetDefault("gitlab.token_env", "GITLAB_TOKEN")
	v.SetDefault("gitlab.parent_folder", "")

	v.SetDefault("ai.provider", "anthropic")
	v.SetDefault("ai.timeout_seconds", 180)
	v.SetDefault("ai.anthropic.api_key_env", "ANTHROPIC_API_KEY")
	v.SetDefault("ai.anthropic.model", "claude-sonnet-4-20250514")
	v.SetDefault("ai.anthropic.max_tokens", 8192)
	v.SetDefault("ai.anthropic.temperature", 0.7)
	v.SetDefault("ai.gemini.api_key_env", "GEMINI_API_KEY")
	v.SetDefault("ai.gemini.model", "gemini-2.5-flash")
	v.SetDefault("ai.gemini.max_tokens", 8192)
	v.SetDefault("ai.nvidia.api_key_env", "NVIDIA_API_KEY")
	v.SetDefault("ai.nvidia.model", "nvidia/nemotron-3-nano-omni-30b-a3b-reasoning")
	v.SetDefault("ai.nvidia.max_tokens", 65536)
	v.SetDefault("ai.nvidia.temperature", 0.6)
	v.SetDefault("ai.nvidia.top_p", 0.95)
	v.SetDefault("ai.nvidia.enable_thinking", false)
	v.SetDefault("ai.nvidia.reasoning_budget", 16384)

	v.SetDefault("review.output.directory", "./.data")
	v.SetDefault("review.output.filename_pattern", "{project}_mr_review_{mr_number}.md")
	v.SetDefault("review.output.include_metadata", true)
	v.SetDefault("review.output.include_diff", false)
	v.SetDefault("review.filters.exclude_files", []string{"*.lock", "vendor/*", "*.generated.go"})

	v.SetDefault("issues.output.directory", "./.data")
	v.SetDefault("issues.output.filename_pattern", "{project}_issues_{date}.md")

	v.SetDefault("other.directory", "./.data")

	v.SetDefault("merge.squash", false)
	v.SetDefault("merge.remove_source_branch", true)

	v.SetDefault("cli.color_output", true)
	v.SetDefault("cli.markdown_rendering", true)
	v.SetDefault("cli.verbose", false)
	v.SetDefault("cli.confirm_before_post", true)
	v.SetDefault("cli.non_interactive", false)
	v.SetDefault("cli.auto_confirm", false)
	v.SetDefault("cli.idle_timeout_minutes", 60)
	v.SetDefault("cli.output_format", "text")
	v.SetDefault("cli.theme", "default")
	v.SetDefault("cli.open_in_browser", true)

	v.SetDefault("ticket_content.template", `# Ticket: <TITLE>

## Summary

<Create a concise summary of the work, objective, and expected outcome.>

---

# Problem Statement

Describe:
- the current problem
- existing gaps
- operational pain points
- technical limitations
- missing capabilities
- why this work is needed

---

# Deliverables

- item 1
- item 2
- item 3

---

# Acceptance Criteria

| Criteria | Definition of Done |
|---|---|
| Example 1 | Clear measurable expectation |
| Example 2 | Validation expectation |
| Example 3 | Operational expectation |

---`)

	v.SetDefault("epic_content.template", `## Background
- <Why this epic exists>
- <Current limitations or business/technical motivation>
- <Any prior decisions or incidents that led to this work>

## Goals / Objectives
- <Primary goal>
- <Secondary goal>
- <Success definition (measurable if possible)>

## Scope
### In Scope
- <Major feature / system change 1>
- <Major feature / system change 2>

### Out of Scope
- <Explicit exclusions to prevent scope creep>

### Sub-tasks / Deliverables
- [ ] <Task 1 – short description>
- [ ] <Task 2 – short description>
- [ ] <Task 3 – short description>

## Architecture / Technical Approach
- <High-level design>
- <Key components involved (services, pipelines, DB, etc.)>
- <Data flow changes>
- <Trade-offs and design decisions>

## Dependencies
- <Upstream/downstream services>
- <External systems>
- <Team dependencies>

## Risks & Mitigations
- <Risk 1> → <Mitigation>
- <Risk 2> → <Mitigation>

## Acceptance Criteria
- [ ] All sub-tasks completed
- [ ] End-to-end flow works as expected
- [ ] No regression in existing systems
- [ ] Performance and scalability targets met

## Impact & Timeline
- Teams / services affected:
- Breaking changes (if any):
- Phase 1:
- Phase 2:
- Phase 3:`)
}
