package script

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// PlayerCell holds one player, or none. Like the other cells it is
// leaf-locked, takes no closure, and its zero value is ready to use.
type PlayerCell struct {
	mu   sync.Mutex
	held attackable.Combatant
}

// Get returns the player held; nil when the cell is empty.
func (c *PlayerCell) Get() *Player {
	c.mu.Lock()
	held := c.held
	c.mu.Unlock()
	return PlayerOf(held)
}

// Set makes the cell hold p; a nil p or a handle on nothing empties it.
func (c *PlayerCell) Set(p *Player) {
	self := combatantOf(p)
	c.mu.Lock()
	c.held = self
	c.mu.Unlock()
}

// Clear empties the cell.
func (c *PlayerCell) Clear() {
	c.mu.Lock()
	c.held = nil
	c.mu.Unlock()
}

// ListCell is an ordered list, empty at first, that keeps duplicates as
// the reference's lists do. A creature handle (an *NPC, a *Player or a
// Creature) matches by world identity: an NPC's is its spawn slot's, the
// one the timers use; a player's is its character, whichever handle
// reaches it; any other creature's is the creature itself. A nil handle or
// a handle on nothing is never stored and matches nothing. Any other value
// matches by ==: a script lists clans by clan id, and keeps a fixed row of
// slots as a list it fills with zeroes at construction. Like the other
// cells it is leaf-locked, takes no closure, and its zero value is ready to
// use.
type ListCell[T comparable] struct {
	mu    sync.Mutex
	items []listItem[T]
}

// listItem is one element: its identity and the value as handed to the
// cell, which the cell hands back.
type listItem[T comparable] struct {
	key any
	v   T
}

// listKey returns the identity v matches by, and false for a nil handle or
// a handle on nothing.
func listKey[T comparable](v T) (any, bool) {
	switch x := any(v).(type) {
	case nil:
		return nil, false
	case Creature:
		id := creatureIdentity(x)
		return id, id != nil
	default:
		return x, true
	}
}

// creatureIdentity is the identity of the creature c handles: the spawn
// slot's for an NPC, the character for a player, the creature itself
// otherwise; nil for none.
func creatureIdentity(c Creature) any {
	switch h := c.(type) {
	case *NPC:
		return npcIdentity(h)
	case *Player:
		return playerIdentity(h)
	}
	return combatantOf(c)
}

// index returns the position of the first element of identity key, -1
// when there is none. The caller holds mu.
func (c *ListCell[T]) index(key any) int {
	for i, e := range c.items {
		if e.key == key {
			return i
		}
	}
	return -1
}

// Add appends vs in order, skipping each nil handle or handle on nothing.
func (c *ListCell[T]) Add(vs ...T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, v := range vs {
		if key, ok := listKey(v); ok {
			c.items = append(c.items, listItem[T]{key: key, v: v})
		}
	}
}

// AddIfAbsent appends v when no element matches it, and reports whether it
// did: the reference's contains-then-add, in one step.
func (c *ListCell[T]) AddIfAbsent(v T) bool {
	key, ok := listKey(v)
	if !ok {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index(key) >= 0 {
		return false
	}
	c.items = append(c.items, listItem[T]{key: key, v: v})
	return true
}

// Index returns the position of the first element matching v, -1 when
// none does.
func (c *ListCell[T]) Index(v T) int {
	key, ok := listKey(v)
	if !ok {
		return -1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.index(key)
}

// Remove removes the first element matching v, and reports whether there
// was one. The elements after it move up one place.
func (c *ListCell[T]) Remove(v T) bool {
	key, ok := listKey(v)
	if !ok {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	i := c.index(key)
	if i < 0 {
		return false
	}
	c.items = append(c.items[:i], c.items[i+1:]...)
	return true
}

// ReplaceFirst puts v in place of the first element matching old, and
// returns its position; -1, changing nothing, when none matches or v is a
// nil handle or a handle on nothing. A row of slots claims its first free
// slot with ReplaceFirst(0, id) and frees it with ReplaceFirst(id, 0).
func (c *ListCell[T]) ReplaceFirst(old, v T) int {
	oldKey, ok := listKey(old)
	if !ok {
		return -1
	}
	key, ok := listKey(v)
	if !ok {
		return -1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	i := c.index(oldKey)
	if i >= 0 {
		c.items[i] = listItem[T]{key: key, v: v}
	}
	return i
}

// Len returns the number of elements.
func (c *ListCell[T]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Items returns a copy of the elements, in order.
func (c *ListCell[T]) Items() []T {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values()
}

// Take returns the elements, in order, and empties the list in one step,
// for the reference's walk of a list followed by its clear: an element
// added meanwhile is neither walked nor dropped unseen.
func (c *ListCell[T]) Take() []T {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.values()
	c.items = nil
	return out
}

// Clear empties the list.
func (c *ListCell[T]) Clear() {
	c.mu.Lock()
	c.items = nil
	c.mu.Unlock()
}

// values copies the elements out. The caller holds mu.
func (c *ListCell[T]) values() []T {
	out := make([]T, len(c.items))
	for i, e := range c.items {
		out[i] = e.v
	}
	return out
}
