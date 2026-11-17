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

To load a deployed or locally started server from outside its process, use the `botswarm` client
([Running Loginserver And Gameserver](run-servers.md#bot-swarm)) and profile the server through its
debug endpoint as above.

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

### Variants

The baseline puts every client at one spot, on the always-passable test geodata, with no scripts. Each
variant below changes one thing, keeps the baseline's pass criteria (no disconnects, no timeouts,
goroutines back to baseline) and skips unless `ACIS_PERF_CLIENTS` is set. Select one or more with
`ACIS_PERF_VARIANT=<names>` (comma-separated):

| Variant | What changes | Extra setting |
|---|---|---|
| `spread` | Clients stand in a square grid of regions, 4096 apart, instead of one spot, so the region fan-out and visibility bookkeeping see many occupied regions. | `ACIS_PERF_REGIONS` (default 16) |
| `geo` | Clients and monsters stand on the shipped geodata of a Gludio tile, so movement and attacks go through `CanMove` and `Height`. Skips without geodata (it is not in the repository: `ACIS_GEODATA_DIR`, or an `aCis_datapack` that carries it). Does not combine with `spread`. | `ACIS_GEODATA_DIR` |
| `scripts` | Every monster has a script on the production registry path, subscribed to the attack hook; the run logs how many hooks the scripts heard and fails if none ran. | none |
| `cast` | Every client also casts a skill at its monster (two steps of eight) and every monster, wired with the AI-cast seam, answers each hit with a cast desire at its attacker, so the cast controller, the effect dispatch and the NPC cast-desire path run under load. The run fails unless every client cast and saw its monster cast. Does not combine with `scripts` or `catalog`. | none |
| `catalog` | The monsters are copies of shipped templates, each bound to a different script of the production content catalog (`scripts.xml` plus `internal/gameserver/script/catalog`, with the shipped NPC and skill tables), so those scripts' attack hooks run on every hit and `go tool pprof -focus gameserver/script` has samples. Needs a datapack. Implies `spread` with one region per client, because the shipped behaviors aggro and call their clan, and 200 clients at one spot would put crowds of monsters on the first few. The engine catalogs (quests, features, teleporters, tasks and the base behaviors) live in `cmd/gameserver` and are not in it. Does not combine with `scripts`, `cast` or `geo`. | `ACIS_PERF_CATALOG_NPCS` (default 24), `ACIS_PERF_REGIONS` |
| `fixture` | The database holds a production-sized population before the clients enter: `ACIS_PERF_ITEMS` inventory items per client, `ACIS_PERF_BG_CHARS` other characters and `ACIS_PERF_BG_ITEMS` other item rows, all bulk-inserted. The run ends by stopping the server with every client still connected and logs the drain and the detach (every connection saving at once on the persistence lanes); it fails if the stop outlasts `network.LivePlayerPersistWait`, the time a connection waits for its player's saves. The selection latency shows the items table's cost, since `items.owner_id` has no index. | `ACIS_PERF_ITEMS` (120), `ACIS_PERF_BG_CHARS` (2000), `ACIS_PERF_BG_ITEMS` (500000) |

Each variant is one command, the baseline command with the variant set:

```bash
ACIS_PERF_VARIANT=spread ACIS_PERF_CLIENTS=200 ACIS_PERF_DURATION=30s ACIS_PERF_PROFILE_DIR=/tmp/acis-prof-spread \
  go -C acis_golang test ./tests/perf/ -run TestLoadBaseline -v -count=1 -timeout 12m
```

Measured on the 2026-11-17 host (4 vCPU, `GOMAXPROCS=3`), `ACIS_PERF_CLIENTS=200 ACIS_PERF_DURATION=30s`, every
run passing:

| Variant | Step p50 / p99 / max | Process CPU | Allocated |
|---|---|---|---|
| baseline | 4.5 / 68 / 167 ms | 20.7 s | 1029 MiB |
| `spread` (16 regions) | 2.6 / 7.2 / 9.8 ms | 3.9 s | 109 MiB |
| `geo` | 3.9 / 90 / 124 ms | 18.7 s | 1066 MiB |
| `scripts` | 4.1 / 55 / 148 ms | 21.0 s | 1037 MiB |
| `cast` | 6.9 / 70 / 103 ms | 26.4 s | 1327 MiB |
| `catalog` (200 regions) | 2.7 / 4.4 / 8.7 ms | 2.7 s | 57 MiB |
| `fixture` | 3.8 / 55 / 162 ms | 21.0 s | 1027 MiB |

What each new variant shows, same run:

- `cast`: 1890 player and 1251 monster `MagicSkillUse` frames in the window; `go tool pprof -top -cum -focus 'actor/cast'` puts 0.70 s of the 26.2 s CPU (2.7%) in the cast controllers, and the monsters' cast desires run inside `ai.(*Attackable).think`.
- `catalog`: `-focus gameserver/script` is 0.03 s of 2.5 s (1.2%): the shipped scripts' hooks, dispatched through `script.(*Registry).run`. The other variants' fan-out dominates their CPU; this one has none, so the script engine's own share is visible.
- `fixture` (24000 client items, 2000 other characters, 500000 other item rows, seeded in 2.2 s): selection latency p50 / p99 / max = 99 / 104 / 114 ms against 4.8 / 5.7 / 7 ms without it, the unindexed `items.owner_id` read at each character selection. Stopping the server with all 200 clients connected took 3 ms to drain and 171 ms for every connection to detach and save, against the 16 s a connection waits for its saves.

The spread run is cheap because a client then sees only its own region: the baseline's cost is the
every-client-sees-every-client fan-out.

The log's `load baseline: variant=...` line names the variant; the step p50 / p99, process CPU and
the three profiles are reported as for the baseline. Not covered: the engine catalogs' scripts (quests, features,
teleporters, tasks, base behaviors) in the `catalog` variant, and a client that casts a shipped skill (a shipped
skill would one-shot the 36-HP test class).

