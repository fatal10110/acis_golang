package spawn

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// Group is one npcmaker at run time as its maker sees it: the maker's
// declaration, the NPCs its spawns placed, and what a maker does with
// them. The NPC population implements it; maker scripts receive it in
// every hook.
type Group interface {
	// Maker is the npcmaker's declaration.
	Maker() *Maker
	// Alive counts the group's NPCs in the world.
	Alive() int
	// Held reports whether the maker's spawn condition holds: while it
	// does, the maker keeps its NPCs out of the world.
	Held() bool
	// Listed reports whether event is in the SpawnEvents list.
	Listed(event string) bool
	// Spawns are the maker's spawns, one per entry, in declaration order.
	Spawns() []GroupSpawn
	// DeleteAll deletes every NPC of the group, dead ones included, and
	// cancels their respawns, one spawn after another as DeleteNPCs does.
	// A deleted NPC's maker hook does not run.
	DeleteAll()
	// FindSpawn returns the first spawn, in list order across every
	// npcmaker, whose NPC template has alias, ignoring case; nil when none
	// has.
	FindSpawn(alias string) GroupSpawn
	// SendMakerEvent runs the maker script event name of the npcmaker
	// named maker (case-insensitive, the first one in list order) and
	// reports whether one has that name; nothing runs when none has.
	SendMakerEvent(maker, name string, int1, int2 int) bool
	// After runs fn on the makers' queue once d has elapsed. It is never
	// cancelled.
	After(d time.Duration, fn func())
	// Every runs fn on the makers' queue every d until the returned ticker
	// is stopped.
	Every(d time.Duration, fn func()) *sim.Ticker
}

// Locator is a spawn as the points it places its NPC at.
type Locator interface {
	// DrawSpawnLocation returns a point the spawn would place its NPC at
	// now and the way it would face there: its one declared point, a
	// weighted pick of its points with a random heading, or a new random
	// point of its territory on every call. ok is false when the territory
	// has no point to draw.
	DrawSpawnLocation() (at Position, ok bool)
}

// GroupSpawn is one spawn entry of a Group and the NPCs it placed.
type GroupSpawn interface {
	Locator
	// Entry is the spawn's declaration.
	Entry() Entry
	// Alias is the alias of the NPC template the entry names, which every
	// NPC of the spawn has; empty when the template has none or is
	// unknown.
	Alias() string
	// Total is how many NPCs the spawn keeps: one for a database-tracked
	// spawn, the entry's total otherwise.
	Total() int
	// Spawned counts the spawn's NPCs in the world; Decayed counts the
	// ones it still keeps that are not.
	Spawned() int
	Decayed() int
	// Persisted reports a database-tracked spawn.
	Persisted() bool
	// NPCs are the NPCs the spawn keeps, in the world or not, in the order
	// the spawn created them.
	NPCs() []GroupNPC
	// FirstNPC is the first of NPCs; nil when the spawn keeps none.
	FirstNPC() GroupNPC
	// SendScriptEvent runs, before it returns, the script-event hooks of
	// every NPC the spawn keeps, out of the world ones included, as
	// GroupNPC.SendScriptEvent does.
	SendScriptEvent(eventID, arg1, arg2 int32)
	// DeleteNPCs deletes every NPC the spawn keeps, dead ones and the
	// privates scripts created for them included, and cancels their
	// respawns; the spawn then keeps none, and a database-tracked spawn's
	// row goes back to uninitialized. A deleted NPC's maker hook does not
	// run.
	DeleteNPCs()
	// CheckDead reports whether a database-tracked spawn's row is saved
	// dead with its respawn time still ahead. Any other spawn keeps no
	// row: asking panics.
	CheckDead() bool
	// CheckAlive reports whether a database-tracked spawn's row is saved
	// alive. A row saved dead whose respawn time has passed, or one never
	// initialized, instead becomes alive at at with the template's base
	// max HP and MP, and CheckAlive reports false. Any other spawn keeps
	// no row: asking panics.
	CheckAlive(at Position) bool
	// Spawn places one more NPC of the spawn; a database-tracked spawn
	// comes back as its saved row says.
	Spawn()
	// Respawn brings back the first NPC of the spawn that is not in the
	// world, if any.
	Respawn()
	// LoadDBInfo runs the maker's npc DB info hook for this spawn.
	LoadDBInfo()
	// ClearDBLoaded clears the mark a database-tracked spawn's NPC
	// carries when it was placed from a live row (npc.SpawnRow.DBLoaded);
	// nothing for any other spawn.
	ClearDBLoaded()
}

