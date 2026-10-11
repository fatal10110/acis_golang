package script

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// GlobalMemo is the server-wide script memo: one value per key, shared by
// every script of the registry. It lives in memory only, so every boot
// starts it empty. It is leaf-locked: no method calls out while holding
// its lock.
type GlobalMemo struct {
	mu     sync.Mutex
	values map[string]string
	// world is where Creature looks the stored object ids up; nil finds
	// none.
	world *world.State
}

// newGlobalMemo returns an empty memo resolving creatures in w.
func newGlobalMemo(w *world.State) *GlobalMemo {
	return &GlobalMemo{values: map[string]string{}, world: w}
}

// Int returns the value of key read as an int; def when key has none. A
// value that does not read as a 32-bit int panics, so the hook aborts
// there.
func (m *GlobalMemo) Int(key string, def int) int {
	m.mu.Lock()
	v, ok := m.values[key]
	m.mu.Unlock()
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		panic(fmt.Sprintf("script: global memo %q holds %q, not an int", key, v))
	}
	return int(n)
}

// Set stores v under key.
func (m *GlobalMemo) Set(key string, v int) {
	s := strconv.Itoa(v)
	m.mu.Lock()
	m.values[key] = s
	m.mu.Unlock()
}

// Remove drops key.
func (m *GlobalMemo) Remove(key string) {
	m.mu.Lock()
	delete(m.values, key)
	m.mu.Unlock()
}

// Creature returns the creature whose object id key holds: nil when key
// holds none or 0, when no creature of that id is in the world, and when
// it is an NPC that has decayed.
func (m *GlobalMemo) Creature(key string) Creature {
	id := m.Int(key, 0)
	if id == 0 || m.world == nil {
		return nil
	}
	o, ok := m.world.Object(int32(id))
	if !ok {
		return nil
	}
	c, ok := o.(attackable.Combatant)
	if !ok {
		return nil
	}
	if n := NPCOf(c); n != nil && n.Decayed() {
		return nil
	}
	return creatureOf(c)
}

// GlobalMemo returns the server-wide memo every script shares.
func (s *Script) GlobalMemo() *GlobalMemo { return s.registry.memo }
