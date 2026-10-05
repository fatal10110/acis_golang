# Profiling And GC Tuning

How to take CPU, heap, and goroutine profiles of the Go servers, how to check for goroutine leaks,
and the measured profiling run behind the GC decision: the servers run with the Go runtime's
default GC settings.

## Profiling A Running Server

Start the gameserver or loginserver with `-debug-addr 127.0.0.1:6060` (see
[Debug Endpoints](run-servers.md#debug-endpoints), and bind it to loopback only). Then, from the
same host:

```bash
# 30 s CPU profile, opened in the interactive viewer
go tool pprof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'

# Live heap (inuse_space) and everything allocated since start (alloc_space)
go tool pprof http://127.0.0.1:6060/debug/pprof/heap
go tool pprof -sample_index=alloc_space http://127.0.0.1:6060/debug/pprof/heap

# Goroutines grouped by stack; the first line is "goroutine profile: total N"
curl -s 'http://127.0.0.1:6060/debug/pprof/goroutine?debug=1' > goroutines.txt

# 5 s execution trace: scheduler latency, GC phases, blocking
curl -s -o trace.out 'http://127.0.0.1:6060/debug/pprof/trace?seconds=5' && go tool trace trace.out
```

Inside `go tool pprof`, `top -cum` lists the most expensive call paths and `peek <regexp>` shows who
calls a function. Add `-http=127.0.0.1:8081` to open a flame graph in a browser instead.

### Finding A Goroutine Leak On A Live Server

Each connected game client costs the gameserver two goroutines: the read loop
(`network.(*GameClientLink).Handle`) and the write loop (`network.(*Conn).writeLoop`). Everything
else is a fixed set started at boot: scheduler tickers, persistence workers, the executor pool, the
accept loops, the login link, and the database driver. So `total` from the goroutine profile should
stay close to `fixed + 2 × connections`, with `connections` read from `/debug/vars`.

A client that drops while its character is in attack stance keeps its read loop alive for 15 s, the
combat linger in `network.disconnectCombatDelay`, parked in `network.awaitDetachDelay`. That is
expected. A total that keeps growing while the connection count stays flat is a leak: take two
`debug=1` dumps a few minutes apart and compare the per-stack counts to find the stack that keeps
growing.

## Profiling Under Load

`tests/perf` drives many scripted clients against one in-process server: the production boot path,
the production tickers, the real executor pool, and the shared test MariaDB. It is skipped unless
`ACIS_PERF_CLIENTS` is set:

```bash
ACIS_PERF_CLIENTS=200 ACIS_PERF_DURATION=30s ACIS_PERF_PROFILE_DIR=/tmp/acis-prof \
  go -C acis_golang test ./tests/perf/ -run TestLoadBaseline -v -count=1 -timeout 12m
```

| Variable | Default | Effect |
|---|---|---|
| `ACIS_PERF_CLIENTS` | unset (skip) | Number of clients, each with its own character and monster. |
| `ACIS_PERF_DURATION` | `30s` | Length of the measured window. |
| `ACIS_PERF_PROFILE_DIR` | unset | When set, the run writes `cpu.pprof` for the measured window, then `heap.pprof` and `goroutine.txt` (debug=1) at its end, while every client is still connected. Do not combine it with `-cpuprofile`. |

The run logs step latency, process CPU, GC count and pauses, the GC's CPU time, bytes allocated in
the window, heap size, and goroutines under load. After the window it closes every client and fails
if the process does not return to its post-boot goroutine count within 25 s, which covers the 15 s
combat linger. The failure includes the grouped goroutine dump, also written as
`goroutine-leak.txt` when `ACIS_PERF_PROFILE_DIR` is set. Read the profiles with
`go tool pprof <dir>/cpu.pprof`.

The scripted clients run in the same process as the server, so their frame reads appear in the
profiles. Pass `-ignore 'startReader|runClient|ScriptedClient'` to `go tool pprof` to see only the
server's share.

## Measured Run (2026-10-05)

- Code: `origin/main` at `675b4935` plus this harness.
- Host: AMD Ryzen 7 5800X, 4 vCPU and 7 GB available to the container, `GOMAXPROCS=3`, Go 1.27.1.
- Workload: `ACIS_PERF_CLIENTS=200 ACIS_PERF_DURATION=30s`, one run per `GOGC` value.
- Every run passed: no disconnects or timeouts, every client walked and attacked, and the goroutine
  count went from 626 under load back to 26 after disconnect (baseline 27, which included the boot
  client's connection).

| `GOGC` | GCs | Pause total / max | GC CPU (% of process CPU) | Heap in use / reserved at window end | Step p50 / p99 / max | Process CPU |
|---|---|---|---|---|---|---|
| 100 (default) | 20 | 6.0 ms / 3.6 ms | 220 ms (1.4 %) | 47 / 82 MiB | 2.6 / 47 / 62 ms | 16.1 s |
| 200 | 9 | 1.4 ms / 0.2 ms | 98 ms (0.6 %) | 91 / 129 MiB | 2.2 / 41 / 63 ms | 15.4 s |
| 400 | 5 | 0.7 ms / 0.2 ms | 58 ms (0.4 %) | 103 / 197 MiB | 2.4 / 48 / 62 ms | 15.5 s |

Each run allocated about 635–655 MiB in the window, roughly 21 MiB/s.

Where the CPU goes (`GOGC=100`, 15.8 s of samples):

- About 64 % is the scripted clients reading frames, which is load-generator cost.
- About 21 % is the server's write loops. Nearly all of that is the `writev` syscall in
  `network.(*Conn).writeBatch`.
- About 7 % is executor-pool work (`sim.(*Pool).drain`): player attack thinking, attack and move
  broadcasts, and the region fan-out (`network.broadcastFrame`, about 5 %).
- No server-package function takes more than 1 % of samples flat. The runtime clock reads
  (`runtime.nanotime`, `time.runtimeNow`, about 2 % each) are shared with the clients.

Where the allocations go (`alloc_space`):

- About two thirds are the scripted clients' frame decoding.
- The largest server-side allocator is `world.(*State).AppendKnown` → `(*Region).appendObjectsExcept`,
  at 78 MiB over the run, about 11 % of the total.
- It is followed by the write path: `(*Conn).writeBatch` and `(*Conn).SendFrame`, about 31 MiB
  together.

### GC Decision: Keep The Runtime Defaults

- **GC cost is small.** At default settings the GC used 1.4 % of process CPU, and pauses stayed
  under 4 ms.
- **Raising `GOGC` buys little.** It saves about 1 % of CPU and doubles to quadruples the heap the
  process reserves.
- **The latency tail is not GC.** The p99 and max step latency stayed at 41–48 ms and about 62 ms at
  every setting, so GC does not explain them.
- **No tuning in code.** The servers neither call `debug.SetGCPercent` or `debug.SetMemoryLimit` nor
  ship a non-default `GOGC`.

Operators can still use the standard runtime environment variables without any server change:

- **`GOMEMLIMIT`** (for example `GOMEMLIMIT=1GiB`) makes the GC work harder as the heap nears a
  container's memory cap, rather than letting the process be killed for running out of memory. Set
  it below the cap, leaving room for non-heap memory.
- **`GOGC`** trades memory for GC CPU, as in the table above. Only change it after a profile of the
  real workload shows the GC as a material share of CPU.

Re-run the table above before revisiting this decision, at the player count you are targeting.
