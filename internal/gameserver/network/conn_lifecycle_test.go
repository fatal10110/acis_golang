package network

import (
	"encoding/binary"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/rs/zerolog"
)

// countedFrame returns a frame of total length size whose payload starts
// with seq, and counts its releases in released.
func countedFrame(t *testing.T, size int, seq uint32, released *atomic.Int64) wire.Frame {
	t.Helper()
	payload := make([]byte, size-wire.FrameHeaderSize)
	binary.LittleEndian.PutUint32(payload, seq)
	bytes, err := wire.FrameBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	return wire.OwnedFrame(bytes, nil, func(*wire.Writer) { released.Add(1) })
}

// sendWithin fails the test if SendFrame blocks instead of returning.
func sendWithin(t *testing.T, c *Conn, frame wire.Frame) bool {
	t.Helper()
	done := make(chan bool, 1)
	go func() { done <- c.SendFrame(frame) }()
	select {
	case ok := <-done:
		return ok
	case <-time.After(time.Second):
		t.Fatal("SendFrame blocked on a peer that is not reading")
		return false
	}
}

// A burst far beyond the old 64-frame queue is accepted without blocking while
// the peer is not reading yet, then arrives complete and in send order.
func TestConnDeliversLargeBurstInOrderWithoutBlocking(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	c := newConn(server, zerolog.Nop())

	const frames = 200
	var released atomic.Int64
	for i := range frames {
		if !sendWithin(t, c, countedFrame(t, 64, uint32(i), &released)) {
			t.Fatalf("frame %d rejected by a healthy connection", i)
		}
	}

	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	for i := range frames {
		payload, err := wire.ReadFrame(client)
		if err != nil {
			t.Fatalf("read frame %d: %v", i, err)
		}
		if got := binary.LittleEndian.Uint32(payload); got != uint32(i) {
			t.Fatalf("frame %d arrived as sequence %d", i, got)
		}
	}
	if _, err := wire.ReadFrame(client); err == nil {
		t.Fatal("read after Close drained the queue succeeded, want connection closed")
	}
	if err := <-closed; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := released.Load(); got != frames {
		t.Fatalf("released %d frames, want %d", got, frames)
	}
}

// A peer that never reads is disconnected once its unwritten backlog would
// pass the byte high-water mark; no send ever blocks and every frame is
// released exactly once.
func TestConnAbortsStalledReaderAtHighWater(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	c := newConn(server, zerolog.Nop())

	const frameSize = wire.MaxFrameLength
	var released atomic.Int64
	sent, accepted := 0, 0
	for ; sent <= outboundHighWater/frameSize+1; sent++ {
		if !sendWithin(t, c, countedFrame(t, frameSize, uint32(sent), &released)) {
			break
		}
		accepted++
	}
	sent++ // the rejected frame
	if accepted != outboundHighWater/frameSize {
		t.Fatalf("accepted %d frames of %d bytes, want %d under the %d-byte high-water mark",
			accepted, frameSize, outboundHighWater/frameSize, outboundHighWater)
	}
	if sendWithin(t, c, countedFrame(t, 64, 0, &released)) {
		t.Fatal("send after abort succeeded")
	}
	sent++

	select {
	case <-c.stopped:
	case <-time.After(time.Second):
		t.Fatal("aborted connection writer did not stop")
	}
	// net.Pipe may still hand over part of the write that was in flight at
	// abort; the peer must then see the connection closed.
	buf := make([]byte, frameSize)
	for {
		if _, err := client.Read(buf); err != nil {
			break
		}
	}
	if got := released.Load(); got != int64(sent) {
		t.Fatalf("released %d frames, want %d", got, sent)
	}
}
