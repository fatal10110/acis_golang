package script

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// newNPCHandle returns a handle on a fresh civilian NPC of template id.
func newNPCHandle(t *testing.T, objectID int32, id int) *NPC {
	t.Helper()
	inst, err := npc.NewInstance(objectID, &npc.Template{ID: id, TemplateID: id, Type: "Merchant", Name: "Ghost", Level: 1, HPMax: 10})
	if err != nil {
		t.Fatal(err)
	}
	f, err := npc.NewFolk(inst)
	if err != nil {
		t.Fatal(err)
	}
	return NPCOf(f)
}

// An NPC cell holds one NPC by identity: set-if-empty keeps the first, and
// clear-if-same leaves it when another NPC of the same template asks, even
// through a fresh handle on the held one.
func TestNPCCellIdentity(t *testing.T) {
	var c NPCCell
	if !c.Empty() || c.Get() != nil {
		t.Fatal("zero cell is not empty")
	}
	first := newNPCHandle(t, 100, 31524)
	other := newNPCHandle(t, 101, 31524)

	if c.SetIfEmpty(nil) || c.SetIfEmpty(NPCOf(nil)) || !c.Empty() {
		t.Fatal("set-if-empty stored a handle on nothing")
	}
	if !c.SetIfEmpty(first) {
		t.Fatal("set-if-empty on an empty cell refused")
	}
	if c.SetIfEmpty(other) {
		t.Fatal("set-if-empty replaced the held NPC")
	}
	if got := c.Get(); got == nil || got.ObjectID() != 100 {
		t.Fatalf("Get = %v, want the first NPC", got)
	}
	if c.ClearIf(other) || c.ClearIf(nil) || c.Empty() {
		t.Fatal("clear-if-same cleared for another NPC")
	}
	if !c.ClearIf(NPCOf(first.self)) || !c.Empty() {
		t.Fatal("clear-if-same kept the held NPC")
	}
	c.Set(other)
	if got := c.Get(); got == nil || got.ObjectID() != 101 {
		t.Fatal("Set did not store")
	}
	c.Clear()
	if !c.Empty() {
		t.Fatal("Clear left the NPC")
	}
}

// Concurrent set-if-empty and clear-if-same on one cell never lose the
// identity rule: exactly one setter wins each round, and only its NPC
// clears it.
func TestNPCCellConcurrentSetClear(t *testing.T) {
	var c NPCCell
	npcs := make([]*NPC, 8)
	for i := range npcs {
		npcs[i] = newNPCHandle(t, int32(200+i), 31524)
	}
	for range 200 {
		var wg sync.WaitGroup
		wins := make([]bool, len(npcs))
		for i, n := range npcs {
			wg.Go(func() { wins[i] = c.SetIfEmpty(n) })
		}
		wg.Wait()
		winner := -1
		for i, won := range wins {
			if won {
				if winner >= 0 {
					t.Fatalf("two setters won: %d and %d", winner, i)
				}
				winner = i
			}
		}
		if winner < 0 {
			t.Fatal("no setter won")
		}
		cleared := make([]bool, len(npcs))
		for i, n := range npcs {
			wg.Go(func() { cleared[i] = c.ClearIf(n) })
		}
		wg.Wait()
		for i, ok := range cleared {
			if ok != (i == winner) {
				t.Fatalf("ClearIf by %d = %v, winner %d", i, ok, winner)
			}
		}
		if !c.Empty() {
			t.Fatal("cell not empty after the winner cleared it")
		}
	}
}

func TestIntAndBoolCells(t *testing.T) {
	var n IntCell
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { n.Add(2) })
	}
	wg.Wait()
	if n.Get() != 100 {
		t.Fatalf("after 50 concurrent adds of 2 = %d, want 100", n.Get())
	}
	n.Set(-3)
	if n.Get() != -3 {
		t.Fatal("int set")
	}

	var b BoolCell
	if b.Get() {
		t.Fatal("zero bool cell is set")
	}
	b.Set(true)
	if !b.Get() {
		t.Fatal("bool set")
	}
	b.Set(false)
	if b.Get() {
		t.Fatal("bool set")
	}
}

func TestMapCell(t *testing.T) {
	var m MapCell[int32, string]
	if _, ok := m.Get(1); ok {
		t.Fatal("zero map cell is not empty")
	}
	if _, ok := m.Delete(1); ok {
		t.Fatal("delete from an empty cell reported a value")
	}
	m.Set(1, "a")
	m.Set(2, "c")
	m.Set(1, "b")
	if v, ok := m.Get(1); !ok || v != "b" {
		t.Fatalf("Get(1) = %q %v, want the later value", v, ok)
	}
	if v, ok := m.Delete(1); !ok || v != "b" {
		t.Fatalf("delete = %q %v", v, ok)
	}
	if _, ok := m.Get(1); ok {
		t.Fatal("deleted key still present")
	}
	if v, ok := m.Get(2); !ok || v != "c" {
		t.Fatalf("Get(2) = %q %v", v, ok)
	}

	var wg sync.WaitGroup
	var shared MapCell[int, int]
	for i := range 64 {
		wg.Go(func() {
			shared.Set(i%8, i)
			shared.Get(i % 8)
			shared.Delete(i % 3)
		})
	}
	wg.Wait()
}
