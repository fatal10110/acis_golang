package network

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// heldFrameLog records, in send order, the frames a hold released: each
// frame's one-byte tag, and a 'v' after it when it went out through the
// visibility path.
type heldFrameLog struct{ sent []byte }

func (g *heldFrameLog) send(f heldFrame) {
	g.sent = append(g.sent, f.frame.Bytes()[0])
	if f.visibility {
		g.sent = append(g.sent, 'v')
	}
}

func (g *heldFrameLog) want(t *testing.T, step, want string) {
	t.Helper()
	if string(g.sent) != want {
		t.Fatalf("%s: sent %q, want %q", step, g.sent, want)
	}
}

func tagFrame(tag byte) wire.Frame { return wire.BorrowedFrame([]byte{tag}) }

// TestPendingPvPChangesHold walks the hold a player's PvP flag changes put
// on its frames through each of its branches, step by step.
func TestPendingPvPChangesHold(t *testing.T) {
	t.Run("nothing pending sends at once", func(t *testing.T) {
		var p pendingPvPChanges
		if held, started := p.holdBehind(tagFrame('a')); held || started {
			t.Fatalf("holdBehind with nothing pending = %v, %v, want not held", held, started)
		}
		if p.hold(tagFrame('b'), false) {
			t.Fatal("hold with nothing held held the frame")
		}
	})

	// A later plain send waits behind a frame held behind a pending change,
	// and the settle that runs the change sends both, in order, through
	// their own client paths.
	t.Run("later sends wait behind the held frame", func(t *testing.T) {
		var p pendingPvPChanges
		var log heldFrameLog
		p.push(pvpChange{reset: true})
		if held, started := p.holdBehind(tagFrame('a')); !held || !started {
			t.Fatalf("holdBehind with a change pending = %v, %v, want held and started", held, started)
		}
		if held, started := p.holdBehind(tagFrame('b')); !held || started {
			t.Fatalf("second holdBehind = %v, %v, want held, not started", held, started)
		}
		if !p.hold(tagFrame('c'), false) || !p.hold(tagFrame('d'), true) {
			t.Fatal("a later send overtook the held frames")
		}
		if changes := p.take(); len(changes) != 1 || !changes[0].reset {
			t.Fatalf("take = %+v, want the pending reset", changes)
		}
		log.want(t, "during the settle", "")
		p.release(log.send)
		log.want(t, "after the settle", "abcdv")
		if p.holding.Load() || p.hold(tagFrame('e'), false) {
			t.Fatal("the hold outlived the flush")
		}
	})

	// A frame the settle itself sends is the settle's own and goes out at
	// once, ahead of the frames held behind it.
	t.Run("sends during a settle are not held", func(t *testing.T) {
		var p pendingPvPChanges
		var log heldFrameLog
		p.push(pvpChange{reset: true})
		p.holdBehind(tagFrame('a'))
		p.take()
		if p.hold(tagFrame('s'), false) {
			t.Fatal("hold held a frame sent while the settle runs")
		}
		// The proc's own frames still wait behind a running settle.
		if held, _ := p.holdBehind(tagFrame('b')); !held {
			t.Fatal("holdBehind sent at once while the settle runs")
		}
		p.release(log.send)
		log.want(t, "after the settle", "ab")
	})

	// Nested settles flush only once the outer one ends.
	t.Run("an inner settle leaves the flush to the outer one", func(t *testing.T) {
		var p pendingPvPChanges
		var log heldFrameLog
		p.push(pvpChange{})
		p.holdBehind(tagFrame('a'))
		p.take()
		p.take()
		p.release(log.send)
		log.want(t, "after the inner settle", "")
		p.release(log.send)
		log.want(t, "after the outer settle", "a")
	})

	// A change queued while the flush sends keeps the hold for the settle
	// it posted: a frame sent after it waits for that settle.
	t.Run("a change pushed during the flush keeps the hold", func(t *testing.T) {
		var p pendingPvPChanges
		var log heldFrameLog
		p.push(pvpChange{reset: true})
		p.holdBehind(tagFrame('a'))
		p.hold(tagFrame('b'), false)
		p.take()
		p.release(func(f heldFrame) {
			if f.frame.Bytes()[0] == 'a' {
				p.push(pvpChange{useFlaggedDuration: true})
			}
			log.send(f)
		})
		log.want(t, "after the first settle", "ab")
		if !p.holding.Load() {
			t.Fatal("the hold ended with a change still pending")
		}
		if !p.hold(tagFrame('c'), false) {
			t.Fatal("a send after the new change overtook its settle")
		}
		p.take()
		p.release(log.send)
		log.want(t, "after the second settle", "abc")
		if p.holding.Load() {
			t.Fatal("the hold outlived the second settle's flush")
		}
	})

	// A frame held while the flush sends goes out after the ones it sends.
	t.Run("a frame held during the flush follows it", func(t *testing.T) {
		var p pendingPvPChanges
		var log heldFrameLog
		p.push(pvpChange{reset: true})
		p.holdBehind(tagFrame('a'))
		p.take()
		p.release(func(f heldFrame) {
			if f.frame.Bytes()[0] == 'a' && !p.hold(tagFrame('b'), true) {
				t.Fatal("a send during the flush overtook it")
			}
			log.send(f)
		})
		log.want(t, "after the settle", "abv")
		if p.holding.Load() {
			t.Fatal("the hold outlived the flush")
		}
	})

	// A forced flush, for a player whose queue takes no more tasks, sends
	// every held frame although a change is still pending.
	t.Run("a forced flush sends past a pending change", func(t *testing.T) {
		var p pendingPvPChanges
		var log heldFrameLog
		p.push(pvpChange{reset: true})
		p.holdBehind(tagFrame('a'))
		p.hold(tagFrame('b'), true)
		p.flush(log.send, false)
		log.want(t, "unforced flush with a change pending", "")
		p.flush(log.send, true)
		log.want(t, "forced flush", "abv")
		if p.holding.Load() || p.hold(tagFrame('c'), false) {
			t.Fatal("the hold outlived the forced flush")
		}
	})
}

