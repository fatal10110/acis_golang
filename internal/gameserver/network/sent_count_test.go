package network

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// servedFrames maps a test listener's address to the frame counts of the
// connections it serves, so dialCounted can give its client a sent-frame
// count.
var servedFrames sync.Map // string -> *frameCounts

// catchUpTimeout bounds the wall time a test server may take to handle the
// frames a client already wrote and run the work they posted. It only bounds
// how long a failure takes to show.
const catchUpTimeout = 5 * time.Second

// frameCounts counts the frames crossing each connection of one test server,
// keyed by the client's address, and records the actor queues the server
// creates, so a test can ask how many frames the server has queued to a
// client once it has caught up, instead of waiting out a quiet spell.
type frameCounts struct {
	queues *countedQueues

	mu    sync.Mutex
	conns map[string]*connFrames
}

type connFrames struct {
	reads atomic.Int64 // reads started: frames fully handled + 1
	sent  atomic.Int64 // frames queued to the client
	done  atomic.Bool  // the connection's handler returned
}

// serveFrames registers counts for the listener at addr until the test ends.
func serveFrames(t *testing.T, addr string, queues *countedQueues) *frameCounts {
	t.Helper()
	counts := &frameCounts{queues: queues, conns: make(map[string]*connFrames)}
	servedFrames.Store(addr, counts)
	t.Cleanup(func() { servedFrames.Delete(addr) })
	return counts
}

// track starts counting conn's frames; the returned func marks it closed.
// Call it before conn reaches its handler.
func (f *frameCounts) track(conn *Conn) (done func()) {
	cf := new(connFrames)
	f.mu.Lock()
	f.conns[conn.RemoteAddr().String()] = cf
	f.mu.Unlock()
	conn.ObserveReads(func() { cf.reads.Add(1) })
	conn.ObserveSends(func([]byte) { cf.sent.Add(1) })
	return func() { cf.done.Store(true) }
}

// queuedTo waits until the server has handled every frame c wrote (or c's
// connection closed) and the actor queues have run what was posted to them,
// then returns how many frames the server has queued to c in all.
func (f *frameCounts) queuedTo(c *testsupport.ScriptedClient) (int64, error) {
	deadline := time.Now().Add(catchUpTimeout)
	for {
		f.mu.Lock()
		cf := f.conns[c.LocalAddr().String()]
		f.mu.Unlock()
		if cf != nil && (cf.done.Load() || cf.reads.Load()-1 >= c.Sent()) {
			if err := f.queues.settle(deadline); err != nil {
				return 0, err
			}
			return cf.sent.Load(), nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("server did not handle every client frame within %v", catchUpTimeout)
		}
		time.Sleep(50 * time.Microsecond)
	}
}

// dialCounted dials the test server at addr with a client whose ReadQueued
// and ExpectNoFrame read the server's sent-frame count for it.
func dialCounted(t *testing.T, addr string) *testsupport.ScriptedClient {
	t.Helper()
	v, ok := servedFrames.Load(addr)
	if !ok {
		t.Fatalf("dialCounted: no frame counts for %s", addr)
	}
	counts := v.(*frameCounts)
	c := testsupport.Dial(t, addr)
	c.SetQueued(func() (int64, error) { return counts.queuedTo(c) })
	return c
}

// countedQueues is a sim.Pool that records every queue it creates, so a test
// can wait for the work already posted to them.
type countedQueues struct {
	*sim.Pool

	mu  sync.Mutex
	all []*sim.Queue
}

func (q *countedQueues) NewQueue(id string) *sim.Queue {
	queue := q.Pool.NewQueue(id)
	q.mu.Lock()
	q.all = append(q.all, queue)
	q.mu.Unlock()
	return queue
}

var errUnsettled = errors.New("actor queues did not settle")

// settle waits until every task posted so far to a still-open queue has run,
// and a closed queue's in-flight drain has returned, failing at deadline.
func (q *countedQueues) settle(deadline time.Time) error {
	q.mu.Lock()
	all := append([]*sim.Queue(nil), q.all...)
	q.mu.Unlock()
	timeout := time.After(time.Until(deadline))
	var closed []*sim.Queue
	for _, queue := range all {
		done := make(chan struct{})
		if !queue.Post(func() { close(done) }) {
			closed = append(closed, queue)
			continue
		}
		select {
		case <-done:
		case <-timeout:
			return errUnsettled
		}
	}
	if len(closed) == 0 {
		return nil
	}
	done := make(chan struct{})
	go func() {
		for _, queue := range closed {
			sim.RunOwned(queue, func() {})
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-timeout:
		return errUnsettled
	}
}

// TestSentCountReadsUnreadReplies pins that a counted client's ReadQueued
// returns the replies the server queued to it without the client reading
// first, both before and after it enters the world, so ExpectNoFrame on it
// cannot pass while a reply is still on its way.
func TestSentCountReadsUnreadReplies(t *testing.T) {
	c, _, _, _ := newLinkedGameClientSeedOneChar(t)

	// A create whose name fails validation is answered CharCreateFail and,
	// unlike a restore, leaves the character-select reuse gate open.
	c.Send(encodeRequestCharacterCreate("!!", 0, 0, 0, 1, 0, 0))
	if frames := c.ReadQueued(); len(frames) != 1 || frames[0][0] != serverpackets.OpcodeCharCreateFail {
		t.Fatalf("create: queued frames = %x, want one CharCreateFail", frames)
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeRequestManorList())
	if frames := c.ReadQueued(); len(frames) != 1 || frames[0][0] != serverpackets.OpcodeExtended {
		t.Fatalf("manor list: queued frames = %x, want one ExSendManorList", frames)
	}
	c.ExpectNoFrame()
}
