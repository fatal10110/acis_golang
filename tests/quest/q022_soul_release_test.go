package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestQ022SoulOfTheWellFreedWhateverKilledIt pins the owner-approved
// divergence of #5161: the soul of the well frees the well once its corpse
// is gone, whoever killed it, and a soul that died before its twenty
// seconds ran out turns no jewel red. A soul killed by a non-player, or by
// the quest player who then left range before the dying hook, used to hold
// the well for every player until a restart, and its pending twenty
// seconds still moved the hitter to cond 11.
func TestQ022SoulOfTheWellFreedWhateverKilledIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		kill func(t *testing.T, w *dialogWorld, soul *npc.Hostile)
	}{
		{"non-player killer", func(t *testing.T, w *dialogWorld, soul *npc.Hostile) {
			onPlayerQueue(t, w.srv, w.player, func() { soul.Kill(nil) })
		}},
		{"quest killer left range", func(t *testing.T, w *dialogWorld, soul *npc.Hostile) {
			x, y, z := w.srv.PlayerPosition(t, w.player)
			p := playerCombatant(t, w.srv, w.player)
			onPlayerQueue(t, w.srv, w.player, func() { soul.TakeDamage(soul.MaxHP()+1, p) })
			if !w.srv.RunScript(t, w.player, q022Name, func(_ *script.Script, p *script.Player) {
				p.InstantTeleportTo(script.Loc{X: x + 5000, Y: y, Z: z})
			}) {
				t.Fatal("the teleport away panicked")
			}
			w.srv.AdvanceUntil(t, "the killer stands 5000 away", func() bool {
				cx, _, _ := w.srv.PlayerPosition(t, w.player)
				return cx == x+5000
			})
			w.advance(t, 20*time.Second)
			if !w.srv.RunScript(t, w.player, q022Name, func(_ *script.Script, p *script.Player) {
				p.InstantTeleportTo(script.Loc{X: x, Y: y, Z: z})
			}) {
				t.Fatal("the teleport back panicked")
			}
			w.srv.AdvanceUntil(t, "the killer is back at the well", func() bool {
				cx, _, _ := w.srv.PlayerPosition(t, w.player)
				return cx == x
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, soul := soulWorld(t)
			hit(t, w.srv, w.player, soul, 10)
			tc.kill(t, w, soul)
			w.advance(t, 20*time.Second)
			if rows := questRows(t, w.srv, w.player, q022Name); rows["<cond>"] != "10" {
				t.Fatalf("rows %v after the kill and twenty seconds, want cond 10", rows)
			}
			if g, r := w.srv.PlayerItemCount(t, w.player, q022GreenJewel), w.srv.PlayerItemCount(t, w.player, q022RedJewel); g != 1 || r != 0 {
				t.Fatalf("green %d red %d after the kill, want 1 and 0", g, r)
			}
			// The suite wires no corpse-decay task for hostile NPCs, so the
			// test runs the decay the task would.
			onPlayerQueue(t, w.srv, w.player, func() { soul.Decay(w.srv.State, nil) })
			if n := len(npcsOf(w.srv, q022Soul)); n != 0 {
				t.Fatalf("%d souls in the world after the corpse decayed, want 0", n)
			}
			if got := w.talk(t, "well"); !shows(got, "31527-01.htm") {
				t.Fatalf("well: got %q, want 31527-01.htm", got)
			}
			w.send(t, encodeBypass("Quest "+q022Name+" 31527-02.htm"))
			var alive int
			for _, n := range npcsOf(w.srv, q022Soul) {
				if !n.Dead() {
					alive++
				}
			}
			if alive != 1 {
				t.Fatalf("%d live souls after the second wake, want 1", alive)
			}
		})
	}
}
