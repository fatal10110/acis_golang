package quest

import (
	"testing"
)

const (
	q226Bandellos    = 30473
	q226GoldenStatue = 2813
	q226Mark         = 2820
)

// offerEvent shows, as a dialog page, a page whose one link is the quest
// event "Quest <quest> <event>", and discards it.
func (w *dialogWorld) offerEvent(t *testing.T, quest, event string) {
	t.Helper()
	w.srv.ShowDialogPage(t, w.player, `<html><body><a action="bypass -h Quest `+quest+` `+event+`">Go</a></body></html>`)
	w.srv.ReadQueued(t, w.srv.Client)
}

// TestQ226HandOverPaysOnce: Bandellos's hand-over of the Golden Statue
// pays the Mark of Healer once; the same event sent again once the test
// is over answers nothing and pays nothing.
func TestQ226HandOverPaysOnce(t *testing.T) {
	t.Parallel()
	w := q226World(t, "23", q226GoldenStatue)
	w.spawnFolk(t, "bandellos", q226Bandellos, 0, -40, 0)
	w.talk(t, "bandellos")

	w.offerEvent(t, q226Name, "30473-09.htm")
	if got := w.send(t, encodeBypass("Quest "+q226Name+" 30473-09.htm")); len(got) == 0 {
		t.Fatal("hand-over answered nothing")
	}
	if n := w.srv.PlayerItemCount(t, w.player, q226Mark); n != 1 {
		t.Fatalf("marks after the hand-over = %d, want 1", n)
	}
	w.offerEvent(t, q226Name, "30473-09.htm")
	if got := w.send(t, encodeBypass("Quest "+q226Name+" 30473-09.htm")); len(got) != 0 {
		t.Fatalf("hand-over again = %q, want nothing", got)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q226Mark); n != 1 {
		t.Fatalf("marks after a second hand-over = %d, want 1", n)
	}
}

// TestQ229LastHandOverRefusedBeforeCond10: Orim's last hand-over sent
// before cond 10 answers nothing.
func TestQ229LastHandOverRefusedBeforeCond10(t *testing.T) {
	t.Parallel()
	w := q229World(t, "9", "", map[int32]int32{q229Crystal: 1}, false)
	w.talk(t, "orim")
	w.offerEvent(t, q229Name, "30630-22.htm")
	if got := w.send(t, encodeBypass("Quest "+q229Name+" 30630-22.htm")); len(got) != 0 {
		t.Fatalf("hand-over on cond 9 = %q, want nothing", got)
	}
	if n := w.srv.PlayerItemCount(t, w.player, q229Crystal); n != 1 {
		t.Fatalf("crystals = %d, want 1", n)
	}
}

// TestQ229LastHandOverPaysOnce: Orim's last hand-over pays on cond 10 to
// a holder of Zeruel's crystal; sent again once the test is over, it
// answers nothing.
func TestQ229LastHandOverPaysOnce(t *testing.T) {
	t.Parallel()
	w := q229World(t, "10", "", map[int32]int32{q229Crystal: 1}, false)
	w.talk(t, "orim")
	w.offerEvent(t, q229Name, "30630-22.htm")
	if got := w.send(t, encodeBypass("Quest "+q229Name+" 30630-22.htm")); len(got) == 0 {
		t.Fatal("hand-over answered nothing")
	}
	if n := w.srv.PlayerItemCount(t, w.player, q229Crystal); n != 0 {
		t.Fatalf("crystals after the hand-over = %d, want 0", n)
	}
	w.offerEvent(t, q229Name, "30630-22.htm")
	if got := w.send(t, encodeBypass("Quest "+q229Name+" 30630-22.htm")); len(got) != 0 {
		t.Fatalf("hand-over again = %q, want nothing", got)
	}
}
