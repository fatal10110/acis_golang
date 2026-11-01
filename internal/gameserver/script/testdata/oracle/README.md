# Script engine reference probe

This directory holds the reference probe for the script engine (slice V0 of
`docs/script-engine-plan.md`, section 11 item 1) and what it produced:

- `manifest.golden`: the script registration manifest for every script the datapack's
  `data/xml/scripts.xml` lists (gate 1).
- `trace_q001.golden`, `trace_q003.golden`: two quests played through client packets, with
  every server packet and SQL statement they caused (gate 2).
- the engine-contract goldens of slice V3 (section 11 item 3), written to
  `internal/testsupport/scriptcontract/testdata/` (see "Engine-contract goldens" below).
- `scenarios/*.golden`: the reference traces of the quest scenarios in
  `tests/quest/testdata/scenarios` (see "Quest scenario traces" below).

Gate status at the committed revision: **gate 1 passes** (857 of 857 listed scripts built
and recorded, no fallback in force); **gate 2 passes** (both quests are played to their
completed state, which the probe requires before it writes a trace, and two consecutive
runs produce byte-identical traces; no fallback in force).

## What the probe is

The whole reference tree (`aCis_gameserver/java`, 2,343 files) is compiled with `javac` in
the `eclipse-temurin:21-jdk-alpine` image, with four infrastructure classes replaced:

| Reference class | Probe version |
|---|---|
| `commons/pool/ThreadPool.java` | `shadow/.../ThreadPool.java`: no threads. `execute` runs inline; scheduled tasks wait on a virtual clock that only the probe advances, then run inline in due-time order (ties in scheduling order). |
| `commons/random/Rnd.java` | `shadow/.../Rnd.java`: one seeded `java.util.Random` with the reference bounds; the trace reseeds it before each quest. A scenario trace can pin the script draws alone (`Rnd.scriptCallers`, below). |
| `gameserver/network/GameClient.java` | the reference file, with the socket write of `sendPacket` replaced at build time by `ProbeWire.capture` (`probe/net/sf/l2j/commons/mmocore/ProbeWire.java`), which writes the packet into a little-endian buffer exactly as the selector does before encryption. The `runImpl` after the send is kept. |
| `commons/pool/ConnectionPool.java` | `shadow/.../ConnectionPool.java`: no database. Updates touch nothing; a `SELECT` from a table the datapack's install scripts (`aCis_datapack/sql/*.sql`) seed returns those rows, as a fresh install would (castle, clan hall, seven signs status and festival, derby bets); any other query returns no row. While a scenario trace plays, the rows of the tables it keeps (`characters`, `items`, `character_quests`, `character_memo`) live in `shadow/.../ProbeStore.java`: the statements on them change and read those rows as the database would (see "Quest scenario traces"). |

Every other read of the wall clock in the reference tree (`System.currentTimeMillis()`) is
pointed at `ProbeClock.wall()` at build time: the real clock, except while a scenario trace
plays, when it is a virtual one (2026-01-01 00:00 UTC at the scenario's start, moving with the
virtual time), so a trace does not depend on when the probe ran.

