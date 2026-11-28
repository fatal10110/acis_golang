# Documentation

Start with the guide for what you want to do. The plans and audits at the end record design
decisions and past reviews; they describe the code at the time they were written.

## Run a server

| Page | Covers |
| --- | --- |
| [run-servers.md](run-servers.md) | build and run the login and game servers directly: schema, config, registration, debug endpoints, IP ban API, bot swarm |
| [ops/docker.md](ops/docker.md) | run MariaDB and both servers with Docker Compose |
| [ops/backup-restore.md](ops/backup-restore.md) | back up and restore the database, config and crests |
| [ops/migrating-from-java.md](ops/migrating-from-java.md) | move an aCis Java installation to the Go servers, and the known differences |
| [observability.md](observability.md) | log files, log format, metrics, and what each failure looks like |
| [profiling.md](profiling.md) | CPU, heap and goroutine profiles, and the GC settings |
| [tools.md](tools.md) | every binary under `cmd/` and its flags |

## Work on the code

| Page | Covers |
| --- | --- |
| [architecture.md](architecture.md) | processes, protocols, boot and shutdown, the concurrency model, package map |
| [development.md](development.md) | setup, build, tests, lint, conventions, CI |
| [network-ownership-map.md](network-ownership-map.md) | what each file family in the game network package owns |
| [`internal/gameserver/network/AGENTS.md`](../internal/gameserver/network/AGENTS.md) | rules for changes to the game network package |

## Contributor and agent guides

These pages are written for coding agents but apply to every contributor. `AGENTS.md` at the
repository root points to the workspace rules they build on.

| Page | Covers |
| --- | --- |
| [agents/go-style.md](agents/go-style.md) | Go style and design rules, including lock order |
| [agents/test-strategy.md](agents/test-strategy.md) | test tiers, the harness, when a fake is allowed |
| [agents/verification.md](agents/verification.md) | which checks to run for a change, and the local-only tests |
| [agents/porting-playbook.md](agents/porting-playbook.md) | how to port behavior from the aCis Java reference and prove it matches |
| [agents/action-response-contract.md](agents/action-response-contract.md) | when a client action must be answered or released |
| [agents/server-initiated-updates.md](agents/server-initiated-updates.md) | rules for updates the server sends on its own |
| [agents/process-skills.md](agents/process-skills.md) | how much process a task class needs |
| [agents/tooling.md](agents/tooling.md) | search and navigation tools for agents |
| [agents/model-policy.md](agents/model-policy.md) | which model to use for which agent task |
| [parity-sweep/README.md](parity-sweep/README.md) | procedure for the behavioral parity sweep |

## Design plans and audits

| Page | Status |
| --- | --- |
| [script-engine-plan.md](script-engine-plan.md) | implemented; its rules are in force for `internal/gameserver/script` |
| [concurrency-refactor-plan.md](concurrency-refactor-plan.md) | per-actor queues instead of per-object locks; the status line in the page lists the phases that landed |
| [actor-model-degoifying-plan.md](actor-model-degoifying-plan.md) | typed registries and a domain event sink in place of hook setters; the event sink (`model/actor/event`) is in the code |
| [memory-redesign-plan.md](memory-redesign-plan.md) | proposal (2026-11-02) |
| [dormant-npc-ark-plan.md](dormant-npc-ark-plan.md) | shelved until measured (2026-11-02) |
| [superpowers/specs/2026-07-11-geometry-territory-design.md](superpowers/specs/2026-07-11-geometry-territory-design.md) | design of the geometry package and the territory shape |
| [infra-review-2026-09-05.md](infra-review-2026-09-05.md) | infrastructure review at that date |
| [packet-gap-audit-2026-07-14.md](packet-gap-audit-2026-07-14.md) | packet coverage audit at that date |

Package-level documentation lives in each package's `doc.go`. Read it with `go doc`, for example
`go doc ./internal/gameserver/sim`.
