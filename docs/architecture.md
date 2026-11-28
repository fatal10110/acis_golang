# Architecture

This page explains how the servers work, from the big picture down to the main packages. Read it
before you change code. Words in *italics* on first use are defined in the [glossary](glossary.md).
The [decision records](decisions/README.md) explain why things are built this way.

## What the system does

Lineage II is an online role-playing game. Players run a game *client* that connects to two servers:

- the **login server** checks the account and shows the list of game servers;
- the **game server** runs the game world: characters, monsters, fights, items, quests, clans and
  everything else.

This project is a Go version of *aCis*, an existing Java server for the *Interlude* client. It
behaves like the Java server, uses the same database, config files and game data, and talks to the
same unmodified client ([decision 0001](decisions/0001-stay-compatible-with-acis.md)).

## The big picture

```text
               ┌─────────────────────────┐
  client ─────►│ login server   :2106    │  accounts, server list, session keys
  (logging in) └───────────┬─────────────┘
                           │                ▲
                           │                │ GS-LS link :9014
                           ▼                │
                ┌─────────────────────┐     │
                │  MariaDB database   │     │
                └─────────────────────┘     │
                           ▲                │
                           │                │
               ┌───────────┴─────────────┐  │
  client ─────►│ game server    :7777    │──┘
  (playing)    │ + datapack + geodata    │
               └─────────────────────────┘
```

There are two programs and one database:

| Program | What it does | Reads |
| --- | --- | --- |
| `cmd/loginserver` | Accepts client logins on port 2106. Accepts game servers on the *GS-LS link* port 9014. | `loginserver.properties`, `serverNames.xml`, banned IPs, the account tables |
| `cmd/gameserver` | Accepts players on port 7777. Connects to the login server's link port. | the game `.properties` files, `hexid.txt`, the *datapack*, the *geodata*, all game tables |

Both programs use the same MariaDB database. One login server can serve several game servers.

## A player's journey

1. **Log in.** The client connects to the login server and sends the account name and password. The
   login server checks them and sends back the list of game servers.
2. **Pick a server.** The login server gives the client a one-time *session key* for that server.
3. **Connect to the game server.** The client sends the session key to the game server. The game
   server asks the login server over the GS-LS link whether the key is valid, and waits for the
   answer.
4. **Choose a character.** The game server shows the account's characters. The player picks one and
   enters the world.
5. **Play.** While the player is online, the game server tells the login server which accounts are
   in the game. If the same account logs in somewhere else, the login server asks the game server
   to disconnect the first session.

The game server proves who it is to the login server with the server id and key stored in
`hexid.txt`. You create that file once with `cmd/gsregister`. If the link drops, the game server
keeps running and reconnects in the background.

### The three protocols

All three use small binary messages, called *packets*, each with a 2-byte length in front. The
shared encoding code is in `internal/commons/wire`.

| Protocol | Between | Code | Encryption |
| --- | --- | --- | --- |
| Login | client and login server | `internal/loginserver/network`, `internal/loginserver/crypt` | RSA for the password, then Blowfish |
| GS-LS link | game server and login server | `internal/link` (shared by both sides) | RSA key exchange, then Blowfish |
| Game | client and game server | `internal/gameserver/network` | a rolling XOR cipher, keyed when the client says hello |

## Inside the game server

The game server is organized in layers. Code calls down into lower layers. Lower layers report
back up through events and interfaces, never by importing a higher layer:

```text
┌──────────────────────────────────────────────────────────────┐
│ Network     decode packets, check them, call the game rules, │  internal/gameserver/network
│             turn results and events into packets             │
├──────────────────────────────────────────────────────────────┤
│ Game rules  combat, skills, items, clans, quests, sieges, ...│  internal/gameserver/<system>,
│             live characters and monsters                     │  model/, skill/, handler/, script/
├──────────────────────────────────────────────────────────────┤
│ World       who is where, who can see whom, terrain          │  world/, geo/
├──────────────────────────────────────────────────────────────┤
│ Data        game data from files, saved data in MariaDB      │  data/xml, data/sql, persist/
└──────────────────────────────────────────────────────────────┘
```

The network layer handles the protocol only. It does not decide game rules: a handler decodes a
request, calls a game-rule function, and turns the result into packets. Game-rule code never builds
packets itself ([decision 0005](decisions/0005-domain-events-to-packets.md)).

## How a request is handled

Here is what happens when a player clicks a monster to attack it.

1. **Read.** The connection's read loop (one goroutine per player) reads the packet, decrypts it
   and decodes it into a Go struct.
