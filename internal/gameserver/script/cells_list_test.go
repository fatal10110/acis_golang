package script

import (
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// playerHandle returns a handle on c through a fresh world object of
// object id objectID, as each attach wraps a character anew.
func playerHandle(objectID int32, c *player.Character) *Player {
	return &Player{self: &characterFake{timerPlayerFake: timerPlayerFake{combatant: combatant{objectID: objectID}}, c: c}}
}

// objectIDs returns the object id of each creature, 0 for nil.
func objectIDs[T Creature](cs []T) []int32 {
	out := make([]int32, len(cs))
	for i, c := range cs {
		out[i] = combatantOf(c).ObjectID()
	}
	return out
}

func TestPlayerCell(t *testing.T) {
	var c PlayerCell
	if c.Get() != nil {
		t.Fatal("zero cell holds a player")
	}
	speller := playerHandle(10, &player.Character{})
	c.Set(speller)
	if got := c.Get(); got == nil || got.ObjectID() != 10 {
		t.Fatalf("Get = %v, want the speller", got)
	}
	c.Set(PlayerOf(nil))
	if c.Get() != nil {
		t.Fatal("a handle on nothing did not empty the cell")
	}
	c.Set(speller)
	c.Set(nil)
	if c.Get() != nil {
		t.Fatal("nil did not empty the cell")
	}
	c.Set(speller)
	c.Clear()
	if c.Get() != nil {
		t.Fatal("Clear left the player")
	}
}

// A list of creatures keeps insertion order and matches by world
// identity: a fresh handle on the same NPC, or another handle on the same
// character, is the same element; another NPC of the same template is not.
func TestListCellCreatureIdentity(t *testing.T) {
	var l ListCell[Creature]
	char := &player.Character{}
	first := playerHandle(10, char)
	again := playerHandle(11, char)
	other := playerHandle(12, &player.Character{})
	guard := newNPCHandle(t, 100, 35377)
	twin := newNPCHandle(t, 101, 35377)

	l.Add(nil, PlayerOf(nil), (*NPC)(nil), NPCOf(nil))
	if l.Len() != 0 || l.AddIfAbsent(nil) || l.AddIfAbsent(PlayerOf(nil)) {
		t.Fatal("stored a nil handle or a handle on nothing")
	}
	if !l.AddIfAbsent(first) || l.AddIfAbsent(again) {
		t.Fatal("AddIfAbsent: another handle on the same character is not the same element")
	}
	l.Add(guard)
	if l.AddIfAbsent(NPCOf(guard.self)) || !l.AddIfAbsent(twin) || !l.AddIfAbsent(other) {
		t.Fatal("AddIfAbsent: NPC identity")
	}
	if got := objectIDs(l.Items()); !slices.Equal(got, []int32{10, 100, 101, 12}) {
		t.Fatalf("Items = %v, want insertion order", got)
	}
	if l.Index(again) != 0 || l.Index(NPCOf(twin.self)) != 2 || l.Index(nil) != -1 {
		t.Fatal("Index")
	}

	// Duplicates are kept by Add, as the reference's lists keep them; Remove
	// drops the first match only, and the rest move up.
	l.Add(guard)
	if !l.Remove(NPCOf(guard.self)) || l.Remove(nil) {
		t.Fatal("Remove")
	}
	if got := objectIDs(l.Items()); !slices.Equal(got, []int32{10, 101, 12, 100}) {
		t.Fatalf("after Remove = %v", got)
	}

	if got := objectIDs(l.Take()); !slices.Equal(got, []int32{10, 101, 12, 100}) || l.Len() != 0 {
		t.Fatalf("Take = %v, left %d", got, l.Len())
	}
	if got := l.Items(); len(got) != 0 {
		t.Fatalf("Items after Take = %v", got)
	}

	var players ListCell[*Player]
	players.Add(first, other)
	if players.Index(again) != 0 || !players.Remove(again) || players.Len() != 1 {
		t.Fatal("player list identity")
	}
	players.Clear()
	if players.Len() != 0 {
		t.Fatal("Clear left elements")
	}
}

// An NPC element is its spawn slot's: every life the slot spawns is the
// same element, as the timers and the NPC map cell key it; another slot's
// NPC of the same template is not.
func TestListCellSpawnSlotLives(t *testing.T) {
	var l ListCell[*NPC]
	slot, other := npc.NewScratch(), npc.NewScratch()
	life := func(objectID int32, s *npc.Scratch) *NPC {
		return &NPC{self: &timerNPCFake{combatant: combatant{objectID: objectID}, id: 35377, scratch: s}}
	}
	first, second, elsewhere := life(100, slot), life(101, slot), life(102, other)
	l.Add(first)
	if l.AddIfAbsent(second) || l.Index(second) != 0 || !l.AddIfAbsent(elsewhere) {
		t.Fatal("NPC elements do not match by spawn slot")
	}
	if !l.Remove(second) || l.Index(first) != -1 || l.Index(elsewhere) != 0 {
		t.Fatal("removing the slot's second life left its first")
	}
}

// A list of plain values matches by ==: a row of slots filled with zeroes
// claims its first free slot and frees a slot by its value.
func TestListCellSlots(t *testing.T) {
	var row ListCell[int32]
	row.Add(0, 0, 0)
	if row.ReplaceFirst(0, 7) != 0 || row.ReplaceFirst(0, 8) != 1 || row.ReplaceFirst(0, 9) != 2 {
		t.Fatal("claims did not take the free slots in order")
	}
	if row.ReplaceFirst(0, 10) != -1 {
		t.Fatal("a full row gave a slot")
	}
	if row.Index(8) != 1 || row.Index(10) != -1 {
		t.Fatal("Index")
	}
	if row.ReplaceFirst(8, 0) != 1 || row.ReplaceFirst(0, 10) != 1 {
		t.Fatal("a freed slot was not claimed again")
	}
	if got := row.Items(); !slices.Equal(got, []int32{7, 10, 9}) {
		t.Fatalf("row = %v", got)
	}

	var npcs ListCell[*NPC]
	guard := newNPCHandle(t, 100, 35377)
	if npcs.ReplaceFirst(nil, guard) != -1 {
		t.Fatal("ReplaceFirst matched a nil handle")
	}
	npcs.Add(guard)
	if npcs.ReplaceFirst(guard, nil) != -1 || npcs.ReplaceFirst(guard, NPCOf(nil)) != -1 || npcs.Index(guard) != 0 {
		t.Fatal("ReplaceFirst stored a handle on nothing")
	}
}

// Concurrent claims never share an element: one AddIfAbsent of a player
// wins, and each free slot goes to one claimant.
func TestListCellConcurrent(t *testing.T) {
	for range 100 {
		var seen ListCell[*Player]
		char := &player.Character{}
		var wg sync.WaitGroup
		wins := make([]bool, 8)
		for i := range wins {
			wg.Go(func() { wins[i] = seen.AddIfAbsent(playerHandle(int32(i), char)) })
		}

		var row ListCell[int32]
		row.Add(0, 0, 0, 0)
		slots := make([]int, 6)
		for i := range slots {
			wg.Go(func() { slots[i] = row.ReplaceFirst(0, int32(i+1)) })
		}
		wg.Wait()

		won := 0
		for _, w := range wins {
			if w {
				won++
			}
		}
		if won != 1 || seen.Len() != 1 {
			t.Fatalf("%d AddIfAbsent wins, %d elements; want 1", won, seen.Len())
		}
		taken := map[int]bool{}
		refused := 0
		for i, s := range slots {
			switch {
			case s < 0:
				refused++
			case taken[s]:
				t.Fatalf("slot %d given twice", s)
			default:
				taken[s] = true
				if row.Index(int32(i+1)) != s {
					t.Fatalf("claimant %d holds slot %d, row %v", i+1, s, row.Items())
				}
			}
		}
		if refused != 2 || len(taken) != 4 {
			t.Fatalf("%d refused, %d slots taken; want 2 and 4", refused, len(taken))
		}
	}
}