// GroupNPC is one NPC a GroupSpawn keeps, in the world or not.
type GroupNPC interface {
	// Decayed reports an NPC that is not in the world.
	Decayed() bool
	// Delete takes the NPC out of the world at once, with no corpse.
	Delete()
	// SendScriptEvent runs, before it returns, the script-event hooks of
	// the scripts bound to the NPC's template: it received eventID with
	// arg1 and arg2. An NPC out of the world receives it as its last life.
	SendScriptEvent(eventID, arg1, arg2 int32)
	// ScheduleRespawn brings the NPC back once d has elapsed; nothing for
	// a non-positive d. A respawn already pending that comes due sooner
	// stands: the earlier of the two brings the NPC back.
	ScheduleRespawn(d time.Duration)
}

// Behavior is the maker an npcmaker runs: the population raises its hooks
// with no lock held, on the goroutine that raises them.
type Behavior interface {
	// Start spawns the group as the maker decides; at boot for every
	// on-start maker and the makers of the listed events, and whenever
	// the group is started again.
	Start(g Group)
	// NPCCreated runs after one of the group's NPCs entered the world.
	NPCCreated(g Group, s GroupSpawn, npc GroupNPC)
	// NPCDeleted runs after one of the group's NPCs left the world.
	NPCDeleted(g Group, s GroupSpawn, npc GroupNPC)
	// NPCDBInfo runs when a database-tracked spawn is loaded.
	NPCDBInfo(g Group, s GroupSpawn)
	// ScriptEvent runs the maker script event name.
	ScriptEvent(g Group, name string, int1, int2 int)
}

// SpawnTimeKind is a maker spawn-time kind.
type SpawnTimeKind uint8

// The spawn-time kinds; SpawnTimeNone is a maker with no valid spawn time.
const (
	SpawnTimeNone SpawnTimeKind = iota
	SpawnTimeHallBattleRoyal
	SpawnTimeHallFinal
	SpawnTimeHallDefend
	SpawnTimeHallAttack
	SpawnTimeSiege
	SpawnTimePCSiege
	SpawnTimeDoorOpen
)

var spawnTimeKinds = [...]struct {
	name string
	kind SpawnTimeKind
}{
	{"agit_battle_royal_start", SpawnTimeHallBattleRoyal},
	{"agit_final_start", SpawnTimeHallFinal},
	{"agit_defend_warfare_start", SpawnTimeHallDefend},
	{"agit_attack_warfare_start", SpawnTimeHallAttack},
	{"siege_warfare_start", SpawnTimeSiege},
	{"pc_siege_warfare_start", SpawnTimePCSiege},
	{"door_open", SpawnTimeDoorOpen},
}

// SpawnTimeOf returns the maker's spawn-time kind and its parameters. A
// spawn time that does not split into a known kind and a parameter list,
// as "siege_warfare_start(7)" does, counts as none.
func (m *Maker) SpawnTimeOf() (SpawnTimeKind, []string) {
	if m == nil {
		return SpawnTimeNone, nil
	}
	parts := splitSpawnTime(m.SpawnTime)
	if len(parts) != 2 {
		return SpawnTimeNone, nil
	}
	kind := SpawnTimeKindOf(parts[0])
	if kind == SpawnTimeNone {
		return SpawnTimeNone, nil
	}
	return kind, strings.Split(parts[1], ";")
}

// SpawnTimeKindOf returns the spawn-time kind named name, ignoring case;
// SpawnTimeNone for an unknown name.
func SpawnTimeKindOf(name string) SpawnTimeKind {
	for _, k := range spawnTimeKinds {
		if strings.EqualFold(k.name, name) {
			return k.kind
		}
	}
	return SpawnTimeNone
}

// EventName is the maker's event: its event attribute, else the EventName
// of its maker memo, else empty.
func (m *Maker) EventName() string {
	if m == nil {
		return ""
	}
	if m.Event != "" {
		return m.Event
	}
	return m.AIParams["EventName"]
}

// OnStart reports whether the maker starts at boot: it has no event
// attribute, no spawn time, and its memo does not set on_start_spawn to 0.
func (m *Maker) OnStart() bool {
	if m == nil || m.Event != "" {
		return false
	}
	if kind, _ := m.SpawnTimeOf(); kind != SpawnTimeNone {
		return false
	}
	if v, ok := m.AIParams["on_start_spawn"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n == 0 {
			return false
		}
	}
	return true
}
