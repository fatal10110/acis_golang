package ai

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// zakenSendFar teleports the character c to at and fails unless it stands
// there.
func (w *zakenWorld) zakenSendFar(t *testing.T, c attackable.Combatant, at location.Location) {
	t.Helper()
	if !w.srv.RunScript(t, c.ObjectID(), zakenDirector, func(_ *script.Script, p *script.Player) {
		p.TeleportTo(script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0)
	}) {
		t.Fatal("setup: the teleport panicked")
	}
	w.srv.Settle(t)
	if x, y, z := w.srv.PlayerPosition(t, c.ObjectID()); x != at.X || y != at.Y || z != at.Z {
		t.Fatalf("setup: player %d stands at %d,%d,%d, want %v", c.ObjectID(), x, y, z, at)
	}
}

// TestZakenMassTeleportLeavesAFarMark: Mate, marked, stands 65,536 west of
// where the character lands, on its height. Its ground distance is far
// over 250, so the mass teleport leaves it where it stands and draws no
// room for it: the room draws queued after the landing (5, 0, 0) stay
// queued. A 32-bit square of that gap is 0 and would pull it in (#5131).
func TestZakenMassTeleportLeavesAFarMark(t *testing.T) {
	t.Parallel()
	w := bootZaken(t, zakenBoot{})
	h := w.zaken(t)
	w.settle(t)
	mate := (&ind1World{warriorWorld: w.warriorWorld}).joinCharacter(t, "mate", "Mate")
	landing := location.Location{X: zakenCharacter.X + 100, Y: zakenCharacter.Y, Z: -3216}
	far := location.Location{X: landing.X - 65536, Y: landing.Y, Z: landing.Z}
	w.zakenSendFar(t, mate, far)
	w.onQueue(t, h, func() {
		n := script.NPCOf(h)
		n.SetSlot(npc.CreatureQuest0, script.PlayerOf(w.player))
		n.SetSlot(npc.CreatureQuest1, script.PlayerOf(mate))
		n.SetInt(npc.IntQuest0, 2)
	})
	w.draws.set(1<<30, 7, 252, 378, 5, 0, 0)
	w.zakenCast(t, h, zakenMassTeleport, w.player, w.playerAt(t, w.player, landing))
	w.srv.Settle(t)
	if x, y, z := w.srv.PlayerPosition(t, mate.ObjectID()); x != far.X || y != far.Y || z != far.Z {
		t.Fatalf("Mate after the mass teleport at %d,%d,%d, want where it stood %v", x, y, z, far)
	}
	w.draws.mu.Lock()
	left := slices.Clone(w.draws.queue)
	w.draws.mu.Unlock()
	if !slices.Equal(left, []int{5, 0, 0}) {
		t.Fatalf("draws left after the mass teleport = %v, want [5 0 0]", left)
	}
}

// TestZakenLeavesAFarFightPastAFarMark: at night Zaken fights a top desire
// target more than 1500 from its Instant Move point, and its one mark,
// Mate, stands 46,341 west of that point. Nobody is within 1500, so it
// clears its marks, picks a room (draws 2, 0, 0: room 3's corner) and wants
// Instant Move. A 32-bit square of Mate's gap wraps negative and would
// count it present (#5131).
func TestZakenLeavesAFarFightPastAFarMark(t *testing.T) {
	t.Parallel()
	room3 := location.Location{X: 54950, Y: 218790, Z: -3488}
	w := bootZaken(t, zakenBoot{night: true})
	needsDrivenClock(t, w.srv)
	h := w.zaken(t)
	w.settle(t)
	mate := (&ind1World{warriorWorld: w.warriorWorld}).joinCharacter(t, "mate", "Mate")
	w.draws.set(1<<30, 9)
	w.hit(t, h, 10)
	w.ticks(t, 1)
	if got := script.NPCOf(h).Intention(); got != script.IntentionAttack {
		t.Fatalf("setup: Zaken's intention = %v, want attack", got)
	}
	w.onQueue(t, h, func() { script.NPCOf(h).SetInt(script.SlotIntAI1, int32(zakenHome.X+1600)) })
	point := zakenDest(h)
	w.zakenSendFar(t, mate, location.Location{X: point.X - 46341, Y: point.Y, Z: point.Z})
	w.onQueue(t, h, func() {
		n := script.NPCOf(h)
		n.SetSlot(npc.CreatureQuest0, script.PlayerOf(mate))
		n.SetInt(npc.IntQuest0, 1)
	})

	w.draws.set(1<<30, 2, 0, 0)
	w.advance(t, 31*time.Second-7*1700*time.Millisecond)
	if got := zakenDest(h); got != room3 {
		t.Errorf("Instant Move point = %v, want room 3's corner %v", got, room3)
	}
	if d, ok := castDesireOf(h, zakenInstantMove); !ok || d.Weight != 10000000 {
		t.Errorf("Instant Move desire = %+v (%v), want one at 10000000", d, ok)
	}
	if got := zakenSlot(h, npc.IntQuest0); got != 0 {
		t.Errorf("marks after the move = %d, want 0", got)
	}
}
