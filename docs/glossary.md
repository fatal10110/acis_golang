# Glossary

Terms used across the code and the documentation, in alphabetical order.

- **aCis**: A Java server for the Lineage II Interlude client. This project is a Go
  version of it, and the Java code is the *reference* for how the game must behave.
- **Actor**: A live object in the game that acts on its own or on a player's orders: a player
  character, a monster, an NPC, a summon or a pet. Each actor has an *actor queue*.
- **Actor queue**: A first-in, first-out list of tasks owned by one actor (`sim.Queue`). The actor's
  tasks run one at a time on a pool of worker goroutines. See [decision
  0003](decisions/0003-per-actor-queues.md).
- **Admin command**: A command a game master types in chat, starting with `//`, such as `//reload`
  or `//teleport`.
- **Client**: The Lineage II game program a player runs. The servers support the Interlude client,
  protocol revision 746.
- **Datapack**: The game data files: XML (items, skills, NPCs, spawns, zones, shops, scripts list),
  HTML (NPC dialogs), and the SQL that creates the database. The full datapack also holds geodata.
  The repository's `aCis_datapack/` is a copy without geodata, used by tests.
- **Event**: A typed record of something that happened to an actor, such as `Died` or `Moved`
  (`model/actor/event`). Game code emits events; the network layer turns them into packets. See
  [decision 0005](decisions/0005-domain-events-to-packets.md).
- **Fx**: [Uber Fx](https://github.com/uber-go/fx), the library that assembles each server from its
  services and runs their start and stop steps in order. See [decision
  0002](decisions/0002-wire-servers-with-uber-fx.md).
- **Game server**: The program that runs the game world (`cmd/gameserver`). Players connect to it
  after they log in.
- **Geodata**: Terrain data: the height of the ground and the walls in every cell of the map, used
  for movement, line of sight and pathfinding. It comes as region files in L2OFF (`*_conv.dat`) or
  L2J (`*.l2j`) format. See [decision 0008](decisions/0008-memory-mapped-geodata.md).
- **GS-LS link**: The protocol between a game server (GS) and the login server (LS). The game server
  registers over it, checks players' session keys, and reports who is online.
- **hexid**: The file (`hexid.txt`) that holds a game server's id and secret key. `cmd/gsregister`
  writes it, and the game server presents it when it links to the login server.
- **Interlude**: The version (chronicle) of Lineage II this server supports.
- **Login server**: The program that checks accounts and shows the server list (`cmd/loginserver`).
- **NPC**: Non-player character: any character the server controls, such as a shopkeeper, a guard or
  a monster.
- **Opcode**: The first byte of a packet, which says what kind of packet it is. Some packets add a
  second "extended" opcode.
- **Packet**: One binary message between a client and a server, or between two servers.
- **Persistence lane**: One of the five background goroutines that write to the database in order
  (`persist.Worker`). See [decision 0004](decisions/0004-ordered-persistence-lanes.md).
- **Reference**: The aCis Java server, used to decide and test how the Go server must behave.
- **Region**: A square area of the map. The world is a grid of regions. Geodata is also split into
  region files.
- **Script**: Game content written as Go code: a quest, an NPC behavior, a teleporter, a scheduled
  event or a spawn maker (`internal/gameserver/script`). See [decision
  0006](decisions/0006-scripts-as-compiled-go.md).
- **Session key**: A one-time key the login server gives a client for one game server. The game
  server checks it with the login server before it lets the player in.
- **Store**: A Go type that reads and writes one group of database tables (`data/sql`).
- **Ticker**: A goroutine that runs a job at a fixed rate, such as regeneration or autosave
  (`commons/scheduler`).
- **Virtual clock**: The test clock used by `sim.Inline`. Time moves only when a test says so, so
  tests never sleep. See [decision 0007](decisions/0007-behavior-first-tests.md).
