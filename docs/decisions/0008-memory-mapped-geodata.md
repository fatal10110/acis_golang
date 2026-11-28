# 0008. Keep geodata off the Go heap in a memory-mapped cache file

- **Status:** Accepted and implemented (phase P1 of the memory redesign plan)
- **Date:** 2026-11
- **Sources:** [memory-redesign-plan.md](../memory-redesign-plan.md) (sections 1 and P1),
  `internal/gameserver/geo/geodata`, `cmd/geopack`

## Context

Geodata describes the terrain: heights, walls and layers for every cell of the map. The server
needs it for movement, line of sight and pathfinding.

A heap profile of the production game server with no players online showed a live heap of 1.38 GB.
Geodata was the largest part: about 693 MB, or 58 %. With the default garbage collector setting,
the heap may grow to twice the live size before a collection, so 650 MB of data that never changes
also cost about 1.3 GB of headroom. Decoding the region files at boot also took 20 to 30 seconds.

## Decision

Geodata is stored in a packed cache file and mapped into memory read-only, outside the Go heap.

- `cmd/geopack` builds the cache at deploy time from the region files listed in
  `geoengine.properties`. If the cache is missing or out of date, the game server builds it at boot.
- The cache is keyed on its format version, the geodata type, the region list, and each source
  file. A stale cache can never hide a missing or changed region file.
- The cache is written to a temporary file and renamed into place, never rewritten while mapped.
- Doors still add temporary blockers on top of the read-only base (`geo/dynamic`).
- `-geodata-cache ""` loads the region files onto the heap instead, for platforms or setups that
  cannot map the file.

## Consequences

- The Go heap shrinks by the whole geodata, and the garbage collector no longer scans it.
- The pages are file-backed. The kernel can drop them under memory pressure and keeps them warm
  across restarts, so boot is faster.
- Deployments gain a step (`geopack`) and a cache directory the server can write to (Docker mounts
  `./cache`).
- Tests compare the cache with the heap loader byte for byte, and `cmd/geoprobe` replays captured
  query answers to prove nothing changed.

## Alternatives considered

- **Keep geodata on the heap.** Simplest, but it was more than half the heap and doubled the memory
  the process needed.
- **Compress regions with zstd, LZ4 or S2.** Rejected: compressed blocks cannot be read cell by cell
  in constant time, and every pathfinding step would pay to decompress.
- **Deduplicate identical blocks.** Rejected: only 2 % of complex and 17 % of multilayer blocks
  repeat, which is not worth the hashing and the extra indirection.
