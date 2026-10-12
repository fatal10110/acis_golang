package quest

import (
	"bytes"
	"maps"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// pausedGive holds the next script give that allocates an object id at
// that allocation: past the give's checks, before the item exists.
type pausedGive struct {
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func newPausedGive() *pausedGive {
	return &pausedGive{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *pausedGive) wrap(next func() (int32, error)) func() (int32, error) {
	return func() (int32, error) {
		if g.armed.CompareAndSwap(true, false) {
			close(g.entered)
			<-g.release
		}
		return next()
	}
}

// parkedDetach reports whether the goroutine detaching a player is parked
// on a lock and, when it is, whether that lock is the one MarkDetaching
// waits on for the gives and takes already past their checks.
func parkedDetach() (parked, inMark bool) {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	for _, g := range bytes.Split(buf, []byte("\n\n")) {
		header, _, _ := bytes.Cut(g, []byte("\n"))
		if !bytes.Contains(header, []byte("[sync.")) || !bytes.Contains(g, []byte(").detachLivePlayer(")) {
			continue
		}
		return true, bytes.Contains(g, []byte("(*Character).MarkDetaching("))
	}
	return false, false
}

// TestScriptGiveFromAnotherQueueRacingDetachIsSaved raises a give and a
// take for a player from outside its queue while the player goes back to
// character select. The give is held past its checks until the detach
// stops for it, which must be where it marks the player as leaving, then
// let go: it lands before the final item save, so its row is written and
// the reselected player holds it. The take, raised once the detach waits
// at the mark, takes nothing: the stack it asked for is still saved in
// full.
//
// It runs alone, not in parallel: the detach it waits for is found among
// the process's goroutines, and no other suite's may be there.
func TestScriptGiveFromAnotherQueueRacingDetachIsSaved(t *testing.T) {
	paused := newPausedGive()
	srv, objID := bootHelpers(t, gameservertest.WithScriptItemIDs(paused.wrap))
	var s *script.Script
	var p *script.Player
	run(t, srv, objID, func(sc *script.Script, pl *script.Player) {
		sc.GiveItems(pl, dropItem+1, 5)
		s, p = sc, pl
	})
	srv.ReadQueued(t, srv.Client)

	paused.armed.Store(true)
	given := make(chan struct{})
	go func() {
		defer close(given)
		s.GiveItems(p, dropItem, 1)
	}()
	<-paused.entered

	var restarted, pastMark atomic.Bool
	taken := make(chan struct{})
	go func() {
		defer close(paused.release)
		for !restarted.Load() {
			if parked, inMark := parkedDetach(); parked {
				pastMark.Store(!inMark)
				break
			}
			runtime.Gosched()
		}
		go func() {
			defer close(taken)
			s.TakeItems(p, dropItem+1, -1)
		}()
	}()
	restart(t, srv)
	restarted.Store(true)
	<-given
	<-taken
	if pastMark.Load() {
		t.Error("the detach went past marking the player while a give was in flight")
	}
	srv.FlushItems(t)

	want := map[int32]int{dropItem: 1, dropItem + 1: 5}
	if got := itemRows(t, srv, objID); !maps.Equal(got, want) {
		t.Fatalf("item rows after the detach = %v, want %v", got, want)
	}
	enterWorld(t, srv)
	for id, n := range want {
		if got := srv.PlayerItemCount(t, objID, id); got != n {
			t.Errorf("reselected holding %d of item %d, want %d", got, id, n)
		}
	}
}
