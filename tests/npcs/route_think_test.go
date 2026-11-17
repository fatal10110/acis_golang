package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
)

// TestThinkPassLeavesAMonsterOnItsRouteAlone pins the continue pass (a bow
// reuse or a control effect ending asks for it) on a monster whose current
// intention is its route: the base think dispatch has no route case, so the
// pass sends no walk, shows nothing, and the monster goes on to its first
// node, its intention unchanged, both mid-walk and with its walk stopped.
func TestThinkPassLeavesAMonsterOnItsRouteAlone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		stopped bool
	}{
		{name: "mid-walk"},
		{name: "walk stopped", stopped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, h := bootPatrol(t, 10)
			onQueueOf(t, h, func() {
				if tc.stopped {
					h.StopMove()
				}
			})
			drainFrames(t, w.c)

			var err error
			onQueueOf(t, h, func() { err = h.Think() })
			if err != nil {
				t.Fatalf("Think() = %v", err)
			}
			w.srv.Settle(t)
			if shown := aboutNPC(drainFrames(t, w.c), h.ObjectID()); len(shown) != 0 {
				t.Fatalf("the continue pass showed the monster %d frames (first %#x), want none", len(shown), shown[0].opcode)
			}
			if got := h.AI().CurrentIntention(); got != ai.IntentionMoveRoute {
				t.Fatalf("intention after the continue pass = %v, want move_route", got)
			}
			if tc.stopped {
				return
			}
			w.srv.AdvanceUntil(t, "patrol reaches its first node", func() bool {
				x, _, _ := h.Position()
				return x <= w.a[patrolID].X
			})
		})
	}
}
