package quest

import (
	"testing"
	"time"
)

// TestQ334WishLatchClearsAfterRelog: a second wish while Matild's lines
// over the first play out is refused and keeps its potion. A
// character who leaves the world then stops the lines with his timers: the
// wish pays nothing and its potion stays spent, but the latch left behind
// no longer blocks him. His next talk to Matild clears it, with the wish it
// stood for, and his next potion makes a normal wish. An owner-approved
// divergence (#5169) from a latch only the last line clears.
func TestQ334WishLatchClearsAfterRelog(t *testing.T) {
	t.Parallel()
	draws := &scriptDraws{}
	w := q334World(t, draws, 0, map[int32]int32{q334WishPotion: 2, q334Orb: 1})
	if !w.srv.DrivesClock() {
		t.Skip("the wish's lines are pinned to the second, which the wall clock does not meet")
	}
	toWishes := func(step string) {
		t.Helper()
		if got := w.talk(t, "matild"); !shows(got, "30738-12.htm") {
			t.Fatalf("%s: Matild: got %q, want 30738-12.htm", step, got)
		}
		if got := w.q334Event(t, "30738-14.htm"); !shows(got, "30738-13.htm") {
			t.Fatalf("%s: 30738-14.htm: got %q, want 30738-13.htm", step, got)
		}
	}

	toWishes("the first wish")
	if got := w.q334Event(t, "30738-16.htm"); !shows(got, "30738-16.htm") {
		t.Fatalf("the first wish: got %q", got)
	}
	wantLines(t, "the first line", w.advance(t, 3*time.Second), say("matild", q334Matild, 33415))

	toWishes("a wish while the lines play out")
	w.q334Event(t, "30738-17.htm")
	if n := w.srv.PlayerItemCount(t, w.player, q334WishPotion); n != 1 {
		t.Fatalf("potions after the refused wish = %d, want 1", n)
	}
	q334Row(t, w, "wish", "1")

	restart(t, w.srv)
	enterWorld(t, w.srv)
	w.advance(t, 20*time.Second)
	q334Row(t, w, "exchange", "1")
	if n := w.srv.PlayerItemCount(t, w.player, q334Adena); n != 0 {
		t.Fatalf("adena after the lost wish = %d, want 0", n)
	}

	if got := w.talk(t, "matild"); !shows(got, "30738-12.htm") {
		t.Fatalf("Matild after the relog: got %q, want 30738-12.htm", got)
	}
	q334Row(t, w, "exchange", "0")
	q334Row(t, w, "wish", "0")

	// Wish 2's second outcome of three pays 10000 adena.
	draws.queue(1)
	q334Wish(t, w, 2)
	q334Row(t, w, "exchange", "0")
	if n := w.srv.PlayerItemCount(t, w.player, q334Adena); n != 10000 {
		t.Fatalf("adena after the wish = %d, want 10000", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q334WishPotion); n != 0 {
		t.Fatalf("potions after the wish = %d, want 0", n)
	}
}