`probe/ScriptProbe.java` then runs the reference boot sequence of the game server up to and
including the script loader (`ScriptData`), minus logging setup, sockets and the login
link. Geodata is not loaded (every region falls back to the reference's null block);
nothing in registration reads it.

Before the script loader runs, every NPC template's event map is replaced with a recording
map. The template's own registration code (`NpcTemplate.addQuestEvent`) still decides the
result; the recording map only notes each call. That is how each script's own bindings are
captured, including registrations driven by collections, loops and the behavior
reflection pass (`Quest.feedEventHandlers`), and including registrations the template then
refuses (a second first-talk script).

## Engine-contract goldens

`probe/ContractProbe.java` runs last. It drops every task the boot queued
(`ThreadPool.cancelAll`), so no boot task runs inside a scene (the festival manager, due
120 s of virtual time after boot, blocks in real time), then exercises the reference classes behind each engine contract on
probe-made players, NPCs and scripts, and writes one golden file per contract family to
`internal/testsupport/scriptcontract/testdata/` (`journal`, `questlist`, `drops`,
`range`, `dialog`, `timers`, `schedule`, `fanout`, `aggro_tick`). The `*_hand.golden`
files beside them are derived by hand from the cited lines and reviewed blind; `run.sh`
never writes them. Package `scriptcontract` embeds both and lists, per contract, the
tables and the slices that implement it.

Two more build-time changes serve these goldens, and leave the reference behavior alone
until a contract scene uses them:

- the shadow random source can be scripted (`Rnd.script`): its int draws then return given
  values and record each draw's bound; any other draw while scripted is an error;
- `scripting/ScheduledQuest.java` is compiled with its clock read and its
  `Calendar.getInstance()` pointed at `ProbeClock`
  (`probe/net/sf/l2j/commons/probe/ProbeClock.java`), which answers as the reference
  does until the schedule scene sets an instant, zone and locale.

## Quest scenario traces

`probe/ScenarioTrace.java` runs last, after the contract scenes. It plays every scenario
file of `tests/quest/testdata/scenarios` and `tests/ai/testdata/scenarios` whose `trace` line names a file under
`oracle/scenarios/`, from the scenario file itself, and writes that file. Each scenario is set
up the way the Go runner (`internal/testsupport/scenario`) sets it up, so the trace answers
what the reference does in the scenario's own world:

- unless its first step creates it, the player is created at the character line's level and
  the class line's class (default 0), standing where the Go fixture character enters the world (10, 20, 30); a subclass
  line gives it that subclass in slot 1 at its level, its row written as the Go runner writes it, and
  makes it the played class, as the reference's restore leaves a character that logged out on it
  (the base class keeping the character line's level); a clan line creates a
  clan the player leads and sets its level; a siege line registers that clan on its side of the
  castle's siege (the castles load at boot), or, for owner, makes it the castle's owner on its
  siege's owner side, as the reference's restores leave a stored registration or ownership; each
  quest line puts its rows in the player's quest state before anything is recorded;
- a `clan <level> hall <id>` line makes the clan the owner of that clan hall (`setOwnerId` and
  `Clan.setClanHallId`, as the clan hall restore leaves a stored owner); a `hall <id> <status>
  [date <ms>]` line puts the siegable hall's siege at that status (registering, waiting for its
  battle, in progress) and, with a date, its stored next siege date (the reference replaces a stored
  past date at boot, so a scenario stores a future one); a `hallstatus <id> <status>` step changes
  it. The halls a scenario touched are given back (status, date, owner) after it. A `mate <name>
  <level> leader` line (the only mate a traced scenario takes) is the clan's leader: it creates the
  clan and the player joins it as a plain member. Offline, it is a character with no client. With
  `sponsor` or `apprentice` it is the player's sponsor or apprentice (the ids set on both sides); with
  `online` (and `hold <item id> <count>` lines, an online mate only) it is in the world beside the
  player, at its offset, in a client of its own (its own account, a `GameClient` of its own); an
  `enter <mate>` step makes an offline mate select its stored character and enter the world on its
  own client, so the clan's members and its sponsor or apprentice are told, as the reference does.
  An `as <mate> <action|bypass|link>` step sends that packet from the mate's client. The
  `away` step moves the player 1500 behind where it entered, out of every NPC's sight and the
  mate's reach, and runs one NPC AI tick;
- each NPC line's NPC is spawned at its offset from the player. Every NPC template is bound to
  the scenario's script and stub lines only, from the bindings the script loader left (copied
  before the contract scenes change some), since the Go runner registers no other script:
  another quest on the same NPC is not in its chooser, and a monster's own behavior does not
  run;
- each NPC line's NPC has the casting stats of the Go runner's fixture NPC of its type, which
  every NPC line's NPC has there: WIT and DEX 0, and a base attack speed of 253 for a civilian
  kind (a Folk instance class, the siege towers and flag, not the Christmas tree), 300 for any
  other; a cast's hit time and reuse delay follow them, so its `MagicSkillUse` matches the Go
  run's;
