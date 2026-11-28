# Architecture decision records

This folder records the important decisions behind the project: what was decided, why, and what it
costs. Each record is short and stands on its own. Read them when you want to know why the code is
shaped the way it is, or before you propose to change one of these choices.

The format follows the common
[architecture decision record](https://adr.github.io/) practice. Each record has:

- **Status**: *Proposed*, *Accepted*, *Superseded by NNNN*, or *Deprecated*.
- **Context**: the problem and the forces at play.
- **Decision**: what we do.
- **Consequences**: what gets easier, what gets harder, and what we accept.
- **Alternatives considered**: the options we did not take, and why.

Most of these decisions were first made in design plans, reviews and issues. The records summarize
them in one place and link to the original documents for the full detail. Where an original
document is long, it stays the authority; the record is the short version.

## Index

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-stay-compatible-with-acis.md) | Stay compatible with the aCis Java server | Accepted |
| [0002](0002-wire-servers-with-uber-fx.md) | Wire the servers with Uber Fx | Accepted |
| [0003](0003-per-actor-queues.md) | Run each actor's work on its own queue instead of locking every object | Accepted |
| [0004](0004-ordered-persistence-lanes.md) | Write to the database through ordered persistence lanes | Accepted |
| [0005](0005-domain-events-to-packets.md) | Domain code reports events; only the network layer builds packets | Accepted |
| [0006](0006-scripts-as-compiled-go.md) | Write game scripts as compiled Go, with no script interpreter | Accepted |
| [0007](0007-behavior-first-tests.md) | Test behavior end to end against a real database and a virtual clock | Accepted |
| [0008](0008-memory-mapped-geodata.md) | Keep geodata off the Go heap in a memory-mapped cache file | Accepted |
| [0009](0009-default-gc-settings.md) | Keep the Go runtime's default garbage collector settings | Accepted |
| [0010](0010-secure-defaults.md) | Bind privileged listeners to loopback by default | Accepted |
| [0011](0011-in-repo-datapack-copy.md) | Keep a datapack copy in the repository; run geodata and reference tests locally | Accepted |
| [0012](0012-deploy-on-release.md) | Deploy only when a release is published | Accepted |
| [0013](0013-code-describes-itself.md) | Production code describes this Go system only | Accepted |

## Smaller decisions: deliberate differences from the reference

[Decision 0001](0001-stay-compatible-with-acis.md) says the Go server behaves like the aCis Java
server. Where we chose on purpose to behave differently, usually to fix a bug or close an abuse path
in the reference, the choice is recorded as a **parity exception** in
[agents/porting-playbook.md](../agents/porting-playbook.md#3-check-the-client-visible-surface),
usually with its issue number and the test that pins it. There are 35 of them today. Some examples:

- Unicode handling follows the Go standard library instead of copying Java's quirks.
- A missing or malformed `.properties` file stops boot, where the reference logs it and carries on
  (#4772).
- Crafted private store and trade requests are refused (#4826).
- A wedding accept re-checks both spouses and takes payment first (#5010).

Before you open an issue about a difference from the reference, check that list. Add a new parity
exception there, in the same format, when you make one.

## Adding a record

1. Copy [template.md](template.md) to `NNNN-short-title.md`, using the next free number.
2. Fill it in. Keep it short: a reader should finish it in a few minutes. Link to the plan, issue
   or pull request that holds the detail.
3. Add it to the index above in the same pull request as the change it describes, or before it.
4. Never rewrite an accepted record to say something else. Write a new record and mark the old one
   *Superseded by NNNN*.
