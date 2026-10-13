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

Claude roles currently use Haiku for the narrow lookup, Sonnet at high effort for multi-file research,
and Fable at high effort for selective high-risk review; see the 2026-10-13 benchmark below. The
agent files use floating aliases, so each role follows the newest model of its family. Codex uses GPT-5.6 Luna at medium effort for narrow Java lookup and keeps
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

## Claude subagent benchmark (2026-10-13)

Each run pair used an identical prompt with a per-call model or effort override, scored against
answers verified in the Java source or against saved reviews of merged PRs. Agents were told not to
read the cached answer. Samples are small (one run per arm); rerun before reversing a decision.

**`java-lookup`, Haiku vs Sonnet** (10 exact-fact questions in 7 rounds: opcodes, constants, a caller
list, a regex comment-vs-code trap, a nonexistent feature, dead code, rejection replies, a heal
formula). Both answered every core question correctly. Sonnet added five confidently wrong details:
three wrong worked examples in the `replaceAll(".xml", "")` trap, one wrong SystemMessage id, and a
missed `Player.canBeHealed` override. Haiku had none. Haiku used 150k tokens, 63 tool calls, and 187 s;
Sonnet used 118k, 39, and 171 s. Haiku's per-token price is 1/20 of Sonnet's, so it costs about 1/16
as much. Decision: Haiku.

**`port-reviewer`, Fable vs Opus at high effort** (PR #2344 and #2363 reviewed at their merge commits,
scored against the four verified findings of each saved review).

| PR | Opus | Fable |
| --- | --- | --- |
| #2344 (33 files) | 2/4, including the Medium item-loss/dupe (non-deferred round accounting); plus one valid new race, since fixed on `main`. 137k tokens, 9 min. | 2/4; missed the dupe and called that path safe. 118k tokens, 10 min. |
| #2363 (178 files) | 0/4; hit the 40-turn limit before reaching the wander and cast-gate code. 188k tokens, 5 min. | 3/4, including the High wander regression on a newly live path. 337k tokens, 12 min. |

Fable found 5/8 known defects and Opus 2/8. Fable cost about 3.5× as much per review (2.5× the
per-token price, more tokens). The reviewer runs only on explicit request for high-risk changes,
so recall matters more than cost there. Decision: Fable. Both still miss defects, so a clean report
is not proof.

**`java-researcher`, Sonnet at medium vs high effort** (bow fire-time sequence and NPC `teleportTo`
lifecycle, 10 checkpoints each from verified reports).

| Topic | Medium | High |
| --- | --- | --- |
| Bow fire time | 9/10; NPC soulshot recharge muddled. 52k tokens, 1.1 min. | 9.5/10, more correct extras. 75k tokens, 2.5 min. |
| NPC teleport | 10/10; one wrong detail (said `TargetUnselected` skips self). 54k tokens, 1.8 min. | 10/10, no wrong details. 93k tokens, 2.8 min. |

Both missed that `Weapon.getMpConsume()` rolls `mp_consume_reduce` on every call. High cost about
1.5× the tokens and twice the time, and had no wrong details. Wrong research details become parity
bugs, so the decision is to keep high.

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