2. **Check.** It checks that this kind of packet is allowed right now (a player cannot attack from
   the character selection screen) and that the client is not flooding the server.
3. **Queue.** The handler for this packet is put on the player's *actor queue*. The read loop waits
   until the handler is done before it reads the next packet, so a player's requests are always
   handled in order.
4. **Run the rules.** A worker goroutine runs the handler. It calls into the combat code, which
   checks the target, starts the attack and later applies damage to the monster.
5. **Report.** The combat code reports what happened as *events*, for example "attacked" or "died".
   The network layer turns each event into packets for the attacker and for every player who can
   see the fight.
6. **Send.** Each client's packets go into that client's outbound queue. A writer goroutine for each
   client encrypts them and sends them, in order.
7. **Save.** If the fight changes something that must be saved (experience, a dropped item), the
   change goes to the *persistence lanes*, which write it to the database in the background.

## Running many players at once

The game server must use every CPU core, without two goroutines changing the same character at the
same time.

**Each actor has its own queue.** An *actor* is a live player, monster or NPC. Each one owns a
first-in, first-out task queue (`sim.Queue`). Everything that changes the actor runs as a task on
that queue: its packet handlers, its timers and its regular ticks. Pets, summons and cubics use
their owner's queue.

**A pool of workers runs the queues.** There is one worker goroutine per CPU core. A worker takes a
queue that has work and runs its tasks. Two workers never run the same queue at once, so an
actor's tasks run one at a time. Different actors run in parallel.

This means most actor state needs no locks. A few values that other actors must change directly,
such as HP when someone hits you, have small locks of their own. `sim.AssertOwner` checks that
queue-owned state is only touched from its own queue.

A task must never wait on something slow, such as the database, because it would hold a worker that
other actors need. Database reads that a request needs run on the connection's goroutine, and the
result is posted back to the queue. Database writes go to the persistence lanes.
[Decision 0003](decisions/0003-per-actor-queues.md) explains why we chose queues over locks.

The goroutines in a running game server:

| Goroutine | How many | Job | Code |
| --- | --- | --- | --- |
| Read loop | one per client | read, decrypt, decode, check, hand to the queue | `network/client_loop.go` |
| Writer | one per client | encrypt and send packets in order | `network/conn.go` |
| Worker | one per CPU core | run actor queue tasks | `sim/pool.go` |
| Ticker | one per periodic system | regular jobs: regeneration, AI, decay, autosave, ... | `commons/scheduler` |
| Persistence lane | five | write to the database | `persist/worker.go` |
| Login link | one | talk to the login server | `network/loginlink.go` |

## Saving data

Game code does not write to the database directly. It builds a snapshot of what to save and hands
it to `persist.Worker`, which has five *lanes*:

- four lanes for players, items, clans and other owners. All saves for one owner go to the same
  lane, so they reach the database in the order they were made;
- one lane for game-wide systems (Olympiad, heroes, raid points, lottery), so a slow write there
  never delays a player's save.

Handing over a save never blocks. When the server shuts down, every lane finishes its work before
the database connection closes. The SQL lives in one *store* per group of tables, in
`internal/gameserver/data/sql`. [Decision 0004](decisions/0004-ordered-persistence-lanes.md) has
the details.

## Game data, terrain and scripts

**Game data** (the datapack) stays as files on disk: XML for items, skills, NPCs, spawns, zones and
shops, and HTML for NPC dialogs. Loaders in `internal/gameserver/data/xml` read them at boot. Some
can be reloaded while the server runs with the `//reload` admin command.

**Terrain** (geodata) says how high the ground is and where the walls are. The server uses it for
movement, line of sight and pathfinding (`internal/gameserver/geo`). It is large, so the server
keeps it in a cache file mapped into memory instead of on the Go heap. `cmd/geopack` builds the
cache ([decision 0008](decisions/0008-memory-mapped-geodata.md)). Doors add temporary walls on top.

**The world** (`internal/gameserver/world`) splits the map into a grid of regions. It answers "what
is near this point" and tracks which objects can see each other. When an object comes into or goes
out of view, players get the packets that make it appear or disappear.

**Scripts** hold most game content: quests, NPC behavior, teleporters, special NPCs, scheduled
events and spawn rules. They are plain Go code in `internal/gameserver/script`, not a separate
scripting language. `data/xml/scripts.xml` still decides which scripts run
([decision 0006](decisions/0006-scripts-as-compiled-go.md)).

