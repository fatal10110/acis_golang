package script

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// NPCMapCell maps NPCs to NPCs, empty at first: the reference's map keyed
// by an NPC object (the third-class sagas' busy tablets and their fights).
// A key or value is the NPC itself, not its template or object id: another
// NPC of the same template is another key. Like the other cells it is
// leaf-locked, takes no closure, and its zero value is ready to use.
type NPCMapCell struct {
	mu sync.Mutex
	// entries are the pairs held, in the order their keys were first set.
	entries []npcPair
}

type npcPair struct{ key, value attackable.Combatant }

// index returns the position of key's entry, -1 when key is not held.
// The caller holds mu.
func (c *NPCMapCell) index(key attackable.Combatant) int {
	for i, e := range c.entries {
		if e.key == key {
			return i
		}
	}
	return -1
}

// Get returns the NPC key maps to; nil when key is not held, or is nil or a
// handle on nothing.
func (c *NPCMapCell) Get(key *NPC) *NPC {
	k := combatantOf(key)
	if k == nil {
		return nil
	}
	c.mu.Lock()
	var held attackable.Combatant
	if i := c.index(k); i >= 0 {
		held = c.entries[i].value
	}
	c.mu.Unlock()
	return NPCOf(held)
}

// Has reports whether key is held.
func (c *NPCMapCell) Has(key *NPC) bool {
	k := combatantOf(key)
	if k == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.index(k) >= 0
}

// Set maps key to value, replacing what key mapped to. A nil key or value,
// or a handle on nothing, stores nothing.
func (c *NPCMapCell) Set(key, value *NPC) {
	k, v := combatantOf(key), combatantOf(value)
	if k == nil || v == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if i := c.index(k); i >= 0 {
		c.entries[i].value = v
		return
	}
	c.entries = append(c.entries, npcPair{key: k, value: v})
}

// Delete removes key and reports whether it was held.
func (c *NPCMapCell) Delete(key *NPC) bool {
	k := combatantOf(key)
	if k == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	i := c.index(k)
	if i < 0 {
		return false
	}
	c.entries = append(c.entries[:i], c.entries[i+1:]...)
	return true
}

// DeleteValue removes the first entry, in the order the keys were first
// set, that maps to value, and reports whether there was one: the
// reference's removal of a value from the map's values. Other entries
// mapping to value stay.
func (c *NPCMapCell) DeleteValue(value *NPC) bool {
	v := combatantOf(value)
	if v == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, e := range c.entries {
		if e.value == v {
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			return true
		}
	}
	return false
}
