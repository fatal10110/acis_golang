package manager

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// coreDoorID is the door whose opening releases the door_open makers named
// [kuruma_parent].
const coreDoorID = 20210001

// SpawnTimes is the world state the npcmakers with a spawn time follow.
type SpawnTimes interface {
	// CastleSiege reports whether the siege zone of castle id is active
	// and whether a clan owns the castle; ok is false for no such castle.
	CastleSiege(id int) (active, owned, ok bool)
	// HallSiegeInProgress reports whether the siege of the siegable clan
	// hall id is being fought.
	HallSiegeInProgress(id int) bool
	// DoorOpen reports whether door id stands open.
	DoorOpen(id int) bool
}

// SetSpawnTimes hands the population the world state its makers with a
// spawn time follow. Until then each of them holds.
func (n *Npcs) SetSpawnTimes(st SpawnTimes) {
	n.mu.Lock()
	n.spawnTimes = st
	n.mu.Unlock()
}

func (n *Npcs) spawnTimesState() SpawnTimes {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.spawnTimes
}

// spawnTimeHeld reports whether the spawn time of m keeps its NPCs out of
// the world. A hall's defend and attack makers hold unless the hall's siege
// is being fought; a castle's siege makers unless its siege zone is active
// and it has no owner, its owner's (pc_siege) makers unless the zone is
// active and it has one; the door_open makers named [kuruma_parent] unless
// Core's door stands open. Every other spawn time holds.
func (n *Npcs) spawnTimeHeld(kind spawn.SpawnTimeKind, params []string) bool {
	st := n.spawnTimesState()
	if st == nil || len(params) == 0 {
		return true
	}
	switch kind {
	case spawn.SpawnTimeHallDefend, spawn.SpawnTimeHallAttack:
		id, err := strconv.Atoi(params[0])
		return err != nil || !st.HallSiegeInProgress(id)
	case spawn.SpawnTimeSiege, spawn.SpawnTimePCSiege:
		id, err := strconv.Atoi(params[0])
		if err != nil {
			return true
		}
		active, owned, ok := st.CastleSiege(id)
		if !ok || !active {
			return true
		}
		return owned != (kind == spawn.SpawnTimePCSiege)
	case spawn.SpawnTimeDoorOpen:
		if strings.EqualFold(params[0], "[kuruma_parent]") {
			return !st.DoorOpen(coreDoorID)
		}
	}
	return true
}

// StartSpawnTime starts every npcmaker whose spawn time is name, ignoring
// case, and whose parameters args match, and returns how many NPCs those
// makers then have. A maker matches when each of its first three
// parameters equals, ignoring case, the argument at the same place; a
// missing argument matches none.
func (n *Npcs) StartSpawnTime(name string, args ...string) int {
	total := 0
	for _, g := range n.spawnTimeGroups(name, args) {
		g.behavior.Start(g.in(&hookScope{}))
		total += g.Alive()
	}
	n.log.Info().Str("spawn_time", name).Strs("params", args).Int("npcs", total).Msg("spawned spawn-time npcs")
	return total
}

// StopSpawnTime deletes the NPCs of every npcmaker StartSpawnTime would
// start for name and args, and returns how many it deleted.
func (n *Npcs) StopSpawnTime(name string, args ...string) int {
	total := 0
	for _, g := range n.spawnTimeGroups(name, args) {
		total += g.Alive()
		g.DeleteAll()
	}
	n.log.Info().Str("spawn_time", name).Strs("params", args).Int("npcs", total).Msg("despawned spawn-time npcs")
	return total
}

// spawnTimeGroups returns the npcmakers of the spawn list in use whose spawn
// time is name and whose parameters args match, in list order.
func (n *Npcs) spawnTimeGroups(name string, args []string) []*makerGroup {
	want := spawn.SpawnTimeKindOf(name)
	if want == spawn.SpawnTimeNone {
		return nil
	}
	var out []*makerGroup
	for _, g := range n.currentGroups() {
		if kind, params := g.def.SpawnTimeOf(); kind == want && spawnTimeParamsMatch(params, args) {
			out = append(out, g)
		}
	}
	return out
}

// spawnTimeParamsMatch reports whether args match the first three of a
// maker's spawn-time params.
func spawnTimeParamsMatch(params, args []string) bool {
	for i := range min(len(params), 3) {
		if i >= len(args) || !strings.EqualFold(args[i], params[i]) {
			return false
		}
	}
	return true
}
