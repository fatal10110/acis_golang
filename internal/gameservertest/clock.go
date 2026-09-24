package gameservertest

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Advance lets d pass for the actor queues. On the inline executor it first
// lets the server catch up (see catchUp), then moves the Inline clock by d
// and runs every timer and task that comes due, without waiting; on the
// real pool it sleeps for d. Either way the
// tasks posted by then have run when it returns.
func (s *Server) Advance(tb testing.TB, d time.Duration) {
	tb.Helper()
	if s.queues.advance != nil {
		if err := s.catchUp(); err != nil {
			tb.Fatal(err)
		}
		s.queues.advance(d)
	} else {
		time.Sleep(d)
	}
	s.Settle(tb)
}

// DrivesClock reports whether the test moves the actor queues' clock (the
// inline executor) rather than the wall clock doing it (the real pool).
func (s *Server) DrivesClock() bool { return s.queues.advance != nil }

// advanceStep is how far AdvanceUntil moves the clock between checks, and
// advanceLimit how far it goes before giving up.
const (
	advanceStep  = 10 * time.Millisecond
	advanceLimit = 10 * time.Second
)

// AdvanceUntil lets time pass in small steps until cond holds, failing the
// test once advanceLimit has passed without it. On a driven clock the steps
// take no wall time, and cond sees every client frame handled and every
// persistence job on a lane the test does not hold run before each step.
func (s *Server) AdvanceUntil(tb testing.TB, what string, cond func() bool) {
	tb.Helper()
	for passed := time.Duration(0); !cond(); passed += advanceStep {
		if passed >= advanceLimit {
			tb.Fatalf("%s not observed within %v", what, advanceLimit)
		}
		s.Advance(tb, advanceStep)
	}
}

// traffic counts the frames crossing each server connection, keyed by the
// client's address, so a driven clock can tell when the server has caught
// up with every client and whether a frame is on its way to one.
type traffic struct {
	mu      sync.Mutex
	conns   map[string]*connTraffic
	clients []*testsupport.ScriptedClient
}

type connTraffic struct {
	waits atomic.Int64 // reads started: frames fully handled + 1
	sent  atomic.Int64 // frames queued to the client
	done  atomic.Bool  // the connection's handler returned
}

// track starts counting conn's frames; the returned func marks it closed.
func (t *traffic) track(conn *network.Conn) (sent func(), done func()) {
	ct := new(connTraffic)
	t.mu.Lock()
	if t.conns == nil {
		t.conns = make(map[string]*connTraffic)
	}
	t.conns[conn.RemoteAddr().String()] = ct
	t.mu.Unlock()
	conn.ObserveReads(func() { ct.waits.Add(1) })
	return func() { ct.sent.Add(1) }, func() { ct.done.Store(true) }
}

func (t *traffic) conn(addr net.Addr) *connTraffic {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conns[addr.String()]
}

// addClient registers a harness client; on a driven clock its reads wait by
// moving the clock.
func (s *Server) addClient(c *testsupport.ScriptedClient) {
	s.traffic.mu.Lock()
	s.traffic.clients = append(s.traffic.clients, c)
	s.traffic.mu.Unlock()
	if s.queues.advance != nil {
		c.SetAwait(func(d time.Duration) bool { return s.awaitFrame(c, d) }, s.queues.inline.Now)
	}
}

// catchUpTimeout bounds the wall time the server may take to handle frames
// a client already wrote. While a test holds a persistence lane, a handler
// waiting on that lane cannot finish until the test releases it, so catchUp
// gives up on the frames after heldLaneGrace and lets the clock move: a held
// lane is a database round trip that takes time.
const (
	catchUpTimeout = 5 * time.Second
	heldLaneGrace  = 100 * time.Millisecond
)

// catchUp waits until the server has handled every frame a client wrote,
// the actor queues have run everything posted and the persistence lanes the
// test does not hold have run their jobs, so nothing but the clock can start
// more work.
func (s *Server) catchUp() error {
	held := s.anyLaneHeld()
	limit := catchUpTimeout
	if held {
		limit = heldLaneGrace
	}
	deadline := time.Now().Add(limit)
	for !s.handledAll() {
		if time.Now().After(deadline) {
			if held {
				break
			}
			return errBehind
		}
		time.Sleep(50 * time.Microsecond)
	}
	s.queues.advance(0)
	// Database work takes no time on a driven clock: whatever the tasks
	// handed the persistence lanes lands, and its continuations run, before
	// the clock may move. A lane a test holds stays stalled.
	if err := s.flushUnheldLanes(); err != nil {
		return err
	}
	s.queues.advance(0)
	return nil
}

func (s *Server) anyLaneHeld() bool {
	for i := range s.heldLanes {
		if s.heldLanes[i].Load() != 0 {
			return true
		}
	}
	return false
}

func (s *Server) flushUnheldLanes() error {
	// One owner id per unheld lane: Flush takes owners, not lanes.
	var owners []int32
	var seen [persist.Lanes]bool
	for id, found := int32(0), 0; found < persist.Lanes; id++ {
		lane := persist.LaneIndex(id)
		if seen[lane] {
			continue
		}
		seen[lane] = true
		found++
		if s.heldLanes[lane].Load() == 0 {
			owners = append(owners, id)
		}
	}
	if len(owners) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), catchUpTimeout)
	defer cancel()
	return s.persist.Flush(ctx, owners...)
}

var errBehind = fmt.Errorf("server did not handle every client frame within %v", catchUpTimeout)

func (s *Server) handledAll() bool {
	s.traffic.mu.Lock()
	clients := append([]*testsupport.ScriptedClient(nil), s.traffic.clients...)
	s.traffic.mu.Unlock()
	for _, c := range clients {
		ct := s.traffic.conn(c.LocalAddr())
		if ct == nil {
			return false // accepted but not yet tracked
		}
		if !ct.done.Load() && ct.waits.Load()-1 != c.Sent() {
			return false
		}
	}
	return true
}

// awaitFrame lets up to d pass on the driven clock, a timer deadline at a
// time, until a frame is on its way to c or its connection closed.
func (s *Server) awaitFrame(c *testsupport.ScriptedClient, d time.Duration) bool {
	inline := s.queues.inline
	end := inline.Now().Add(d)
	for {
		if err := s.catchUp(); err != nil {
			return true // let the read itself time out and report
		}
		if ct := s.traffic.conn(c.LocalAddr()); ct != nil && (ct.done.Load() || ct.sent.Load() > c.Received()) {
			return true
		}
		now := inline.Now()
		if !now.Before(end) {
			return false
		}
		step := end.Sub(now)
		if next, ok := inline.NextTimer(); ok && next.Sub(now) < step {
			step = max(next.Sub(now), 0)
		}
		s.queues.advance(step)
	}
}
