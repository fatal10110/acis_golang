package script

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// NPCMapCell maps NPCs to NPCs, empty at first: a map keyed by an NPC
// (the third-class sagas' busy tablets and their fights).
// Keys and values match by NPC identity, the one the timers use: the spawn
// slot's, shared by every life the slot spawns, since a respawn is the
// same NPC in a new life; an NPC with no slot is its own identity.
// Neither the template nor the object id is the identity: another NPC of
// the same template is another key. Like the other cells it is
// leaf-locked, takes no closure, and its zero value is ready to use.
type NPCMapCell struct {
	mu sync.Mutex
	// entries are the pairs held, in the order their keys were first set.
	entries []npcPair
}

// npcPair is one entry: the key's identity, the value's identity, and the
// value as last handed to Set, which Get hands back.
type npcPair struct {
	key, valueID any
	value        attackable.Combatant
}

// index returns the position of the entry keyed by identity key, -1 when
// key is not held. The caller holds mu.
func (c *NPCMapCell) index(key any) int {
	for i, e := range c.entries {
		if e.key == key {
			return i
		}
	}
	return -1
}

// Get returns the NPC key maps to, as last handed to Set; nil when key is
// not held, or is nil or a handle on nothing.
func (c *NPCMapCell) Get(key *NPC) *NPC {
	k := npcIdentity(key)
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
	k := npcIdentity(key)
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
	k, vid, v := npcIdentity(key), npcIdentity(value), combatantOf(value)
	if k == nil || vid == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if i := c.index(k); i >= 0 {
		c.entries[i].valueID, c.entries[i].value = vid, v
		return
	}
	c.entries = append(c.entries, npcPair{key: k, valueID: vid, value: v})
}

// Delete removes key and reports whether it was held.
func (c *NPCMapCell) Delete(key *NPC) bool {
	k := npcIdentity(key)
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
// set, that maps to value, and reports whether there was one: a removal
// of a value from the map's values. Other entries
// mapping to value stay.
func (c *NPCMapCell) DeleteValue(value *NPC) bool {
	v := npcIdentity(value)
	if v == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, e := range c.entries {
		if e.valueID == v {
			c.entries = append(c.entries[:i], c.entries[i+1:]...)
			return true
		}
	}
	return false
}
