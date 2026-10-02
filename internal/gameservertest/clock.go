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
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
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

// postureDelay is how long a sit-down or stand-up transition lasts.
const postureDelay = 2500 * time.Millisecond

// SettlePosture lets the sit-down or stand-up the player objID just started
// run out: it advances the transition's length, then waits until the
// transition has ended. On the wall clock the transition's timer can fire a
// few milliseconds after that length, timed from the ChangeWaitType reply.
func (s *Server) SettlePosture(tb testing.TB, objID int32) {
	tb.Helper()
	s.Advance(tb, postureDelay)
	c := s.onlineCharacter(tb, objID)
	s.AdvanceUntil(tb, "posture transition end", func() bool { return !c.SittingNow() && !c.StandingNow() })
}

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
	// parked holds the owners whose saves the handler is waiting for (empty:
	// every owner), nil while it is not waiting.
	parked atomic.Pointer[[]int32]
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
	conn.ObservePersistWaits(func(owners []int32) func() {
		owners = append([]int32{}, owners...)
		ct.parked.Store(&owners)
		return func() { ct.parked.Store(nil) }
	})
	return func() { ct.sent.Add(1) }, func() { ct.done.Store(true) }
}

func (t *traffic) conn(addr net.Addr) *connTraffic {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conns[addr.String()]
}

// addClient registers a harness client. On a driven clock its reads wait by
// moving the clock. On the wall clock (the real pool) its EnterWorld returns
// only once the server has handled it; see awaitEnterWorld.
func (s *Server) addClient(c *testsupport.ScriptedClient) {
	s.traffic.mu.Lock()
	s.traffic.clients = append(s.traffic.clients, c)
	s.traffic.mu.Unlock()
	if s.queues.advance != nil {
		c.SetAwait(func(d time.Duration) bool { return s.awaitFrame(c, d) }, s.queues.inline.Now)
		return
	}
	c.SetAfterSend(func(payload []byte) {
		if len(payload) > 0 && payload[0] == clientpackets.OpcodeEnterWorld {
			s.awaitEnterWorld(c)
		}
	})
}

// enterWorldTimeout bounds the wall time awaitEnterWorld waits. The login
// runs on the player's actor queue while the connection waits for it, and a
// machine running every suite at once can hold that queue back; giving up
// only hands the wait to the reads, so the bound is generous.
const enterWorldTimeout = 30 * time.Second

// awaitEnterWorld waits until the server has handled the EnterWorld c just
// sent: the player is spawned and its whole burst is queued. EnterWorld does
// both on the player's actor queue while the connection waits, so on the
// wall clock a suite reading "until quiet" right after it can stop before the
// spawn or the burst, and then find no player in sight or read the burst
// late. On a driven clock every read already waits for the server to catch
// up, and the login runs on an actor queue only the clock moves, so this wait
// is for the wall clock alone. It also returns once the connection closed or
// the login waits on a persistence lane the test holds, and gives up quietly
// after enterWorldTimeout, leaving the reads to report.
func (s *Server) awaitEnterWorld(c *testsupport.ScriptedClient) {
	deadline := time.Now().Add(enterWorldTimeout)
	for time.Now().Before(deadline) {
		ct := s.traffic.conn(c.LocalAddr())
		if ct != nil && (ct.done.Load() || ct.waits.Load()-1 >= c.Sent() || s.parkedOnHeldLane(ct)) {
			return
		}
		time.Sleep(50 * time.Microsecond)
	}
}

// catchUpTimeout bounds the wall time the server may take to handle frames
// a client already wrote.
const catchUpTimeout = 5 * time.Second

// catchUp waits until the server has handled every frame a client wrote,
// the actor queues have run everything posted and the persistence lanes the
// test does not hold have run their jobs, so nothing but the clock can start
// more work.
func (s *Server) catchUp() error {
	if err := s.awaitHandled(); err != nil {
		return err
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

// awaitHandled waits until the server has handled every frame a client
// already wrote, or its connection closed, or its handler is parked on a
// lane the test holds.
func (s *Server) awaitHandled() error {
	deadline := time.Now().Add(catchUpTimeout)
	for !s.handledAll() {
		if time.Now().After(deadline) {
			return errBehind
		}
		time.Sleep(50 * time.Microsecond)
	}
	return nil
}

// parkedOnHeldLane reports whether ct's handler is waiting for saves on a
// lane the test holds: it cannot finish until the test releases the lane, so
// the clock may move meanwhile, as a slow database round trip lets time pass.
func (s *Server) parkedOnHeldLane(ct *connTraffic) bool {
	owners := ct.parked.Load()
	if owners == nil {
		return false
	}
	for lane := range s.heldLanes {
		if s.heldLanes[lane].Load() == 0 {
			continue
		}
		if len(*owners) == 0 {
			return true
		}
		for _, id := range *owners {
			if int(persist.LaneIndex(id)) == lane {
				return true
			}
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
		if !ct.done.Load() && ct.waits.Load()-1 != c.Sent() && !s.parkedOnHeldLane(ct) {
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

// ReadQueued returns every frame the server has queued to c that c has not
// read yet, once the server has handled what c sent (see Settle). It lets no
// time pass on a driven clock, so no timer comes due meanwhile, and on the
// wall clock it does not stop at a quiet spell: a frame already queued is
// read however long the machine takes to deliver it.
func (s *Server) ReadQueued(tb testing.TB, c *testsupport.ScriptedClient) [][]byte {
	tb.Helper()
	s.Settle(tb)
	ct := s.traffic.conn(c.LocalAddr())
	if ct == nil {
		tb.Fatal("ReadQueued: client connection not tracked")
	}
	var frames [][]byte
	for c.Received() < ct.sent.Load() {
		frames = append(frames, c.Read())
	}
	return frames
}
