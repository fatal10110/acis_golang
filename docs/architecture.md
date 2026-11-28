# Architecture

This page explains how the Go servers are put together: the processes, the protocols between
them, how the game server boots, which goroutines own which state, and where each kind of code
lives. It describes the code as it is today. For how to build and test it, see
[development.md](development.md). For how to run it, see [run-servers.md](run-servers.md).

## The system at a glance

The project is a Go port of the aCis server for the Lineage II Interlude client (protocol revision
746). It has two server processes and one shared MariaDB database:

```text
                    ┌──────────────────────┐
  game client ─────►│ loginserver  :2106   │  accounts, server list, session keys
  (login phase)     │                      │◄──────────┐
                    └──────────┬───────────┘           │ GS-LS link :9014
                               │ MariaDB               │ (registration, session-key checks,
                    ┌──────────▼───────────┐           │  online players, kicks, access levels)
                    │   acis database      │           │
                    └──────────▲───────────┘           │
                               │                       │
  game client ─────►┌──────────┴───────────┐           │
  (game phase)      │ gameserver   :7777   │───────────┘
                    │ datapack (XML, HTML) │
                    │ geodata              │
                    └──────────────────────┘
```

| Process | Binary | Listens on | Reads |
| --- | --- | --- | --- |
| Login server | `cmd/loginserver` | client port (`LoginserverPort`, 2106) and GS-LS link port (`LoginPort`, 9014) | `loginserver.properties`, `serverNames.xml`, `banned_ips.properties`, the `accounts` and `gameservers` tables |
| Game server | `cmd/gameserver` | game client port (`GameserverPort`, 7777); connects out to the login server's link port | the game `.properties` files, `hexid.txt`, the datapack under `-data-root`, geodata, every game table |

Both processes use the same database schema, the same config files and the same datapack layout as
the aCis Java server, so an existing installation can switch over; see
[ops/migrating-from-java.md](ops/migrating-from-java.md).

## Protocols

There are three wire protocols. All of them use little-endian framing with a 2-byte length prefix,
and they share the primitives in `internal/commons/wire`.

| Protocol | Between | Code | Encryption |
| --- | --- | --- | --- |
| Login | client ↔ login server | `internal/loginserver/network` (`clientpackets`, `serverpackets`), `internal/loginserver/crypt` | RSA for the credentials block, Blowfish for the stream |
| GS-LS link | game server ↔ login server | `internal/link` (shared message encode/decode), `internal/loginserver/gameserverlink.go`, `internal/gameserver/network/loginlink.go` | RSA key exchange, then Blowfish (`internal/commons/crypt`) |
| Game | client ↔ game server | `internal/gameserver/network` (`clientpackets`, `serverpackets`, `cipher`) | XOR rolling-stream cipher, keyed by `ProtocolVersion` |

A player's session crosses both servers:

1. The client logs in to the login server, which checks the account and returns the server list.
2. The client picks a server. The login server issues a play session key.
3. The client connects to the game server with that key. The game server asks the login server
   over the link (`PlayerAuthRequest`) whether the key is valid, and waits for the answer before it
   shows character selection.
4. While the player is in the game, the game server reports online accounts to the login server
   (`PlayerInGame`, `PlayerLogout`). The login server can kick a player (`KickPlayer`) when the
   account logs in again elsewhere.

The game server registers with the login server using the server id and key in `hexid.txt`, which
`cmd/gsregister` writes. If the link drops, the game server keeps running and reconnects in the
background.

## Repository layout

```text
cmd/                 binaries: the two servers and their command-line tools (see tools.md)
internal/
  commons/           shared by both servers: wire, crypt, db pool, logging, scheduler, id factory, ...
  config/            .properties parsing and typed config keys
  link/              GS-LS link messages, shared by both servers
  loginserver/       login server: client link, game server link, accounts, bans
  gameserver/        game server (see the package map below)
  gameservertest/    boots a full game server inside a test
  testsupport/       scripted game client and shared test helpers
  dbtest/            the shared test MariaDB and per-test databases
  botclient/         simulated players used by cmd/botswarm
  datadiff/          canonical dumps used to compare loaders
  provenance/        a repository check on production comments and names
tests/<domain>/      behavior suites: full flows through real packets and a real database
aCis_datapack/       test copy of the datapack (XML, HTML, SQL), without geodata
ops/                 Docker, systemd and datapack-manifest files
docs/                this documentation
```

