package quest

import (
	"maps"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestScriptTakeHeldItemsTakesOnlyWhatIsHeld: TakeHeldItems takes as
// TakeItems does when the player holds the count, with the same chat line,
// and reports how many units it took. Units moved away between a script's
// count and the take are not taken and reported as none, short of the count
// nothing goes and nothing is said, so a script pays or admits nothing for
// them. Quests 343, 350, 620 and 635 take through it: an owner-approved
// divergence (#5164, #5165, #5166) from a take that reports nothing.
func TestScriptTakeHeldItemsTakesOnlyWhatIsHeld(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			takeHeldItems(t, tc.opts...)
		})
	}
}

func takeHeldItems(t *testing.T, opts ...gameservertest.Option) {
	srv, objID := bootHelpers(t, opts...)
	c := onlineCharacter(t, srv, objID)
	step := func(name string, fn func(sc *script.Script, p *script.Player) int32, want int32, lines ...string) {
		t.Helper()
		var got int32
		run(t, srv, objID, func(sc *script.Script, p *script.Player) { got = fn(sc, p) })
		if got != want {
			t.Fatalf("%s took %d, want %d", name, got, want)
		}
		if got := immediateLines(t, srv); !slices.Equal(got, lines) {
			t.Fatalf("%s packets:\n got %q\nwant %q", name, got, lines)
		}
	}
	give := func(id, n int32) {
		t.Helper()
		run(t, srv, objID, func(sc *script.Script, p *script.Player) { sc.GiveItems(p, id, n) })
		immediateLines(t, srv)
	}
	count := func(id int32) int { return srv.PlayerItemCount(t, objID, id) }

	give(dropItem, 10)
	step("a stack moved away after the count", func(sc *script.Script, p *script.Player) int32 {
		if n := p.ItemCount(dropItem); n != 10 {
			t.Errorf("counted %d, want 10", n)
		}
		c.ConsumeItem(dropItem, 10)
		return sc.TakeHeldItems(p, dropItem, -1)
	}, 0)
	give(dropItem, 10)
	step("part of a stack moved away after the count", func(sc *script.Script, p *script.Player) int32 {
		c.ConsumeItem(dropItem, 4)
		return sc.TakeHeldItems(p, dropItem, 10)
	}, 0)
	if n := count(dropItem); n != 6 {
		t.Fatalf("held %d after a short take, want the 6 left", n)
	}
	step("all of what is left", func(sc *script.Script, p *script.Player) int32 {
		return sc.TakeHeldItems(p, dropItem, -1)
	}, 6, "S SystemMessage id=301 3:1081 6:6")
	give(dropItem, 10)
	step("part of a stack", func(sc *script.Script, p *script.Player) int32 {
		return sc.TakeHeldItems(p, dropItem, 3)
	}, 3, "S SystemMessage id=301 3:1081 6:3")
	step("none", func(sc *script.Script, p *script.Player) int32 {
		return sc.TakeHeldItems(p, dropItem, 0) + sc.TakeHeldItems(p, 999999, -1)
	}, 0)
	give(swordID, 2)
	step("more instances than held", func(sc *script.Script, p *script.Player) int32 {
		return sc.TakeHeldItems(p, swordID, 3)
	}, 0)
	step("one instance", func(sc *script.Script, p *script.Player) int32 {
		return sc.TakeHeldItems(p, swordID, 1)
	}, 1, "S SystemMessage id=302 3:30")
	if count(dropItem) != 7 || count(swordID) != 1 {
		t.Fatalf("held %d of %d and %d of %d, want 7 and 1", count(dropItem), dropItem, count(swordID), swordID)
	}

	srv.FlushItems(t)
	if got := itemRows(t, srv, objID); !maps.Equal(got, map[int32]int{dropItem: 7, swordID: 1}) {
		t.Fatalf("item rows = %v", got)
	}
}
