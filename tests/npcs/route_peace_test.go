package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// peaceTicks is enough AI ticks for the attack's hate of 15 to run out (6.6
// every third tick), with room to spare after the return to peace.
const peaceTicks = 15

// onRouteNode reports whether the patrol monster stands on one of its
// route's nodes.
func (w *routeWorld) onRouteNode(h *npc.Hostile) bool {
	x, y, _ := h.Position()
	for _, node := range []location.Location{w.a[patrolID], w.b[patrolID]} {
		if x == node.X && y == node.Y {
			return true
		}
	}
	return false
}

// routeLegsOnly fails unless every walk among frames heads for a route node.
func (w *routeWorld) routeLegsOnly(t *testing.T, frames []npcFrame) {
	t.Helper()
	dests, _ := legs(frames)
	for _, dest := range dests {
		if dest != w.a[patrolID] && dest != w.b[patrolID] {
			t.Fatalf("patrol legs = %v, want route nodes only", dests)
		}
	}
}

// Going back to peace ends the fight, not the route (NpcAI.setBackToPeace
// clears the aggro and hate lists only): a monster that left its route for
// an attack and whose hate then runs out stays on its route desire and walks
// on to a route node.
func TestRouteOutlivesTheHateRunningOut(t *testing.T) {
	t.Parallel()
	executors(t, testRouteOutlivesHate)
}

func testRouteOutlivesHate(t *testing.T, opts ...gameservertest.Option) {
	w, h := bootPatrol(t, 10, opts...)

	w.attack(t, h, 15)
	if got := h.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("intention after a heavier attack desire = %v, want attack", got)
	}
	drainFrames(t, w.c)

	var shown []npcFrame
	back := false
	for i := range peaceTicks {
		shown = append(shown, aboutNPC(w.tickAI(t), h.ObjectID())...)
		got := h.AI().CurrentIntention()
		if got == ai.IntentionMoveRoute {
			back = true
		} else if back {
			t.Fatalf("intention on AI tick %d, back on the route before = %v, want move_route", i, got)
		}
	}
	if !back {
		t.Fatalf("intention after %d AI ticks = %v, want move_route", peaceTicks, h.AI().CurrentIntention())
	}
	w.routeLegsOnly(t, shown)
	w.srv.AdvanceUntil(t, "patrol walks to a route node", func() bool { return w.onRouteNode(h) })
}

// A monster that walks its route when it goes back to peace walks on: a
// route weighing 50 outranks the attack desire of 15 throughout, and neither
// the hate running out nor a direct return to peace, as a region going
// inactive makes, stops it.
func TestRouteWalkCarriesOnThroughTheReturnToPeace(t *testing.T) {
	t.Parallel()
	w, h := bootPatrol(t, 50)

	w.attack(t, h, 15)
	var shown []npcFrame
	for i := range peaceTicks {
		shown = append(shown, aboutNPC(w.tickAI(t), h.ObjectID())...)
		if got := h.AI().CurrentIntention(); got != ai.IntentionMoveRoute {
			t.Fatalf("intention on AI tick %d = %v, want move_route", i, got)
		}
	}
	onQueueOf(t, h, func() { h.AI().SetBackToPeace() })
	if got := h.AI().CurrentIntention(); got != ai.IntentionMoveRoute {
		t.Fatalf("intention after the return to peace = %v, want move_route", got)
	}
	shown = append(shown, aboutNPC(w.tickAI(t), h.ObjectID())...)
	w.srv.AdvanceUntil(t, "patrol walks on past its first node", func() bool {
		x, _, _ := h.Position()
		return x > w.a[patrolID].X
	})
	shown = append(shown, aboutNPC(drainFrames(t, w.c), h.ObjectID())...)
	w.routeLegsOnly(t, shown)
	for _, fr := range shown {
		if fr.opcode == serverpackets.OpcodeStopMove {
			t.Fatal("patrol shown StopMove as it went back to peace, want it walking on")
		}
	}
}
