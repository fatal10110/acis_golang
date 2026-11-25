package npc

import (
	"sync"
	"testing"
)

// TestMarkSpoilRefusesCorpse covers #5086: the spoil mark is taken under the
// death latch, so a dead monster takes no mark.
func TestMarkSpoilRefusesCorpse(t *testing.T) {
	h := partyHostile(t, 1, 0, &hostileMove{})
	spawnPartyWorld(t, h)
	if alive, marked := h.MarkSpoil(7); !alive || !marked || h.SpoilerID() != 7 {
		t.Fatalf("MarkSpoil(7) on a live monster = %t/%t, spoiler %d; want true/true, 7", alive, marked, h.SpoilerID())
	}
	if alive, marked := h.MarkSpoil(8); !alive || marked || h.SpoilerID() != 7 {
		t.Fatalf("MarkSpoil(8) on a spoiled monster = %t/%t, spoiler %d; want true/false, 7", alive, marked, h.SpoilerID())
	}

	corpse := partyHostile(t, 1, 0, &hostileMove{})
	spawnPartyWorld(t, corpse)
	if !corpse.MarkDead() {
		t.Fatal("MarkDead() = false, want a fresh death")
	}
	if alive, marked := corpse.MarkSpoil(7); alive || marked || corpse.Spoiled() {
		t.Fatalf("MarkSpoil(7) on a corpse = %t/%t, spoiled %t; want false/false/false", alive, marked, corpse.Spoiled())
	}
}

// TestMarkSpoilRacingDeathNeverMarksCorpse races the mark against the death
// latch: whichever wins, a mark exists only if it was taken while alive.
func TestMarkSpoilRacingDeathNeverMarksCorpse(t *testing.T) {
	for range 200 {
		h := partyHostile(t, 1, 0, &hostileMove{})
		var alive, marked bool
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); alive, marked = h.MarkSpoil(7) }()
		go func() { defer wg.Done(); h.MarkDead() }()
		wg.Wait()
		if !h.Dead() || alive != marked || h.Spoiled() != marked {
			t.Fatalf("dead %t, MarkSpoil = %t/%t, spoiled %t; want a mark only when taken alive", h.Dead(), alive, marked, h.Spoiled())
		}
	}
}
