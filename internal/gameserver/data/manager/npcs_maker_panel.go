package manager

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// defaultMakerScript is the maker script an npcmaker whose maker type has
// no script of its own runs.
const defaultMakerScript = "default_maker"

// MakerState is one npcmaker of the spawn list in use as the admin maker
// panel shows it.
type MakerState struct {
	Maker *spawn.Maker
	// Territory and Banned are the territories the maker spawns in and
	// avoids, each merged when it names several; nil when it names none.
	Territory, Banned *spawn.Territory
	// Script names the maker script the npcmaker runs.
	Script string
	// Alive counts its NPCs in the world; Decayed its slots whose NPC is
	// gone, waiting for a respawn or never coming back.
	Alive, Decayed int
	Spawns         []MakerSpawnState
}

// MakerSpawnState is one spawn entry of a MakerState.
type MakerSpawnState struct {
	Entry spawn.Entry
	// Name is the entry NPC's template name, empty when it has none.
	Name           string
	Spawned, Total int
}

// registeredMakers is a MakerBehaviors that tells the maker types with a
// script of their own.
type registeredMakers interface {
	Registered(aiType string) bool
}

// MakersAt returns the state of every npcmaker whose territory holds loc,
// in list order.
func (n *Npcs) MakersAt(loc location.Location) []MakerState {
	var out []MakerState
	for _, g := range n.currentGroups() {
		if g.def.Contains(loc) {
			out = append(out, n.makerState(g))
		}
	}
	return out
}

// Maker returns the state of the first npcmaker named name, ignoring case.
func (n *Npcs) Maker(name string) (MakerState, bool) {
	g, ok := n.groupNamed(name)
	if !ok {
		return MakerState{}, false
	}
	return n.makerState(g), true
}

// DeleteMakerNPCs deletes every NPC of the first npcmaker named name,
// ignoring case, and reports whether there is one.
func (n *Npcs) DeleteMakerNPCs(name string) bool {
	g, ok := n.groupNamed(name)
	if ok {
		g.DeleteAll()
	}
	return ok
}

// RespawnMakerNPCs respawns at once, per spawn entry of the first
// npcmaker named name (ignoring case), the first of its NPCs that is gone,
// and reports whether there is one.
func (n *Npcs) RespawnMakerNPCs(name string) bool {
	g, ok := n.groupNamed(name)
	if !ok {
		return false
	}
	for _, s := range g.Spawns() {
		s.Respawn()
	}
	return true
}

// SpawnEventNPCs starts every npcmaker whose event is exactly event, in
// list order, and returns how many NPCs those makers then have.
func (n *Npcs) SpawnEventNPCs(event string) int {
	if event == "" {
		return 0
	}
	total := 0
	for _, g := range n.currentGroups() {
		if g.def.EventName() == event {
			g.behavior.Start(g.in(&hookScope{}))
			total += g.Alive()
		}
	}
	n.log.Info().Str("event", event).Int("npcs", total).Msg("spawned event npcs")
	return total
}

// DespawnEventNPCs deletes the NPCs of every npcmaker whose event is
// exactly event and returns how many it deleted.
func (n *Npcs) DespawnEventNPCs(event string) int {
	if event == "" {
		return 0
	}
	total := 0
	for _, g := range n.currentGroups() {
		if g.def.EventName() == event {
			total += g.Alive()
			g.DeleteAll()
		}
	}
	n.log.Info().Str("event", event).Int("npcs", total).Msg("despawned event npcs")
	return total
}

// Territory returns the spawn territory named name, ignoring case; a
// merged territory answers to its grouped_NNN name.
func (n *Npcs) Territory(name string) (*spawn.Territory, bool) {
	return n.Spawns().Table().Territory(name)
}

// TerritoryLocation draws a point in t on the population's geodata (see
// spawn.Territory.RandomLocation).
func (n *Npcs) TerritoryLocation(t *spawn.Territory) (location.Location, bool) {
	return t.RandomLocation(n.geo)
}

func (n *Npcs) groupNamed(name string) (*makerGroup, bool) {
	for _, g := range n.currentGroups() {
		if strings.EqualFold(g.def.Name, name) {
			return g, true
		}
	}
	return nil, false
}

func (n *Npcs) makerState(g *makerGroup) MakerState {
	table := n.Spawns().Table()
	st := MakerState{
		Maker:     g.def,
		Territory: table.MakerTerritory(g.def),
		Banned:    table.MakerBannedTerritory(g.def),
		Script:    g.def.AIType,
		Alive:     g.Alive(),
	}
	if r, ok := n.makers.(registeredMakers); st.Script == "" || ok && !r.Registered(st.Script) {
		st.Script = defaultMakerScript
	}
	for _, s := range g.Spawns() {
		spawned, decayed := s.(groupSpawn).counts()
		st.Decayed += decayed
		row := MakerSpawnState{Entry: s.Entry(), Spawned: spawned, Total: s.Total()}
		if tmpl, ok := n.templates.Get(int(row.Entry.NPCID)); ok {
			row.Name = tmpl.Name
		}
		st.Spawns = append(st.Spawns, row)
	}
	return st
}
