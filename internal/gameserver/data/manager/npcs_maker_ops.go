package manager

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// The spawn operations a maker makes on its spawns, and the spawn lookups
// and spawn-time starts a script makes on the population.

// FindSpawn returns the first spawn, in list order across every npcmaker,
// whose NPC template has alias, ignoring case; nil when none has.
func (g *makerGroup) FindSpawn(alias string) spawn.GroupSpawn {
	if s, ok := g.n.findGroupSpawn(func(t *npc.Template) bool { return strings.EqualFold(t.Alias, alias) }); ok {
		return s
	}
	return nil
}

// findGroupSpawn returns the first npcmaker spawn, in list order, whose
// template match accepts.
func (n *Npcs) findGroupSpawn(match func(*npc.Template) bool) (groupSpawn, bool) {
	for _, g := range n.currentGroups() {
		for i, entry := range g.def.Entries {
			if tmpl, ok := n.templates.Get(int(entry.NPCID)); ok && match(tmpl) {
				return groupSpawn{g: g, i: i}, true
			}
		}
	}
	return groupSpawn{}, false
}

// FindSpawn returns the first spawn of NPC template npcID: the first
// npcmaker spawn in list order, else the standalone spawn placed first of
// those still kept; nil when there is none.
func (n *Npcs) FindSpawn(npcID int32) spawn.Locator {
	if s, ok := n.findGroupSpawn(func(t *npc.Template) bool { return int32(t.ID) == npcID }); ok {
		return s
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	var (
		first standaloneSpawn
		seq   int64
	)
	for key, slot := range n.slot {
		if !slot.fixed || slot.entry.NPCID != npcID {
			continue
		}
		k, err := strconv.ParseInt(strings.TrimPrefix(key, "fixed#"), 10, 64)
		if err != nil {
			continue
		}
		if seq == 0 || k < seq {
			first, seq = standaloneSpawn{at: spawn.Position{Location: slot.at, Heading: slot.heading}}, k
		}
	}
	if seq == 0 {
		return nil
	}
	return first
}

// standaloneSpawn is a standalone spawn as the point it places its NPC at.
type standaloneSpawn struct {
	at spawn.Position
}

// DrawSpawnLocation returns the spawn's point; a negative heading faces a
// random way on each call.
func (s standaloneSpawn) DrawSpawnLocation() (spawn.Position, bool) {
	at := s.at
	if at.Heading < 0 {
		at.Heading = rnd.Get(65536)
	}
	return at, true
}

func (s groupSpawn) DrawSpawnLocation() (spawn.Position, bool) {
	return s.g.n.pickSpawnPosition(s.g.def, s.Entry())
}

func (s groupSpawn) FirstNPC() spawn.GroupNPC {
	if npcs := s.NPCs(); len(npcs) > 0 {
		return npcs[0]
	}
	return nil
}

// SendScriptEvent raises the script event on each NPC the spawn keeps, as
// its last life, with no lock held.
func (s groupSpawn) SendScriptEvent(eventID, arg1, arg2 int32) {
	for _, c := range s.NPCs() {
		c.SendScriptEvent(eventID, arg1, arg2)
	}
}

func (s groupSpawn) DeleteNPCs() { s.g.deleteEntry(s.i) }

func (s groupSpawn) CheckDead() bool {
	name := s.rowName()
	_, dead := s.g.n.currentSpawns().RespawnDeadline(name, s.g.n.now())
	return dead
}

func (s groupSpawn) CheckAlive(at spawn.Position) bool {
	name := s.rowName()
	entry := s.Entry()
	tmpl, ok := s.g.n.templates.Get(int(entry.NPCID))
	if !ok {
		panic(fmt.Sprintf("manager: spawn of maker %s references unknown npc template %d", s.g.def.Name, entry.NPCID))
	}
	_, alive := s.g.n.currentSpawns().Restore(name, at.Location, at.Heading, int(tmpl.HPMax), int(tmpl.MPMax), s.g.n.now())
	return alive
}

func (s groupSpawn) SavedX() int {
	row, _ := s.g.n.currentSpawns().State(s.rowName())
	return row.Location.X
}

// rowName returns the name of the spawn's database row; a spawn that keeps
// none panics.
func (s groupSpawn) rowName() string {
	name := s.Entry().DBName
	if name == "" {
		panic(fmt.Sprintf("manager: spawn %d of maker %s keeps no database row", s.i, s.g.def.Name))
	}
	return name
}

// SendScriptEvent raises the script event on the slot's last life, live or
// not; nothing when the slot is gone or has never placed an NPC.
func (c groupNPC) SendScriptEvent(eventID, arg1, arg2 int32) {
	if c.n.scripts == nil {
		return
	}
	c.n.mu.Lock()
	slot, ok := c.n.slot[c.key]
	c.n.mu.Unlock()
	if !ok || slot.memory == nil {
		return
	}
	if life := slot.memory.scratch.Life(); life != nil {
		c.n.scripts.ScriptEvent(life, eventID, arg1, arg2)
	}
}
