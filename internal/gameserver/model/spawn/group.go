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
	// cancels their respawns. A deleted NPC's maker hook does not run.
	DeleteAll()
	// SendEvent runs the maker script event name of the npcmaker named
	// maker (case-insensitive, the first one in list order); nothing when
	// none has that name.
	SendEvent(maker, name string, int1, int2 int)
	// After runs fn on the makers' queue once d has elapsed. It is never
	// cancelled.
	After(d time.Duration, fn func())
	// Every runs fn on the makers' queue every d until the returned ticker
	// is stopped.
	Every(d time.Duration, fn func()) *sim.Ticker
}

// GroupSpawn is one spawn entry of a Group and the NPCs it placed.
type GroupSpawn interface {
	// Entry is the spawn's declaration.
	Entry() Entry
	// Total is how many NPCs the spawn keeps: one for a database-tracked
	// spawn, the entry's total otherwise.
	Total() int
	// Spawned counts the spawn's NPCs in the world; Decayed counts the
	// ones it still keeps that are not.
	Spawned() int
	Decayed() int
	// Persisted reports a database-tracked spawn.
	Persisted() bool
	// NPCs are the NPCs the spawn keeps, in the world or not.
	NPCs() []GroupNPC
	// Spawn places one more NPC of the spawn; a database-tracked spawn
	// comes back as its saved row says.
	Spawn()
	// Respawn brings back the first NPC of the spawn that is not in the
	// world, if any.
	Respawn()
	// LoadDBInfo runs the maker's npc DB info hook for this spawn.
	LoadDBInfo()
}

// GroupNPC is one NPC a GroupSpawn keeps, in the world or not.
type GroupNPC interface {
	// Decayed reports an NPC that is not in the world.
	Decayed() bool
	// Delete takes the NPC out of the world at once, with no corpse.
	Delete()
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
	for _, k := range spawnTimeKinds {
		if strings.EqualFold(k.name, parts[0]) {
			return k.kind, strings.Split(parts[1], ";")
		}
	}
	return SpawnTimeNone, nil
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
