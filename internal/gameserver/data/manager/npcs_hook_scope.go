package manager

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// hookScope is one run of a maker hook: the npcmaker slots that its
// DeleteAll and DeleteNPCs calls dropped, by serial. A respawn the hook
// arms for an NPC it deleted itself brings that NPC back; a DeleteAll
// that another caller made while the hook ran leaves the NPC gone. The
// hook's group, spawns and NPC handles carry it, and so do the timers the
// hook arms with them. Guarded by n.mu.
type hookScope struct {
	dropped map[task.SlotID]struct{}
}

// recordLocked records that the slot serial was dropped in s; nothing for
// a nil s. The caller holds n.mu.
func (s *hookScope) recordLocked(serial task.SlotID) {
	if s == nil || serial == 0 {
		return
	}
	if s.dropped == nil {
		s.dropped = make(map[task.SlotID]struct{})
	}
	s.dropped[serial] = struct{}{}
}

// droppedLocked reports whether the slot serial was dropped in s. The
// caller holds n.mu.
func (s *hookScope) droppedLocked(serial task.SlotID) bool {
	if s == nil {
		return false
	}
	_, ok := s.dropped[serial]
	return ok
}

// hookGroup is an npcmaker as a hook running in scope sees it: what the
// hook deletes through it, or through the spawns and NPCs it hands out, is
// recorded on scope.
type hookGroup struct {
	*makerGroup
	scope *hookScope
}

// in returns g as a hook running in scope sees it; g itself for a nil
// scope.
func (g *makerGroup) in(scope *hookScope) spawn.Group {
	if scope == nil {
		return g
	}
	return hookGroup{makerGroup: g, scope: scope}
}

func (h hookGroup) Spawns() []spawn.GroupSpawn { return h.spawns(h.scope) }

func (h hookGroup) DeleteAll() { h.deleteAll(h.scope) }

func (h hookGroup) FindSpawn(alias string) spawn.GroupSpawn { return h.findSpawn(alias, h.scope) }
