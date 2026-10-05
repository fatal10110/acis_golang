# Script engine reference probe

This directory holds the reference probe for the script engine (slice V0 of
`docs/script-engine-plan.md`, section 11 item 1) and what it produced:

- `manifest.golden`: the script registration manifest for every script the datapack's
  `data/xml/scripts.xml` lists (gate 1).
- `trace_q001.golden`, `trace_q003.golden`: two quests played through client packets, with
  every server packet and SQL statement they caused (gate 2).

Gate status at the committed revision: **gate 1 passes** (857 of 857 listed scripts built
and recorded, no fallback in force); **gate 2 passes** (two consecutive runs produce
byte-identical traces, no fallback in force).

## What the probe is

The whole reference tree (`aCis_gameserver/java`, 2,343 files) is compiled with `javac` in
the `eclipse-temurin:21-jdk-alpine` image, with four infrastructure classes replaced:

| Reference class | Probe version |
|---|---|
| `commons/pool/ThreadPool.java` | `shadow/.../ThreadPool.java`: no threads. `execute` runs inline; scheduled tasks wait on a virtual clock that only the probe advances, then run inline in due-time order (ties in scheduling order). |
| `commons/random/Rnd.java` | `shadow/.../Rnd.java`: one seeded `java.util.Random` with the reference bounds; the trace reseeds it before each quest. |
| `gameserver/network/GameClient.java` | the reference file, with the socket write of `sendPacket` replaced at build time by `ProbeWire.capture` (`probe/net/sf/l2j/commons/mmocore/ProbeWire.java`), which writes the packet into a little-endian buffer exactly as the selector does before encryption. The `runImpl` after the send is kept. |
| `commons/pool/ConnectionPool.java` | `shadow/.../ConnectionPool.java`: no database. Updates touch nothing; a `SELECT` from a table the datapack's install scripts (`aCis_datapack/sql/*.sql`) seed returns those rows, as a fresh install would (castle, clan hall, seven signs status and festival, derby bets); any other query returns no row. |

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

## Regenerating

From the repository root, with the reference server and datapack checkouts:

```bash
ACIS_JAVA=../aCis_gameserver ACIS_DATAPACK=../aCis_datapack \
  internal/gameserver/script/testdata/oracle/run.sh
```

Both default to the siblings of the repository root, so from the primary checkout in the
workspace no variable is needed. The script builds the probe, runs it and copies the three
`.golden` files here; it needs Docker and nothing else. `PROBE_LOG=<file>` keeps the
reference server's log. Build and run together take under half a minute.

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
```

The player and NPCs stand within interaction range of each other, so no movement is
needed. Each quest uses its own player, created through the character-creation call
and given the needed level. A bypass waits 150 ms of real time first, because the
reference throttles bypasses to one per 100 ms.

## Fallbacks

The plan names a fallback for each gate:

- gate 1: a static scan for the behavior union plus a hand-audited manifest, reviewed
  blind, for the 31 sagas and about 27 collection- or loop-driven registrations;
- gate 2: the reviewer re-derives expectations blind.

Neither is in force. If a later reference or datapack change makes `run.sh` fail a gate,
the matching fallback applies until the probe is repaired.
