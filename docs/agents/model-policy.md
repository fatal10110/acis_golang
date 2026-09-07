# Agent model policy

Optimize for correctly completed issues per subscription usage window. Model roles below are
hypotheses to test, not facts about cost or quality.

## Current policy

- Preserve the working platform default unless comparable local evidence supports a change.
- Use medium reasoning for normal implementation and bounded research.
- Use a small/fast model for narrow, objectively verifiable lookups only when the platform offers one
  and benchmark results show the delegation saves total usage.
- Escalate to a stronger model or higher reasoning for difficult debugging, high-risk review, or
  major design—not for routine work.
- Limit ordinary work to one main agent plus at most one research agent. Maximum two concurrent agent
  threads is an operational rule unless a locally supported configuration key is verified.
- Do not treat generated-token count as a proxy for subscription efficiency.

Claude roles currently use Haiku for the narrow lookup, Sonnet for multi-file research, and Opus for
selective high-risk review. Codex uses GPT-5.6 Luna at medium effort for narrow Java lookup and keeps
GPT-5.5 at medium/high effort for multi-file research and high-risk review. Cursor uses Grok 4.5 at
medium effort for the narrow lookup and keeps Grok 4.6 at medium/high effort for multi-file research
and high-risk review. These assignments are testable hypotheses, not efficiency claims.

## Availability and fallback

Model availability is specific to the installed platform version, account, workspace, and runtime.
Before configuring a model, verify that the local runtime recognizes it, can run it as the primary
model, and can spawn it as a subagent. An explicit subagent override is valid only after that test.

If an override is unavailable, remove it and let the subagent inherit the verified main model.
Report the rejected model and the inherited fallback; never fall back silently or select a model
solely because it appears in the API catalog.

GPT-5.6 Luna replaces GPT-5.4 Mini in this Codex runtime. Its comparative subscription efficiency
for this repository has not yet been measured.

Cursor Grok 4.6's named-model default is high. Ordinary Cursor work still uses medium unless
escalation is justified. Fast variants and Grok 4.6 xhigh are for difficult debugging, high-risk
review, or major design—not routine work. This Cursor runtime currently offers Composer 2.5 Fast,
Grok 4.5 High Fast, and Grok 4.6 High Fast as explicit subagent overrides; any other effort or speed
inherits the verified main model until locally confirmed. Composer 2.5 Fast is the small/fast lookup
candidate when a spawned subagent can use it and a benchmark shows a saving. Comparative Cursor
subscription efficiency for this repository has not yet been measured.

## Benchmark template

Run GPT-5.5, Terra, Luna, Grok 4.6, Grok 4.5, Composer 2.5, or current equivalents on comparable
issue categories before changing the default. Record one row per completed issue:

| Metric | Value |
| --- | --- |
| Model | |
| Reasoning effort | |
| Task category and size | |
| Successful completion | yes / no |
| Human interventions | |
| Research agent calls | |
| Tool calls | |
| Compactions | |
| Retries | |
| Tests passing after first implementation | |
| Reviewer-discovered defects | |
| Approximate subscription usage before/after, when visible | |
| Elapsed time | |

Primary metric: correctly completed issues per subscription usage window.

Secondary metrics: human interventions per issue, retries per issue, and reviewer-discovered defects
per issue.

Codex, Claude, and Cursor do not expose a reliable, comparable per-task subscription-unit counter in
the locally inspected configuration surfaces. Record visible limits or usage only as approximate data,
and do not invent precision.
