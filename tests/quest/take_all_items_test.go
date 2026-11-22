package quest

import (
	"maps"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q316"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestScriptTakeAllItemsReturnsWhatItTook: TakeAllItems takes every unit
// held, with the chat line TakeItems sends for a take of all, and returns how
// many units it took. A unit that lands between a script's ItemCount and the
// take is taken and counted, so a turn-in that pays from the take pays for
// it: an owner-approved divergence (#5172) from a take that reports nothing,
// after which the reference pays from the stale count.
func TestScriptTakeAllItemsReturnsWhatItTook(t *testing.T) {
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
			takeAllItems(t, tc.opts...)
		})
	}
}

func takeAllItems(t *testing.T, opts ...gameservertest.Option) {
	srv, objID := bootHelpers(t, opts...)
	give := func(id, n int32) {
		t.Helper()
		run(t, srv, objID, func(sc *script.Script, p *script.Player) { sc.GiveItems(p, id, n) })
		immediateLines(t, srv)
	}
	count := func(id int32) int { return srv.PlayerItemCount(t, objID, id) }

	give(dropItem, 10)
	var counted, took int32
	run(t, srv, objID, func(sc *script.Script, p *script.Player) {
		counted = p.ItemCount(dropItem)
		sc.GiveItems(p, dropItem, 1) // a party member's kill lands its drop
		took = sc.TakeAllItems(p, dropItem)
	})
	if counted != 10 || took != 11 {
		t.Fatalf("counted %d and took %d, want 10 and the 11 held at the take", counted, took)
	}
	if got := immediateLines(t, srv); len(got) == 0 || got[len(got)-1] != "S SystemMessage id=301 3:1081 6:11" {
		t.Fatalf("packets of the take = %q, want them to end with the take of 11", got)
	}
	if n := count(dropItem); n != 0 {
		t.Fatalf("held %d after a take of all, want 0", n)
	}

	run(t, srv, objID, func(sc *script.Script, p *script.Player) { took = sc.TakeAllItems(p, dropItem) })
	if took != 0 {
		t.Fatalf("took %d of none held, want 0", took)
	}
	if got := immediateLines(t, srv); len(got) != 0 {
		t.Fatalf("packets of a take of none = %q, want none", got)
	}

	give(swordID, 2)
	run(t, srv, objID, func(sc *script.Script, p *script.Player) { took = sc.TakeAllItems(p, swordID) })
	if took != 2 {
		t.Fatalf("took %d instances, want 2", took)
	}
	if got, want := immediateLines(t, srv), []string{"S SystemMessage id=302 3:30", "S SystemMessage id=302 3:30"}; !slices.Equal(got, want) {
		t.Fatalf("packets of a take of instances:\n got %q\nwant %q", got, want)
	}

	srv.FlushItems(t)
	if got := itemRows(t, srv, objID); !maps.Equal(got, map[int32]int{}) {
		t.Fatalf("item rows = %v, want none", got)
	}
}

// TestQ316TurnInPaysFromTheTake: Ellenia's turn-in pays for the fangs it
// takes, the bonus past ten rat fangs included, the same as before the take
// counted (#5172); a second visit with no fang shows 30155-05 and pays
// nothing.
func TestQ316TurnInPaysFromTheTake(t *testing.T) {
	t.Parallel()
	const (
		quest      = "Q316_DestroyPlagueCarriers"
		ellenia    = 30155
		ratFang    = 1042
		varoolFang = 1043
	)
	w := bootX1Quest(t, 18, "quest."+quest, q316.New, []int32{ellenia}, nil,
		func(srv *gameservertest.Server, objID int32) {
			seedJournal(t, srv, objID, []string{quest + ":STARTED"})
			srv.GiveItem(t, objID, ratFang, 11)
			srv.GiveItem(t, objID, varoolFang, 1)
		}, x1Items([]int32{ratFang, varoolFang}))
	npcID := w.spawnFolk(t, "ellenia", ellenia, 0, 30, 0).ObjectID()
	held := func(id int32) int { return w.srv.PlayerItemCount(t, w.player, id) }

	if got, want := w.questWindow(t, npcID), w.page(t, "ellenia", quest, "30155-07.htm"); !slices.Contains(got, want) {
		t.Fatalf("turn-in = %q, want the page 30155-07", got)
	}
	if a, r, v := held(57), held(ratFang), held(varoolFang); a != 11*30+10000+5000 || r != 0 || v != 0 {
		t.Fatalf("adena, rat and Varool fangs = %d, %d, %d; want %d, 0, 0", a, r, v, 11*30+10000+5000)
	}

	if got, want := w.questWindow(t, npcID), w.page(t, "ellenia", quest, "30155-05.htm"); !slices.Contains(got, want) {
		t.Fatalf("visit with no fang = %q, want the page 30155-05", got)
	}
	if a := held(57); a != 11*30+10000+5000 {
		t.Fatalf("adena after a visit with no fang = %d, want it unchanged", a)
	}
}
