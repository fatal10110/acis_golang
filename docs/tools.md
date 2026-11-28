# Command-line tools

Every binary lives under `cmd/`. Run any of them with `go run ./cmd/<name> -h` to see its current
flags. Build the ones you deploy with `go build ./cmd/<name>`. The Docker image ships
`loginserver`, `gameserver`, `gsregister` and `accountmgr`.

| Tool | Purpose | Needs |
| --- | --- | --- |
| [`loginserver`](#loginserver) | the login server process | database, config |
| [`gameserver`](#gameserver) | the game server process | database, config, datapack, geodata |
| [`gsregister`](#gsregister) | register a game server id and write its `hexid.txt` | database |
| [`accountmgr`](#accountmgr) | create, update, delete and list accounts | database |
| [`geopack`](#geopack) | build the packed geodata cache before the first boot | geodata |
| [`botswarm`](#botswarm) | load test with simulated players | running servers |
| [`datadiff`](#datadiff) | dump or compare loaded datapack records | datapack |
| [`geoprobe`](#geoprobe) | compare geodata query answers with a captured dump | geodata |
| [`pageindex`](#pageindex) | regenerate the committed script page index | datapack |
| [`scriptfp`](#scriptfp) | regenerate the script fingerprint goldens | reference Java sources |
| [`testtiming`](#testtiming) | summarize `go test -json` output | a test run |

`cmd/packetdiff/` holds only a placeholder; it has no command yet.

Unless noted, a config-file flag defaults to a path under `config/` in the current directory, as in
a reference server directory. A repository checkout has no `config/` directory, so pass the paths
explicitly when you run from it.

## Servers

### loginserver

```bash
go run ./cmd/loginserver \
  -config ../aCis_gameserver/config/loginserver.properties \
  -logging ../aCis_gameserver/config/logging.properties \
  -server-names ../aCis_datapack/data/serverNames.xml \
  -banned-ips ../aCis_gameserver/config/banned_ips.properties \
  -log-root log/login
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config` | `config/loginserver.properties` | login server properties |
| `-logging` | `config/logging.properties` | logging properties |
| `-server-names` | `serverNames.xml` | server id to name list |
| `-banned-ips` | `config/banned_ips.properties` | permanent IP bans |
| `-log-root` | `.` | directory that log paths resolve against |
| `-debug-addr` | off | `host:port` for pprof, expvar and the IP ban API; loopback only |

### gameserver

See [run-servers.md](run-servers.md#start-gameserver) for a full command line.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config` | `config/server.properties` | server properties (database, ports, login link) |
| `-players-config` | `config/players.properties` | player rules |
| `-npcs-config` | `config/npcs.properties` | NPC rules |
| `-clans-config` | `config/clans.properties` | clan rules |
| `-events-config` | `config/events.properties` | events |
| `-siege-config` | `config/siege.properties` | sieges |
| `-logging` | `config/logging.properties` | logging properties |
| `-hexid` | `config/hexid.txt` | server id and key from `gsregister` |
| `-geo-config` | `config/geoengine.properties` | geodata regions and pathfinding settings |
| `-geodata-cache` | `geodata.cache` | packed geodata cache; built when missing or stale; empty loads region files onto the heap |
| `-data-root` | `.` | datapack root that contains `data/xml` |
| `-log-root` | `.` | directory that log paths resolve against |
| `-debug-addr` | off | `host:port` for pprof, expvar, the IP ban API and the item API; loopback only |

## Setup and administration

### gsregister

Registers a game server in the login database: it picks a server id from `serverNames.xml`,
creates the server's auth key, stores the registration and writes the hexid file the game server
sends when it links.

```bash
# Interactive
go run ./cmd/gsregister -config <loginserver.properties> -names <serverNames.xml>

# Non-interactive; a rerun with the same registration changes nothing
go run ./cmd/gsregister -config <loginserver.properties> -names <serverNames.xml> \
  -register 1 -out <config>/hexid.txt
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config` | `config/loginserver.properties` | where to find the database |
| `-names` | `serverNames.xml` | server id to name list |
| `-register` | `0` (prompt) | register this id without prompting, then exit |
| `-out` | `config/hexid.txt` | the hexid file `-register` writes or checks |

### accountmgr

An interactive menu that creates, updates and deletes login accounts and lists accounts by access
level.

```bash
go run ./cmd/accountmgr -config <loginserver.properties>
```

### geopack

Builds the packed geodata cache that the game server maps into memory at boot, from the regions
`geoengine.properties` lists, and reports its size by block kind. Run it at deploy time so the
first boot does not have to build the cache.

```bash
go run ./cmd/geopack -config <geoengine.properties> -data-root ../aCis_datapack -out geodata.cache
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-config` | `config/geoengine.properties` | region list and `GeoDataPath` |
| `-data-root` | `.` | resolves `GeoDataPath`, as the game server's `-data-root` does |
| `-geodata` | from `-config` | region file directory, overriding `GeoDataPath` |
| `-out` | `geodata.cache` | cache file to write; pass the same path to the game server's `-geodata-cache` |

## Load testing

### botswarm

Runs simulated players against running servers over the real client protocol. It prints latency,
throughput and error counts, and can write them as JSON. Each bot plays account `<prefix><i>`, so
the login server needs `AutoCreateAccounts = True` or existing accounts. See
[run-servers.md](run-servers.md#bot-swarm).

| Flag | Meaning |
| --- | --- |
| `-login` | login server client address (`host:port`) |
| `-game-addr` | game server address, overriding the one in the server list |
| `-bots`, `-prefix`, `-password` | how many bots and their account names and password |
| `-rate` | bots started per second (`0`: all at once) |
| `-duration`, `-actions`, `-interval` | run length, behavior ticks per bot, pause between ticks |
| `-behaviors` | comma-separated cycle of `move`, `validate`, `chat`, `attack`, `idle` |
| `-move-radius` | largest move offset per axis |
| `-revision` | client protocol revision (default 746) |
| `-timeout` | handshake and logout timeout |
| `-json` | also write the report as JSON to a file (`-` for stdout) |

Exit codes: `0` when every bot entered the world without an error or disconnect, `1` otherwise,
`2` on a usage error.

## Parity and data tools

These tools compare what the Go code loads or answers with a dump from somewhere else, such as the
aCis Java server. They all use the same exit codes: `0` means success (a dump was written, or a
comparison found no differences), `1` means a comparison found differences, and `2` means the tool
could not run.

### datadiff

Dumps a data category's loaded records in a stable, field-by-field text form, or compares that
form with an expected dump.

```bash
go run ./cmd/datadiff -list
go run ./cmd/datadiff -category=skill > skill.dump
go run ./cmd/datadiff -category=skill -expected-dump=skill.dump
```

`-datapack` is optional. Without it the tool uses `ACIS_DATAPACK` when it is set, and otherwise the
nearest `aCis_datapack/` found walking up from the current directory, so inside the checkout the
in-repo copy wins. `-dump` reads a saved dump instead of loading a datapack.

### geoprobe

Runs a sample of geodata queries (height, can-move, line of sight, path) against the Go geo engine,
or replays a captured dump of expected answers and reports how well they agree.

| Flag | Meaning |
| --- | --- |
| `-config`, `-geodata`, `-geotype` | where the region files are and their format (`L2OFF` or `L2J`) |
| `-queries` | number of random queries to generate (default 1000) |
| `-dump` | where to write the queries and answers (default stdout) |
| `-expected-dump` | a captured dump to replay instead of generating queries |
| `-allow-empty-geodata` | run against the null engine when no region files exist |

## Generators for committed test data

Tests compare these files with the datapack or the reference sources and fail when they are out of
date. Rerun the generator and commit its output.

### pageindex

Writes `internal/testsupport/scriptpages/index.jsonl`: one record per page under
`data/html/script`, with its bypass commands, placeholders and content hash, but not the page text.

```bash
go run ./cmd/pageindex -datapack ../aCis_datapack -o internal/testsupport/scriptpages/index.jsonl
```

### scriptfp

Writes `reference.golden` (fingerprints of the reference server's script classes) and
`census.golden` (the engine calls those classes make) into `internal/testsupport/scriptfp`. It needs
the aCis Java sources.

```bash
go run ./cmd/scriptfp -java <reference java source dir> -o internal/testsupport/scriptfp
```

## Test helpers

### testtiming

Reads a complete `go test -json` stream on stdin and lists the slowest packages and top-level tests,
without counting subtests twice. CI uses it at the end of each shard.

```bash
go test -json ./tests/items | go run ./cmd/testtiming -n 15
```
