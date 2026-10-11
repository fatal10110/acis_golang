package script

import (
	"strconv"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// The memo stores ints by key: a missing key gives the default, a set key
// its value, a removed key the default again.
func TestGlobalMemoSetGetRemove(t *testing.T) {
	m := newGlobalMemo(nil)
	if got := m.Int("5", -1); got != -1 {
		t.Fatalf("Int of a missing key = %d, want the default -1", got)
	}
	m.Set("5", 268435457)
	if got := m.Int("5", -1); got != 268435457 {
		t.Fatalf("Int after Set = %d, want 268435457", got)
	}
	m.Set("5", 0)
	if got := m.Int("5", -1); got != 0 {
		t.Fatalf("Int after a second Set = %d, want 0", got)
	}
	m.Remove("5")
	m.Remove("never set")
	if got := m.Int("5", -1); got != -1 {
		t.Fatalf("Int after Remove = %d, want the default -1", got)
	}
}

// A value that does not read as a 32-bit int aborts the hook reading it.
func TestGlobalMemoIntPanicsOnANonInt(t *testing.T) {
	m := newGlobalMemo(nil)
	m.mu.Lock()
	m.values["big"] = "2147483648"
	m.mu.Unlock()
	defer func() {
		if recover() == nil {
			t.Fatal("Int of a value past the 32-bit range did not panic")
		}
	}()
	m.Int("big", 0)
}

// The creature lookup resolves the stored object id to the creature in the
// world, and finds nothing for a key holding no id, for an id no object
// has, and for an NPC that has decayed, even before it leaves the world.
func TestGlobalMemoCreature(t *testing.T) {
	w := world.New()
	live := newNPCHandle(t, 501, 29019)
	gone := newNPCHandle(t, 502, 29019)
	for _, n := range []*NPC{live, gone} {
		w.Spawn(n.self.(world.Tracked), 0, 0, 0, 0)
	}
	if !gone.self.(interface {
		Decay(*world.State, func()) bool
	}).Decay(nil, nil) {
		t.Fatal("the NPC did not decay")
	}
	if _, ok := w.Object(502); !ok {
		t.Fatal("the decayed NPC left the world; the check needs it still there")
	}

	m := newGlobalMemo(w)
	m.Set("live", 501)
	m.Set("gone", 502)
	m.Set("missing", 503)
	m.Set("zero", 0)
	if c := m.Creature("live"); c == nil || c.ObjectID() != 501 {
		t.Fatalf("Creature(live) = %v, want NPC 501", c)
	}
	for _, key := range []string{"gone", "missing", "zero", "unset"} {
		if c := m.Creature(key); c != nil {
			t.Errorf("Creature(%s) = %v, want nothing", key, c)
		}
	}
	if c := newGlobalMemo(nil).Creature("live"); c != nil {
		t.Errorf("Creature with no world = %v, want nothing", c)
	}
}

// Every script of a registry shares one memo; another registry, as a
// restart builds, starts with an empty one.
func TestGlobalMemoSharedByTheRegistry(t *testing.T) {
	build := func() *Registry {
		catalog := Catalog{"ai.A": func() Script { return Script{} }, "ai.B": func() Script { return Script{} }}
		return Build([]Listing{{Path: "ai.A"}, {Path: "ai.B"}}, catalog, Config{Log: zerolog.Nop(), Env: &Env{}})
	}
	r := build()
	a, b := r.byName["a"], r.byName["b"]
	if a == nil || b == nil {
		t.Fatal("scripts not registered")
	}
	a.GlobalMemo().Set("7", 42)
	if got := b.GlobalMemo().Int("7", -1); got != 42 {
		t.Fatalf("other script reads %d, want the shared 42", got)
	}
	if got := build().byName["a"].GlobalMemo().Int("7", -1); got != -1 {
		t.Fatalf("a new registry reads %d, want an empty memo", got)
	}
}

// Concurrent writers and readers of one memo, run under -race.
func TestGlobalMemoConcurrentUse(t *testing.T) {
	m := newGlobalMemo(world.New())
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			key := strconv.Itoa(i % 3)
			for v := range 200 {
				m.Set(key, v)
				_ = m.Int(key, -1)
				_ = m.Creature(key)
				if v%7 == 0 {
					m.Remove(key)
				}
			}
		})
	}
	wg.Wait()
}