The run logs step latency, selection latency, process CPU, GC count and pauses, the GC's CPU time, bytes allocated in
the window, heap size, and goroutines under load. After the window it closes every client and fails
if the process does not return to its post-boot goroutine count within 25 s, which covers the 15 s
combat linger. The failure includes the grouped goroutine dump, also written as
`goroutine-leak.txt` when `ACIS_PERF_PROFILE_DIR` is set. Read the profiles with
`go tool pprof <dir>/cpu.pprof`.

The scripted clients run in the same process as the server, so their frame reads appear in the
profiles. Pass `-ignore 'startReader|runClient|ScriptedClient'` to `go tool pprof` to see only the
server's share.

Selection latency is each client's `RequestGameStart` to `CharSelected`, taken while the clients
enter the world one at a time before the window. It covers the selection's wait for the character's
queued saves on its persistence lane plus the restore. A wait that passes half its budget also logs
a `warn` naming the lane and the owner (`select character: wait for queued saves passed half its
budget`), so a lane that couples selections on a live server shows up in the log.

### Selection Latency (2026-11-03)

`ACIS_PERF_CLIENTS=200 ACIS_PERF_DURATION=30s` on `origin/main` at `0696a8cd` plus the selection
measurement, same host class (4 vCPU, `GOMAXPROCS=3`, Go 1.27.1): selection p50 / p99 / max =
4.8 / 5.6 / 5.8 ms; step p50 / p99 / max = 3.6 / 59 / 139 ms. At 50 clients for 10 s: selection
4.7 / 5.2 / 5.4 ms. The wait is far below its 16 s budget. These numbers were measured with idle
lanes: no logout backlog sits ahead of a selection, so the shared-lane coupling under a write
backlog is not exercised. The half-budget warn is what surfaces that coupling on a live server, and
is the evidence to collect before publishing the saved state in memory to skip the wait.

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
