package quest

import (
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestQ635PostLeavesBehindWhoseFragmentsAreGone: a member whose dimension
// fragments are moved away after the post checked every member and before
// his own payment stays behind: he is not moved and his memo state is
// untouched; the rest of the party pays and goes in as before. The payment
// is checked, an owner-approved divergence (#5165): no one enters on
// fragments he no longer holds. It runs on the inline executor and on the
// real pool.
func TestQ635PostLeavesBehindWhoseFragmentsAreGone(t *testing.T) {
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
			postLeavesBehindWhoseFragmentsAreGone(t, tc.opts...)
		})
	}
}

func postLeavesBehindWhoseFragmentsAreGone(t *testing.T, opts ...gameservertest.Option) {
	// The room draw comes after every member's check and before the
	// payments: the move runs there.
	var move atomic.Pointer[func()]
	rnd := func(n int) int {
		if n != 8 {
			return 0
		}
		if f := move.Swap(nil); f != nil {
			(*f)()
		}
		return 5
	}
	w := q635Boot(t, q635Spec{
		mates: 2,
		rules: q635Rules(func(r *script.EventRules) { r.Rift.Costs[0] = 7 }),
		extra: append(opts, gameservertest.WithScriptRand(rnd)),
	})
	mate1 := onlineCharacter(t, w.srv, w.ids["Mate1"])
	memo := w.memo(t, "Mate1", 0)
	moved := false
	takeFragments := func() { moved = mate1.ConsumeItem(q635Fragment, 60) }
	move.Store(&takeFragments)

	got := w.enter(t, "Leader")
	requireLine(t, got, htmlLine("post", q635Page(t, "rift_watcher_05.htm", "")))
	if !moved {
		t.Fatal("Mate1's fragments were not moved away during the entry")
	}

	if r := w.roomOf(t, "Mate1"); r != -1 {
		t.Errorf("Mate1 stands in room %d, want to stay out", r)
	}
	if n := w.fragments(t, "Mate1"); n != 0 {
		t.Errorf("Mate1 holds %d fragments, want 0", n)
	}
	if s := w.memo(t, "Mate1", 0); s != memo {
		t.Errorf("Mate1 memo state = %d, want %d as before", s, memo)
	}
	for _, who := range []string{"Leader", "Mate2"} {
		if r := w.roomOf(t, who); r != 5 {
			t.Errorf("%s stands in room %d, want 5", who, r)
		}
		if n := w.fragments(t, who); n != 53 {
			t.Errorf("%s holds %d fragments, want 53", who, n)
		}
	}
}
