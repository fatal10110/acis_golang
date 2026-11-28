# 0011. Keep a datapack copy in the repository; run geodata and reference tests locally

- **Status:** Accepted
- **Date:** local-only tests decided on #4426
- **Sources:** [agents/verification.md](../agents/verification.md) (Datapack, geodata and reference
  tests), `ops/datapack-manifest.sh`, `ops/datapack.sha256`, `.github/workflows/go.yml`

## Context

Many tests load the datapack (XML and HTML game data) to check loaders and gameplay. Some tests
need two more inputs:

- **geodata**, whose region files are far too large to keep in Git;
- **the aCis Java sources**, which reference tests compare against (script fingerprints, config
  keys, spawn tables). They are not part of this repository.

CI must be reliable, and a test that silently skips proves nothing.

## Decision

- The repository carries a copy of the datapack in `aCis_datapack/`: `data/xml`, `data/html`,
  `serverNames.xml`, `sql`, `tools` and `build.xml`, byte for byte as the full datapack ships them,
  plus the server's own additions. It has no geodata. Tests find it first.
- CI sets `ACIS_REQUIRE_DATAPACK=1`, so a datapack test that cannot find its data fails instead of
  skipping.
- `ops/datapack.sha256` records a checksum of every file in the copy, and CI checks it. To change
  the copy, change the full datapack first, copy the files in, and refresh the manifest in the same
  commit.
- Geodata tests and reference tests are **local-only**. CI has neither input and does not run them
  (`ACIS_REQUIRE_REFERENCE=0`). A change to geodata code, or to anything the reference tests cover,
  must run them locally, and the pull request must report the result.

## Consequences

- A fresh clone can run the whole CI test set with only Docker.
- The datapack copy cannot drift by accident: an edit without a manifest refresh, or a checkout that
  converted line endings, fails CI.
- Servers must not run from the copy, because they write into `data/` (crests, announcements).
- Geodata and reference coverage depends on contributors running those tests locally. A private
  geodata store for CI can be revisited later.

## Alternatives considered

- **Fetch the datapack in CI.** It adds a network dependency and a second place where the version
  could drift.
- **Store geodata in the repository or with Git LFS.** The files are too large for the repository,
  and no CI geodata store exists yet.
- **Let datapack tests skip when data is missing.** A skipped test verifies nothing, and CI would
  stay green while losing coverage.