**Object ids** for characters, items, clans and other saved objects come from
`internal/commons/idfactory`, which scans the ids already in the database at boot.

## Start and stop

Each program is assembled with [Uber Fx](https://github.com/uber-go/fx), a library that builds
each service from the services it depends on and runs start and stop steps in order
([decision 0002](decisions/0002-wire-servers-with-uber-fx.md)).

**Start.** The game server loads config, connects to the database, loads the game data and
geodata, builds the world and every game system, starts the regular jobs, and only then opens the
game port. The parts shared with the test harness live in `internal/gameserver/app`; the rest are in
`cmd/gameserver`, one file per system. If the database does not answer, start-up fails after a
time limit instead of hanging.

**Stop.** Shutdown runs the start steps in reverse:

1. The game port closes. Every client gets a goodbye packet, and each player is saved.
2. The regular jobs stop and the worker pool finishes its queued tasks.
3. Items and the persistence lanes are flushed, and each game system saves its state.
4. The database connection closes last.

The whole shutdown has a time limit (`gameServerStopTimeout` in `cmd/gameserver/main.go`) that adds
up the limit of every step. A test checks the sum. Give the process at least that long to stop:
`docker-compose.yml` allows 240 seconds.

## Package map

Each package's `doc.go` says what it owns. `go doc ./internal/gameserver/<package>` prints it.

| Area | Packages under `internal/gameserver/` |
| --- | --- |
| Start-up and runtime | `app` (shared start-up wiring), `sim` (actor queues), `persist` (database write lanes), `task` (regular jobs), `shutdown` (timed shutdown and restart) |
| Network | `network`, `network/clientpackets` (decoders), `network/serverpackets` (encoders), `network/cipher` |
| Live objects | `model/actor/...` (players, NPCs, summons, pets, cubics, AI, attack, cast, movement, events), `model/item`, `model/itemcontainer`, `model/zone`, and the static data types in `model/*` |
| World and terrain | `world`, `geo/...`, `zonepulse`, `fence` |
| Data | `data/xml` (file loaders), `data/sql` (database stores), `data/cache`, `data/manager` |
| Rule entry points | `handler/...` (chat, item use, skill use, targeting, admin command parsing), `skill/...` (skill definitions, effects, formulas, stats) |
| Items and trade | `inventory`, `enchant`, `augment`, `craft`, `multisell`, `merchant`, `privatestore`, `petitem`, `symbolmaker`, `itemapi` |
| Social | `clan`, `party`, `partymatch`, `request`, `social/relation`, `social/petition`, `bbs`, `wedding`, `duel`, `announcement` |
| Castles and PvP | `castle`, `castlemanor`, `clanhall`, `siege`, `cursedweapon`, `hero`, `olympiad`, `raidpoint` |
| Seven Signs | `sevensigns`, `festival`, `signspriest` |
| Activities and NPC services | `boat`, `derby`, `fishing`, `fishchamp`, `lottery`, `gatekeeper`, `classmaster`, `schemebuffer` |
| Scripts | `script` (engine), `script/catalog`, `script/quest`, `script/ai`, `script/feature`, `script/maker`, `script/task`, `script/teleport` |

Outside the game server:

| Path | Contents |
| --- | --- |
| `internal/loginserver` | the login server: client logins, the game server link, accounts, bans |
| `internal/link` | GS-LS link messages, shared by both servers |
| `internal/commons` | shared code: wire encoding, ciphers, database pool, logging, regular jobs, object ids |
| `internal/config` | `.properties` file parsing |
| `internal/gameservertest`, `internal/testsupport`, `internal/dbtest` | the test harness ([decision 0007](decisions/0007-behavior-first-tests.md)) |

## Logs, metrics and debugging

Both servers write structured logs to the files set in `logging.properties`
([observability.md](observability.md)). With `-debug-addr 127.0.0.1:6060`, a server also serves
profiling data (`pprof`), metrics (`expvar`), an IP ban API and, on the game server, an item API.
Bind it to loopback only ([decision 0010](decisions/0010-secure-defaults.md),
[run-servers.md](run-servers.md#debug-endpoints), [profiling.md](profiling.md)).

## Where to go next

- [development.md](development.md): build, test and contribute.
- [decisions/](decisions/README.md): why the system is built this way.
- [network-ownership-map.md](network-ownership-map.md) and
  [`internal/gameserver/network/AGENTS.md`](../internal/gameserver/network/AGENTS.md): the rules for
  the network package.
- [script-engine-plan.md](script-engine-plan.md): the full design of the script engine.