- an NPC the scenario kills or hits is made the Go runner's fixture monster of the NPC line's
  type: level 1 (the NPC line's level option, if any), 1000 HP, no experience, SP, drops, skills, minions or aggression, its script
  bindings kept. The templates get their own values back once the scenario is over;
- a random line pins the script draws alone: an int draw whose first caller outside `Rnd` is
  a scripting class takes the next listed value (a ranged draw takes its minimum plus a value
  of the range's size, a boolean is a value of bound 2, true when 0), and any other draw comes
  from the seeded generator, reseeded per scenario. A script draw past the listed values, or
  outside its bound, fails the run. Each pinned draw is a `# draw` line of the step that drew
  it; the Go run of the same step must draw the same bounds in the same order;
- the hold, equip, standing, pk, karma, warehouse, hero, noble, cabal, festival, sealowner, memo and spoil lines are seeded as
  the Go runner seeds them (inventory items, a worn item, the Ketra/Varka standing, the PK count,
  the karma, the private warehouse's items, the hero status,
  the noblesse status,
  the Seven Signs sign-up, the festival scores and the seal owners, both given back after the
  scenario, the player memo rows, and the spoiler of the monster of an NPC line: set after
  each spawn of it, as a spawn clears the spoil state, by the player or, with `other`, by an
  object id nobody holds); a traced scenario with a header line the probe does not read fails
  the run;
- an indented `live <npc id> <count>` line under a step makes the probe write a `# live` line
  after the step, the number of living NPCs of that template in the world (what a script
  spawned and despawned included); the Go run counts the same;
- an indented `spawned <npc id> <x>,<y> ...` line under a step makes the probe write a
  `# spawned` line after the step: where the living NPCs of that template that no NPC line placed
  (a script's spawns) stand, sorted, `-` for none; the Go run lists the same places. Only x and
  y: the probe runs with every geodata region removed, so a height it traces is the one asked for,
  not the ground's;
- a `door <role> <door id> <dx> <dy> <dz>` line binds the role to the shipped door of that id (the offsets are the Go
  fixture's; the probe's door stands where the datapack puts it), which the probe closes before the scenario and after it.
  An indented `open <role> <yes|no>`, `dungeon <yes|no>`, `standing player <n>` or `stance player <on|off>` line under a step
  makes the probe write a `# check` line after the step with what its world holds (the door's open state, the player's
  Seven Signs dungeon flag, Ketra/Varka standing, attack stance); the Go run compares its own world's answer line by line.
  A `stance player on|off` step adds the player to the attack stance task or removes it, and a scenario with such a step
  also runs the stance task after each slice of an `advance`, so the stance ends 15 seconds after it was set. The zone line,
  a `zone <boss|peace> <zone id> <dx1> <dy1> <dx2> <dy2> <dz1> <dz2>` line stands the shipped zone of that id (the datapack must have it)
  in that box around where the character enters the world, for the scenario only; the `appear` step, the bypass `xN` count,
  and a member's standing are the Go run's alone (a wanted `loc=x,y,z+-n` teleport line takes the trace's landing inside its box);
- the `teleports` line gives the Go run the shipped gatekeeper teleport lists, which the probe
  always loads; the half-price weekend hours of a standard window read the probe wall clock
  (`ProbeClock.wallCalendar`, a Thursday while a scenario trace plays, as the Go run's clock start
  is), so no window price depends on when the trace was written;
- the first countdown of the Derby track has run before the first scenario, as it has a second
  after the server starts: a race manager (a DerbyTrackManagerNpc) sends and reads the race
  packet and the runners as it appears and decays, which are null before it;
- every scenario's character has the same account (`scenario`) and a clan line's clan the same
  name (`Scenario`), since nothing of an earlier scenario is kept; its character, item, quest,
  memo and subclass rows are kept from its creation on (the quest lines' rows written as the Go runner writes them), so
  a relog stores the character at its logout and restores it from the rows the run wrote, read
  back by the reference's own restore queries; a statement on a kept table the store cannot run
  fails the run;
- the game time is noon of the first game day at each scenario's start; the wall clock starts
  at 2026-01-01 00:00 UTC, or at the `clock` line's time of day on that day, and the game tick
  (`GameTimeTaskManager`'s start time) is that instant, as the Go run's uptime is zero at its
  clock start. The Four Sepulchers manager (RoyalRushNpc) reads its minute and second from the
  probe wall clock;
- a `params <p1> <p2> <p3>` option of an `npc` line is set on the NPC (`_param1` to `_param3`)
  before it enters the world, so its created hook reads them, as a private's master gives them;
  a value is a number, the character or the role of an NPC line above. An `ai <name>=<value>,...`
  option replaces the NPC's template AI parameters with those alone (as the Go runner's fixture
  template has only them) until the scenario is over. The other options of the line (`walks`,
  ...) are not read: the probe plays the shipped template;
- an `event <role> <id> <arg1> <arg2>` step sends the NPC of that role the script event
  (`Npc.sendScriptEvent`) and runs what it queued now, without moving the virtual clock;
- the script events an NPC hears from a script's send or broadcast (`Npc.sendScriptEvent`,
  `Quest.broadcastScriptEvent`, `broadcastScriptEventEx`; the build routes the three delivery
  loops through `ProbeHeard.deliver`) are `# heard` lines of the step that caused them, one per
  quest that runs, whatever step or timer sent them. The event step's own delivery is its `# event`
  line, not a heard one. The Go run compares them per step as a sorted set, the receiver and
  an argument named by role: the order the NPCs around a broadcast hear it in follows each side's
  known list. The maker script events (`onMakerScriptEvent`) are not traced: the probe builds no
  NPC maker (#4337);
- a client packet step is followed by 500 ms of virtual time (the `T 500` line) and a kill
  by seven such ticks; the Go run settles the queues after a packet instead and moves its clock
  only for a kill and an `advance` step, so a timer a script starts in a packet step needs an
  explicit `advance` to line up (the Go step is shorter by the 500 ms);
- every scenario draws its object ids from the same point: the id factory as the first scenario
  found it, after the boot, the quest traces and the contract scenes. Once a scenario is over,
  every world object of an id it drew is removed, wherever it stands, and so are the registry
  entries keyed by one (the clan table, the castle sieges' registrations and the castle owners, the
  Seven Signs sign-ups, the raid points, and the task managers' creatures and items, since a world
  object equals any other of its id: the attack stance a cast leaves would show the next NPC of
  that id in combat); an object still in the world fails the run. Each world region's object map
  is rebuilt at a scenario's start, holding what stood there before the first scenario in object
  id order: a map keeps the capacity an earlier scenario grew it to, and the order a character
  entering the world is shown what stands around it follows that capacity. What loads from the database on first use is loaded before
  the first scenario, so no scenario's trace holds its query: the events the reference loads at
  boot after the scripts (the derby track, whose runners also draw ids, the lottery, the couples,
  the fishing championship when enabled) and the friend lists, which the reference loads when a
  first character enters or leaves the world.

A scenario's trace therefore depends on that scenario alone: adding, removing or renaming
another scenario leaves it byte-identical. A content lane that adds scenarios runs `run.sh` and
commits the whole result, which is its own new goldens only; a change to any other golden means
the lane changed what that scenario plays (a script, a data reader, the probe) and is part of
the review. When a new scenario writes another registry keyed by an object id, the probe clears
that registry too (`ScenarioTrace.Ids.forget`).

Every step is played, in order, and the trace records them all: action, bypass, link and mark
(`RequestTutorialQuestionMark`) are fed as client packets, followed by 500 ms; a relog is a
restart and an enter; a restart is a `RequestRestart`, an enter a `RequestGameStart` of slot 0
(the character select's flood protector reset first: the Go runner selects again at once) and an
`EnterWorld`, a create a `RequestCharacterCreate` of the Go runner's human fighter, the created
character then moved, in its kept row, to where the Go fixture character enters the world; each
of these packets is followed by 500 ms. A kill of N kills N monsters one by one, a fresh one at
the NPC line's offset once the last one is dead, each followed by 3.5 s (the Go runner's kill
wait); a hit takes the damage, followed by 500 ms; an advance passes its time in 500 ms ticks; a
pickup is an `Action` on the one ground item of the template, then 500 ms ticks until the item
has left the ground; an approach puts the player 100 in front of the NPC, on the -x side, and
runs the NPC AI task once; a tick runs it once. The NPC lines' NPCs are spawned when the
character first stands in the world (after the first enter of a created character), what their
spawning sends unrecorded. `internal/testsupport/scenario` then requires the scenario's steps to
replay the whole trace (package doc there). Two consecutive runs produce byte-identical traces.

## Contract golden format

Line-oriented. `#` lines are comments. Each table:

```
table <name>
source <path below aCis_gameserver> <from>[-<to>] <symbol...>   one or more
provenance probe java=<sha> datapack=<sha> | hand
note <text>                                                       zero or more
row <id> <key>=<value> ...                                        one or more
  <output line>                                                   the row's ordered outputs
```

A value is bare when it is printable ASCII without space, `"`, `\` or `=`; otherwise it is
double-quoted with backslash escapes for backslash, quote, `\n`, `\r`, `\t` and `\u00XX`
for other control characters. `-` stands for "none" or the empty list; lists are
comma-separated. The revision is the last commit of `aCis_gameserver/java` and of
`aCis_datapack/data` the probe ran on.

Output lines, in the order they happened:

- `S <packet>`: a server packet to the scene's player. NpcHtmlMessage, SystemMessage,
  PlaySound, ExShowQuestMark, QuestList and ActionFailed are decoded from the wire bytes
  (`scriptcontract.Packet` renders Go packets the same way); any other packet keeps its
  class name and hex body. Object ids are replaced by role names (`obj=darin`), and by
  `{role}` inside page text.
- `Q <sql> | <param> | ...`: an executed statement; `{player}` is the player's object id
  (`scriptcontract.ParseStatement` maps the journal statements to kinds).
- other lines (timers, fan-out, aggro scenes): the script hook calls the scene recorded.

## Regenerating

From the repository root, with the reference server and datapack checkouts:

```bash
ACIS_JAVA=../aCis_gameserver ACIS_DATAPACK=../aCis_datapack \
  internal/gameserver/script/testdata/oracle/run.sh
```

Both default to the siblings of the repository root, so from the primary checkout in the
workspace no variable is needed. The script builds the probe, runs it, copies the three
`.golden` files here, the scenario traces to `scenarios/` (replacing the directory) and the
contract goldens to `internal/testsupport/scriptcontract/testdata/`; it needs Docker and nothing
else; it reads the scenario files from `tests/quest/testdata/scenarios` and `tests/ai/testdata/scenarios`. `PROBE_LOG=<file>` keeps the
reference server's log.

A full run plays every scenario with the reference's real bypass throttle, so it takes several
minutes. `PROBE_ONLY=<globs>` (comma-separated file-name globs, for example
`PROBE_ONLY='q359_*,q362_*'`) plays only the matching scenarios and replaces only their traces;
the other goldens are regenerated as in a full run. Because a trace depends on its scenario alone,
a subset run writes the same bytes a full run would. It never removes the trace of a deleted or
renamed scenario: run without `PROBE_ONLY` for that.

Committed outputs were generated from the reference at workspace commit
`55ff8a4ec7e186d9816cd549246b4cf1f59c9f12` (the last change to `aCis_gameserver/java` and
`aCis_datapack/data`). Two runs give byte-identical outputs.

## Manifest format

Line-oriented, one record per line, fields separated by single spaces. Script keys are the
`scripts.xml` paths (the class name below `net.sf.l2j.gameserver.scripting.`).

```
listed <n> loaded <n>                      scripts.xml entries; instances the loader built
paths quest.* <n> script.* <n> task.* <n>  entries by path prefix
kinds behavior <n> quest <n> scheduled <n> script <n>

script <path>                              one block per scripts.xml entry, in file order
  kind behavior|quest|scheduled|script     behavior: an NPC behavior class; quest: quest id > 0
  quest <id> name <name> descr <rest of line>
  chain <class> < <parent> < ...           the class and its ancestors below the script base class
  items <id>,...                           ids removed when the quest ends
  on enter-world | on death                player-level triggers
  bind <EVENT>,... <npc ids>               the NPC events this script asked to be registered for;
                                           events with identical id sets share one line
script <path> missing                      the loader could not build it (none at this revision)

# classes                                  every class in any chain, sorted by name
class <class> extends <parent>
  hook <method>(<param types>) [event <EVENT>] [super...]...
                                           a method overriding an ancestor's; event: the NPC
                                           event its name binds under the behavior name rule
  named <method>(<params>) event <EVENT>   a non-overriding method that still binds by name
super[:<name>][@<owner>](args=same|changed,at=first|tail|mid|only)
                                           each parent call in the method, in bytecode order:
                                           :<name> when it calls another parent method,
                                           @<owner> when the target is not the direct parent,
                                           args=same when the receiver and every parameter are
                                           passed straight through, at=first when it is the first
                                           statement, tail when a return follows it, only when both

# folded npc events                        the per-(npc, event) script lists after the whole load
fold npc <EVENT>,... <script>,... <npc ids>
                                           in dispatch order; NPCs and events with the same list
                                           share one line

# folded other events
fold item <id> ITEM_USE <script>,...
fold zone <id> ZONE_ENTER|ZONE_EXIT <script>,...
fold door <id> DOOR_CHANGE <script>,...    (none at this revision)
fold game-time - GAME_TIME <script>,...    (none at this revision)
fold maker <name> MAKER_NPCS_KILLED <script>,...  (none at this revision)
```

Id lists are sorted; runs of three or more consecutive ids are written `a-b`.

Facts the manifest records that a port must keep:

- MY_DYING bindings fire 3 s after the death (`Npc.doDie` schedules them; visible in the
  Q003 trace), and a behavior's events are the union, by method name, of every on-method
  declared in its chain.
- `script.ai.boss.core.Core` declares a door-change hook but registers it only in a
  constructor the loader never calls, so no door has a script.
- No live registration is refused by the single first-talk slot. Only one NPC (35596) has
  its first talk claimed more than once, by four behaviors, and each replaces the one
  before it.

`../../oracle_manifest_test.go` checks the manifest against `scripts.xml` (same paths, same
order, all built) and replays every script's bindings through the registry rules of the
engine plan (section 2), requiring the result to equal the folded lists.

## Trace format

```
# <title>
C <client packet> <label>       a client packet the probe feeds through the packet's own read and run
T <ms>                          virtual time passed; due tasks run (inventory updates, AI ticks, ...)
S <packet class> <hex body>     a server packet sent to the player: opcode first, little-endian
Q <sql> | <param> | ...         an executed statement with its bound parameters in index order
# kill <npc id> (object <id>)   the probe kills a monster for the player
# hit <npc id> (object <id>) <damage>   a scenario trace: the player lands a hit of that damage
# advance <ms>                  a scenario trace: that much virtual time passes, in T ticks
# hallstatus <id> <status>      a scenario trace: the siegable hall's siege changes to that status (registering,
                                waiting, siege)
# draw <bound>:<value> <class>  a scenario trace: a script draw the random line pinned, and the
                                class that drew it
# npc <role> <npc id> (object <id>)   a scenario trace: the probe spawned the NPC line's NPC
# approach <npc id> (object <id>)     a scenario trace: the player steps up to the NPC, one AI tick
# tick                          a scenario trace: one run of the NPC AI task
# event <npc id> (object <id>) <id> <arg1> <arg2>   a scenario trace: the NPC is sent a script event
                                (the arguments as the scenario writes them)
# heard <npc id> (object <id>) <id> <arg1> <arg2>   a scenario trace: the NPC hears a script event a script
                                sent or broadcast (the arguments as numbers; an object id is a role to the Go run)
# live <npc id> <count>         a scenario trace: how many living NPCs of that template the world
                                holds once the step is over (the step's live line)
# spawned <npc id> <x>,<y> ...   a scenario trace: where the living NPCs of that template that
                                no NPC line placed stand once the step is over, sorted (the step's
                                spawned line); - for none
# stance <on|off>               a scenario trace: the player's stance step
# check <kind> <what> <value>   a scenario trace: an open, dungeon, standing or stance expectation of the step, with the
                                value the probe's world holds
C Action item <item id> (object <id>)   a scenario trace: the pickup's action on the ground item
# away                          a scenario trace: the player steps 1500 behind where it entered the world, one AI tick
# mate <name> object <id>       a scenario trace with a mate that has a client: the mate's object id
C@<mate> <client packet> <label>   a scenario trace: a client packet of the mate's client (an as step's, or the
                                RequestGameStart and EnterWorld of an enter step)
S@<mate> <packet class> <hex body>   a scenario trace: a server packet sent to the mate's client (its enter
                                burst, which the Go runner drops, is not recorded)
```

An untagged `C`, `S`, `T` or `Q` line is the player's (the main client), so a trace with no mate that
has a client reads as it always did. A `Q` line is the statement of whoever sent the packet that ran
it; a step's `S` lines are the player's and its `S@<mate>` lines the mate's, compared with the
scenario's S lines (the client that played the step) and `to` lines (the others).

The player and NPCs stand within interaction range of each other, so no movement is
needed. Each quest uses its own player, created through the character-creation call
and given the needed level. A bypass waits 150 ms of real time first, because the
reference throttles bypasses to one per 100 ms.

Talking to an NPC is: select it (unless it is already the target), interact, then click
the `Quest` link of its chat window. An NPC tied to more than one quest answers that link
with the quest chooser (Roxxy: Q001 and Q006); the probe then clicks the traced quest's
chooser entry (`npc_<object>_Quest <quest name>`), as a player would. The reference only
accepts a bypass the last window offered, so the probe cannot skip the chooser.

After its last step each quest must be in its completed state; otherwise the probe throws
and `run.sh` exits non-zero without writing any golden, so a stuck flow fails gate 2
instead of being recorded.

## Fallbacks

The plan names a fallback for each gate:

- gate 1: a static scan for the behavior union plus a hand-audited manifest, reviewed
  blind, for the 31 sagas and about 27 collection- or loop-driven registrations;
- gate 2: the reviewer re-derives expectations blind.

Neither is in force. If a later reference or datapack change makes `run.sh` fail a gate,
the matching fallback applies until the probe is repaired.
