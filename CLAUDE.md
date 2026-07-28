@AGENTS.md

# Claude Code project notes

Do not duplicate repository-wide rules here.

## Shared context

- The deterministic cross-client cache is `../.agent-cache/`; start at
  `../.agent-cache/INDEX.md`.
- Do not require OpenMemory or Claude automatic memory for project work. If either is available, it
  is optional and must not replace verified repository files.

## Bounded agents

Workspace-level custom agents live under `../.claude/agents/`:

- `java-lookup`: one exact, read-only lookup;
- `java-researcher`: one bounded multi-file behavior investigation;
- `port-reviewer`: selective read-only review for high-risk changes.

For a bounded, independent read-only lookup, run the free OpenCode scout via
`scripts/agent-tools/opencode_eval.py` before a paid research agent. Verify its evidence locally; use its builder
or reviewer only when the user explicitly requests them.

Use at most one research agent for an ordinary issue and at most two concurrent agent threads. Do
not use agent teams or preload unrelated skills. Prefer CLI tools over an equivalent MCP server when
the CLI returns the same bounded result.

For Claude Code, classifying the task through `docs/agents/process-skills.md` satisfies any generic
initial skill-selection rule. Load only the selected process skills; Java research agents load none.

Agent model assignments are budget hypotheses. Keep normal work at medium effort, escalate only for
hard debugging or high-risk review, and record comparable outcomes before changing defaults. See
[`docs/agents/model-policy.md`](docs/agents/model-policy.md).