// holdTestLive is a player on its own inline queue whose client paths
// record the frames they send, tagged as heldFrameLog does.
func holdTestLive(t *testing.T) (*livePlayer, *sim.Inline, *heldFrameLog) {
	t.Helper()
	in := sim.NewInline(time.Unix(0, 0))
	ch := &player.Character{ID: 7}
	creatureLive, err := creature.NewLive(location.Location{}, 0, testGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	creatureLive.SetQueue(in.NewQueue("hold-test"))
	ch.Live = creatureLive
	log := &heldFrameLog{}
	live := &livePlayer{
		Character: ch, log: zerolog.Nop(),
		session: func(f wire.Frame) bool {
			log.send(heldFrame{frame: f})
			return true
		},
		visibilitySend: func(f wire.Frame) bool {
			log.send(heldFrame{frame: f, visibility: true})
			return true
		},
	}
	return live, in, log
}

// TestSendBehindPvPChangesHoldsLaterSendsUntilTheSettle: once another
// queue holds a frame for a player behind its pending PvP flag changes,
// the player's later SendFrame and visibility sends wait too, while the
// settle's own sends go out first; the settle then releases the rest in
// order.
func TestSendBehindPvPChangesHoldsLaterSendsUntilTheSettle(t *testing.T) {
	l := &GameClientLink{}
	live, in, log := holdTestLive(t)
	live.pvpChanges.push(pvpChange{})

	l.sendBehindPvPChanges(live, tagFrame('a'))
	live.SendFrame(tagFrame('b'))
	live.sendVisibilityFrame(tagFrame('c'))
	log.want(t, "before the settle", "")

	// The settle the hold posted, with a frame of its own: it goes out
	// ahead of the held ones.
	sim.RunOwned(live.Queue(), func() {
		live.pvpChanges.take()
		live.SendFrame(tagFrame('s'))
		live.pvpChanges.release(live.sendHeldFrame)
	})
	log.want(t, "after the settle", "sabcv")

	// The settle the hold posted finds nothing left, and later sends go
	// out at once.
	in.Run()
	live.SendFrame(tagFrame('d'))
	log.want(t, "after the posted settle", "sabcvd")
}

// TestSendBehindPvPChangesPostedSettleFlushes: the settle the hold posts
// to the player's queue runs the pending change and releases the held
// frames.
func TestSendBehindPvPChangesPostedSettleFlushes(t *testing.T) {
	l := &GameClientLink{}
	live, in, log := holdTestLive(t)
	live.pvpChanges.push(pvpChange{})

	l.sendBehindPvPChanges(live, tagFrame('a'))
	live.sendVisibilityFrame(tagFrame('b'))
	log.want(t, "before the settle", "")
	in.Run()
	log.want(t, "after the settle", "abv")
	if !live.pvpChanges.empty() || live.pvpChanges.holding.Load() {
		t.Fatal("the posted settle left the change or the hold behind")
	}
}

// TestSendBehindPvPChangesFlushesOnAClosedQueue: when the player's queue
// takes no more tasks, no settle can release the held frame, so it goes out
// at once and ends the hold; later sends are not stranded.
func TestSendBehindPvPChangesFlushesOnAClosedQueue(t *testing.T) {
	l := &GameClientLink{}
	live, _, log := holdTestLive(t)
	live.Queue().Close()
	live.pvpChanges.push(pvpChange{reset: true})

	l.sendBehindPvPChanges(live, tagFrame('a'))
	log.want(t, "after the failed post", "a")
	if live.pvpChanges.holding.Load() {
		t.Fatal("the hold outlived the forced flush")
	}
	live.SendFrame(tagFrame('b'))
	log.want(t, "a later send", "ab")
}
