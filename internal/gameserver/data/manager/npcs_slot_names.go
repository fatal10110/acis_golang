package manager

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// SlotID returns the id of the spawn slot named name; ok is false when no
// slot has that name. Slot names are built here only, for tests and
// diagnostics: an npcmaker's slot is named maker#entry#seq, seq counting the
// entry's slots from 0, and a database-tracked one by its database name,
// either followed by @run after a RespawnAll; a database-tracked slot its
// spawn let go adds ~seq to its spawn's name. A standalone spawn is named
// fixed#seq and a private a script created private#seq, seq counting both
// from 1.
func (n *Npcs) SlotID(name string) (id task.SlotID, ok bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for id, slot := range n.slot {
		if slotName(slot) == name {
			return id, true
		}
	}
	return 0, false
}

// Slots returns the names of the live spawn slots (see SlotID) by slot id.
func (n *Npcs) Slots() map[task.SlotID]string {
	n.mu.Lock()
	defer n.mu.Unlock()
	names := make(map[task.SlotID]string, len(n.slot))
	for id, slot := range n.slot {
		names[id] = slotName(slot)
	}
	return names
}

// slotName is the name of slot (see SlotID).
func slotName(slot slotInfo) string {
	seq := strconv.Itoa(int(slot.seq))
	switch {
	case slot.fixed:
		return "fixed#" + seq
	case slot.group == nil:
		return "private#" + seq
	}
	g := slot.group
	name := g.def.Name + "#" + strconv.Itoa(slot.spawnIdx) + "#" + seq
	if slot.dbName != "" {
		name = slot.dbName
	}
	if g.gen != 0 {
		name += "@" + strconv.Itoa(g.gen)
	}
	if slot.dbName != "" && slot.key == slot.serial {
		name += "~" + seq
	}
	return name
}
