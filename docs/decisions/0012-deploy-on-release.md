# 0012. Deploy only when a release is published

- **Status:** Accepted
- **Date:** after the 2026-09-05 infrastructure review, which describes the earlier behavior
- **Sources:** `.github/workflows/deploy.yml`,
  [infra-review-2026-09-05.md](../infra-review-2026-09-05.md) (finding 7 notes the earlier
  behavior)

## Context

Production used to deploy automatically on every green push to `main`. The project often merges
dozens of pull requests a day, so the live server changed many times a day, and every merge was
also a release.

## Decision

The deploy workflow runs only when someone publishes a GitHub release. The release's tag selects the
commit. Before deploying, the workflow checks that the required `go` check passed on that commit; if
not, it refuses to deploy. It attaches the built binaries and the datapack to the release, and only
one deploy runs at a time.

## Consequences

- Deploying is a deliberate step, and each deploy has a release with its binaries and datapack
  attached, which also gives a rollback point.
- A merge to `main` no longer reaches players by itself.
- Someone must publish releases for fixes to reach production.

## Alternatives considered

- **Deploy every green `main` push.** Fastest path to production, but every merge goes live with no
  deliberate step.
- **Deploy from a separate release branch.** It gives the same control as tags, with more branch
  upkeep.
