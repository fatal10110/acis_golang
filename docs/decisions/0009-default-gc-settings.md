# 0009. Keep the Go runtime's default garbage collector settings

- **Status:** Accepted
- **Date:** 2026-10-05
- **Sources:** [profiling.md](../profiling.md) (Measured Run and GC Decision)

## Context

Game servers are sensitive to pauses, and tuning the garbage collector (`GOGC`, `GOMEMLIMIT`) is a
common first reflex. Before deciding, we measured. A performance test drove 200 clients walking and
attacking for 30 seconds, once with each `GOGC` value:

| `GOGC` | GC CPU share | Longest pause | Heap reserved | Step latency p99 / max |
| --- | --- | --- | --- | --- |
| 100 (default) | 1.4 % | 3.6 ms | 82 MiB | 47 / 62 ms |
| 200 | 0.6 % | 0.2 ms | 129 MiB | 41 / 63 ms |
| 400 | 0.4 % | 0.2 ms | 197 MiB | 48 / 62 ms |

## Decision

The servers use the Go runtime's defaults. The code does not call `debug.SetGCPercent` or
`debug.SetMemoryLimit`, and the shipped configuration sets no `GOGC`.

## Consequences

- The garbage collector costs about 1.4 % of CPU and pauses stay under 4 ms at the default setting.
- Raising `GOGC` would save about 1 % of CPU, but would double or quadruple the reserved heap.
- The latency tail (p99 and max) is the same at every setting, so it is not caused by the garbage
  collector and needs a different fix.
- Operators can still set the standard variables without a code change. `GOMEMLIMIT` is useful in a
  container with a memory cap; set it below the cap.
- Re-run the measurement at the target player count before revisiting this decision.

## Alternatives considered

- **Ship a higher `GOGC`.** A small CPU gain for a large memory cost, with no latency gain.
- **Set a memory limit in code.** The right limit depends on the host, so it belongs to the
  operator, as an environment variable.