## Game server boot and shutdown

`cmd/gameserver` builds the process as a [uber-go/fx](https://github.com/uber-go/fx) dependency
graph:

- **Providers** (`fx.Provide`) construct each service: config, logger, database pool, the loaded
  game data, the object id allocator, stores, the world, the actor pool, and each game system. The
  providers shared with the test harness live in `internal/gameserver/app`; the rest are still in
  `cmd/gameserver`, one file per system.
- **Invokes** (`fx.Invoke`) start things in a fixed order: scripts, residences and sieges, then the
  periodic tasks, then the debug listener, and last the game client listener (`startGameServer`).
  Clients can connect only once everything else is up.
- **Shutdown** runs the stop hooks in reverse order. The game listener stops first and sends each
  client its close frame, and each connection waits for its player's saves. After that the tickers
  stop, the actor pool drains, the item saves and the persistence lanes are flushed, the game
  systems (spawns, Seven Signs, Olympiad, ground items, ...) save their state, and the database pool
  closes last. The total time is
  capped by `gameServerStopTimeout` in `cmd/gameserver/main.go`. That cap is the sum of each step's
  own limit, and a test checks it. This is why `docker-compose.yml` gives the game server a long
  `stop_grace_period`.

Boot I/O that runs while the graph is built (the database ping, the object id scan, restoring
spawn state) is limited by `gameServerBootTimeout`. A database that does not answer fails boot
instead of hanging it.

The login server is a smaller fx graph built the same way in `cmd/loginserver`.

## Concurrency model

The game server keeps game state off locks wherever it can. Each kind of work runs on a known kind
of goroutine:

| Goroutine | Owns | Code |
| --- | --- | --- |
| Connection read loop, one per client | decoding, the opcode state gate, flood protection | `network/client_loop.go` |
| Connection writer, one per client | the outbound queue, encryption, send order | `network/conn.go` |
| Actor pool workers, `GOMAXPROCS` of them | running the tasks posted to actor queues | `internal/gameserver/sim` |
| Tickers, one per periodic system | fixed-rate ticks (regeneration, AI, decay, autosave, ...) | `internal/commons/scheduler` |
| Persistence lanes, five of them | database writes, in order per owner | `internal/gameserver/persist` |
| Login link reader | the GS-LS link | `network/loginlink.go` |

### Actor queues

Each live actor (a player, a summon, an NPC) owns a `sim.Queue`. A queue's tasks run one at a time
and in the order they were posted, on a pool worker. State that a queue owns is touched only from
that queue's tasks, and `sim.AssertOwner` checks this. Because two goroutines never run the same
actor's work at once, most actor state needs no mutex. The few values that other actors must read
(position, vitals, status flags) use atomics or small leaf locks.

A client packet for a player in the world goes through these steps:

1. The read loop decodes the frame and checks the opcode against the connection's state
   (`state.go`).
2. The handler registry (`client_dispatch.go`) finds the handler. Handlers wrapped in `whenLive`
   are posted to the player's queue.
3. The read loop waits for that task to finish before it reads the next frame. This keeps each
   client's requests and replies in order.
4. Database reads that a request needs run back on the connection goroutine (`whenLiveStaged`,
   `deferLookup`). Their result is posted back to the queue, so a slow database never blocks a pool
   worker.

In tests, the same queues run on `sim.Inline`, a single-goroutine executor with a virtual clock.
Tests are deterministic and never sleep. CI also runs every behavior suite on the real pool
(`ACIS_SIM_EXECUTOR=pool`).

### Persistence

Database writes caused by game state changes do not run on actor queues. They are enqueued as jobs
on `persist.Worker`, which has four lanes for owner ids (characters, items, clans) and one lane for game-wide services
(Olympiad, heroes, raid points, lottery, ...). Jobs for one owner run in the order they were
enqueued, so a later save can never land before an earlier one. `Enqueue` never blocks. Shutdown
flushes every lane before the database pool closes.

The SQL itself lives in stores in `internal/gameserver/data/sql` (and `internal/loginserver/data/sql`),
which the fx graph builds once.

### Domain events and packets

Domain code does not build packets. When something happens to an actor that clients must see, the
domain code emits a typed `event.Event` (`internal/gameserver/model/actor/event`) to the actor's
sink. In the network package, a type switch maps each event to packets, in the order clients
expect. Inbound handlers call domain APIs and turn the typed result into packets. Every rejection
must answer the client; see
[agents/action-response-contract.md](agents/action-response-contract.md).

The full ownership rules for the network package are in
[`internal/gameserver/network/AGENTS.md`](../internal/gameserver/network/AGENTS.md) and
[network-ownership-map.md](network-ownership-map.md).

## World, geodata and data

- **World** (`internal/gameserver/world`): the map is split into a grid of regions. The world
  answers spatial lookups and tracks which objects can see each other as they spawn, move and
  despawn. Visibility changes are what make the server send appear and disappear packets.
- **Geodata** (`internal/gameserver/geo`): height, movement, line-of-sight and pathfinding queries.
  It reads L2OFF or L2J region files. At boot it maps a packed cache file into memory
  (`-geodata-cache`, built by `cmd/geopack`). Doors add temporary blockers on top of the static
  regions (`geo/dynamic`).
- **Static data** (`internal/gameserver/data/xml`, `data/cache`): XML loaders for the datapack
  (items, skills, NPCs, spawns, zones, multisells, ...) and the HTML page cache. The data stays on
  disk and is never imported into the database. Some tables can be reloaded at runtime with the
  `//reload` admin command.
- **Persistent data** (`internal/gameserver/data/sql`): one store per table group.
- **Object ids** (`internal/commons/idfactory`): unique ids for characters, items, clans and other
  persisted objects. At boot it scans the ids already stored.

## Scripts

Quests, NPC behaviors, feature NPCs, teleporters, scheduled tasks and spawn makers are Go code in
`internal/gameserver/script`. Each script is a `Script` value built by a Go constructor.
`script/catalog` maps each `data/xml/scripts.xml` entry to its constructor, and `scripts.xml`
decides which scripts run. The engine calls a script's hooks (talk, kill, attack, see, timers, ...)
when those events happen in the game. The design and its rules are in
[script-engine-plan.md](script-engine-plan.md).

## Game server package map

| Area | Packages under `internal/gameserver/` |
| --- | --- |
| Composition and runtime | `app` (shared fx graph), `sim` (actor queues), `persist` (write lanes), `task` (periodic work), `shutdown` (timed shutdown and restart) |
| Network | `network` (connections, dispatch, login link, event-to-packet mapping), `network/clientpackets`, `network/serverpackets`, `network/cipher` |
| Live objects | `model/actor/...` (creature, player, npc, summon, pet, cubic, ai, attack, cast, move, event), `model/item`, `model/itemcontainer`, `model/zone`, and the other `model/*` static data types |
| World and terrain | `world`, `geo/...`, `zonepulse`, `fence` |
| Data | `data/xml`, `data/sql`, `data/cache`, `data/manager` |
| Rules entry points | `handler/...` (chat, item use, skill use, targeting, admin command parsing), `skill/...` (definitions, effects, formulas, stats) |
| Items and economy | `inventory`, `enchant`, `augment`, `craft`, `multisell`, `merchant`, `privatestore`, `petitem`, `symbolmaker`, `itemapi` (debug-listener item API) |
| Social | `clan`, `party`, `partymatch`, `request`, `social/relation`, `social/petition`, `bbs`, `wedding`, `duel`, `announcement` |
| Residences and PvP | `castle`, `castlemanor`, `clanhall`, `siege`, `cursedweapon`, `hero`, `olympiad`, `raidpoint` |
| Seven Signs | `sevensigns`, `festival`, `signspriest` |
| Activities and NPC services | `boat`, `derby`, `fishing`, `fishchamp`, `lottery`, `gatekeeper`, `classmaster`, `schemebuffer` |
| Scripted content | `script` (engine), `script/catalog`, `script/quest`, `script/ai`, `script/feature`, `script/maker`, `script/task`, `script/teleport` |

Each package's `doc.go` (or package comment) says what it owns; `go doc ./internal/gameserver/<pkg>`
prints it.

## Observability and operations

- Structured logs (zerolog) go to the files configured in `logging.properties`, under `-log-root`.
  See [observability.md](observability.md).
- `-debug-addr` serves `pprof`, `expvar` metrics (connections, players online, ticker timings), the
  IP ban API and the character item API. Bind it to loopback only. See
  [run-servers.md](run-servers.md#debug-endpoints) and [profiling.md](profiling.md).

## Further reading

The design plans in this directory record why the code looks the way it does and what is still
planned. [docs/README.md](README.md) lists them and their status.
