package manager

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// The spawn operations a maker makes on its spawns, and the spawn lookups
// and spawn-time starts a script makes on the population.

// FindSpawn returns the first spawn, in list order across every npcmaker,
// whose NPC template has alias, ignoring case; nil when none has.
func (g *makerGroup) FindSpawn(alias string) spawn.GroupSpawn { return g.findSpawn(alias, nil) }

// findSpawn is FindSpawn for a hook running in scope; a nil scope for
// none.
func (g *makerGroup) findSpawn(alias string, scope *hookScope) spawn.GroupSpawn {
	if s, ok := g.n.findGroupSpawn(func(t *npc.Template) bool { return strings.EqualFold(t.Alias, alias) }); ok {
		s.scope = scope
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
		seq   int32
	)
	for _, slot := range n.slot {
		if !slot.fixed || slot.entry.NPCID != npcID {
			continue
		}
		if seq == 0 || slot.seq < seq {
			first, seq = standaloneSpawn{at: spawn.Position{Location: slot.at, Heading: slot.heading}}, slot.seq
		}
	}
	if seq == 0 {
		return nil
	}
	return first
}

// SpawnNPC is the NPC a spawn keeps: in the world under ID, or, when Away,
// out of it: an npcmaker's NPC that decayed and waits for its spawn to
// respawn it, or one a script hid. Name, Index and Reverse are what an Away
// NPC left the world with: its name, and the walker route node it headed
// for and its direction.
type SpawnNPC struct {
	ID      int32
	Away    bool
	Name    string
	Index   int
	Reverse bool
}

// FirstNPCOfAlias returns the NPC of the first spawn whose NPC template has
// alias, ignoring case: the first npcmaker spawn in list order, else the
// standalone spawn placed first of those still kept. An npcmaker spawn
// keeps an NPC that decayed while it waits to respawn it; a spawn that
// never respawns lets it go. Any spawn keeps an NPC a script hid. Of a
// spawn keeping several NPCs it is the one of lowest object id. ok is false
// when no spawn has the alias or the first one keeps no NPC.
func (n *Npcs) FirstNPCOfAlias(alias string) (kept SpawnNPC, ok bool) {
	match := func(t *npc.Template) bool { return strings.EqualFold(t.Alias, alias) }
	first, inMaker := n.findGroupSpawn(match)
	n.mu.Lock()
	defer n.mu.Unlock()
	var (
		standalone task.SlotID
		seq        int32
	)
	if !inMaker {
		for key, slot := range n.slot {
			if !slot.fixed || (seq != 0 && slot.seq >= seq) {
				continue
			}
			if t, found := n.templates.Get(int(slot.entry.NPCID)); found && match(t) {
				standalone, seq = key, slot.seq
			}
		}
		if seq == 0 {
			return SpawnNPC{}, false
		}
	}
	for oid, key := range n.live {
		if inMaker {
			if slot := n.slot[key]; slot.group != first.g || slot.spawnIdx != first.i {
				continue
			}
		} else if key != standalone {
			continue
		}
		if ok && oid >= kept.ID {
			continue
		}
		kept, ok = SpawnNPC{ID: oid}, true
		if slot := n.slot[key]; slot.hidden {
			kept = SpawnNPC{ID: oid, Away: true, Name: slot.left.name, Index: slot.left.index, Reverse: slot.left.reverse}
		}
	}
	if !inMaker {
		return kept, ok
	}
	for key, slot := range n.slot {
		if slot.group != first.g || slot.spawnIdx != first.i || slot.orphan || slot.liveID == 0 ||
			slot.entry.RespawnDelay <= 0 || n.liveLocked(key) || (ok && slot.liveID >= kept.ID) {
			continue
		}
		kept, ok = SpawnNPC{ID: slot.liveID, Away: true, Name: slot.left.name, Index: slot.left.index, Reverse: slot.left.reverse}, true
	}
	return kept, ok
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

func (s groupSpawn) DeleteNPCs() { s.g.deleteEntry(s.i, s.scope) }

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
	key, ok := c.keyLocked()
	slot := c.n.slot[key]
	c.n.mu.Unlock()
	if !ok || slot.memory == nil {
		return
	}
	if life := slot.memory.scratch.Life(); life != nil {
		c.n.scripts.ScriptEvent(life, eventID, arg1, arg2)
	}
}
