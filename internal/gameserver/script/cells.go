package script

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// The cells are the only place a script keeps state shared by every player and
// NPC it serves, never fields of the script value. Each cell is leaf-locked: no
// method calls out while holding the lock, and none takes a closure. A script
// builds its cells in its constructor and its hooks capture them; the zero value
// of each is ready to use, and a cell is never copied once used.

// NPCCell holds one NPC, or none.
type NPCCell struct {
	mu   sync.Mutex
	held attackable.Combatant
}

// Get returns the NPC held; nil when the cell is empty.
func (c *NPCCell) Get() *NPC {
	c.mu.Lock()
	held := c.held
	c.mu.Unlock()
	return NPCOf(held)
}

// Empty reports whether the cell holds no NPC.
func (c *NPCCell) Empty() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.held == nil
}

// Set makes the cell hold n; a nil n or a handle on nothing empties it.
func (c *NPCCell) Set(n *NPC) {
	self := combatantOf(n)
	c.mu.Lock()
	c.held = self
	c.mu.Unlock()
}

// SetIfEmpty makes an empty cell hold n and reports whether it did. A cell
// already holding an NPC keeps it; a nil n or a handle on nothing is never
// stored.
func (c *NPCCell) SetIfEmpty(n *NPC) bool {
	self := combatantOf(n)
	if self == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.held != nil {
		return false
	}
	c.held = self
	return true
}

// Clear empties the cell.
func (c *NPCCell) Clear() {
	c.mu.Lock()
	c.held = nil
	c.mu.Unlock()
}

// ClearIf empties the cell when it holds n itself, and reports whether it
// did. Another NPC, even one of the same template, stays held.
func (c *NPCCell) ClearIf(n *NPC) bool {
	self := combatantOf(n)
	c.mu.Lock()
	defer c.mu.Unlock()
	if self == nil || c.held != self {
		return false
	}
	c.held = nil
	return true
}

// IntCell holds an int, zero at first.
type IntCell struct {
	mu sync.Mutex
	v  int
}

// Get returns the value.
func (c *IntCell) Get() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v
}

// Set stores v.
func (c *IntCell) Set(v int) {
	c.mu.Lock()
	c.v = v
	c.mu.Unlock()
}

// Add adds delta and returns the new value.
func (c *IntCell) Add(delta int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.v += delta
	return c.v
}

// BoolCell holds a bool, false at first.
type BoolCell struct {
	mu sync.Mutex
	v  bool
}

// Get returns the value.
func (c *BoolCell) Get() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v
}

// Set stores v.
func (c *BoolCell) Set(v bool) {
	c.mu.Lock()
	c.v = v
	c.mu.Unlock()
}

// MapCell maps keys to values, empty at first. A script keying it by NPC
// or player uses the creature's object id.
type MapCell[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]V
}

// Get returns the value of k and whether k is present.
func (c *MapCell[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok
}

// Set maps k to v.
func (c *MapCell[K, V]) Set(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = make(map[K]V)
	}
	c.m[k] = v
}

// Delete removes k and returns the value it had and whether it was
// present.
func (c *MapCell[K, V]) Delete(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	delete(c.m, k)
	return v, ok
}
