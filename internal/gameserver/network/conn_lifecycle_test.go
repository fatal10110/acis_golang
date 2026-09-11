package network

import (
	"encoding/binary"
	"fmt"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
// pass the high-water mark; no send ever blocks and every frame is
// released exactly once.
func TestConnAbortsStalledReaderAtHighWater(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	c := newConn(server, zerolog.Nop())

	const frameSize = wire.MaxFrameLength
	// These frames own no pooled writer, so each costs its bytes plus its
	// queue slot.
	want := outboundHighWater / (frameSize + queuedFrameBytes)
	var released atomic.Int64
	sent, accepted := 0, 0
	for ; sent <= want+1; sent++ {
		if !sendWithin(t, c, countedFrame(t, frameSize, uint32(sent), &released)) {
			break
		}
		accepted++
	}
	sent++ // the rejected frame
	if accepted != want {
		t.Fatalf("accepted %d frames of %d bytes, want %d under the %d-byte high-water mark",
			accepted, frameSize, want, outboundHighWater)
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

// Tiny frames are charged the buffer and queue slot they pin, not just their
// wire bytes, so a stalled reader flooded with 3-byte replies is cut off at
// the same memory bound as one fed large frames. Each frame here is a grown
// pooled writer (4 KiB) reused for a 3-byte reply; the expected count is
// derived from that capacity, independently of how the Conn measures it.
func TestConnChargesSmallFramesTheirPinnedMemory(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	c := newConn(server, zerolog.Nop())

	const writerCap = 4096
	tinyFrame := func() wire.Frame {
		w := wire.NewFrameWriter(writerCap)
		w.WriteUint8(serverpackets.OpcodeActionFailed)
		return wire.OwnedFrame(w.Frame(), w, func(*wire.Writer) {})
	}
	perFrame := writerCap + int(unsafe.Sizeof(wire.Frame{}))
	want := outboundHighWater / perFrame

	accepted := 0
	for sendWithin(t, c, tinyFrame()) {
		accepted++
		if accepted > want+1 {
			break
		}
	}
	if accepted != want {
		t.Fatalf("accepted %d 3-byte frames each pinning a %d-byte writer, want %d under the %d-byte high-water mark",
			accepted, writerCap, want, outboundHighWater)
	}
	select {
	case <-c.stopped:
	case <-time.After(time.Second):
		t.Fatal("aborted connection writer did not stop")
	}
}

// Concurrent senders on one encrypted session queue frames in the order they
// were encrypted. The peer decrypts every frame with a mirror cipher and
// checks every byte: the rolling key advances only in bytes 8..11 of the
// 16-byte key, so a frame decrypted out of step comes back corrupt only in
// payload bytes 8..11 and 24..27 — payloads are long enough to cover both.
// Frame lengths vary so a swap can't hide behind equal key rolls.
func TestSessionConcurrentSendsKeepEncryptOrder(t *testing.T) {
	const senders, perSender = 8, 250
	key := make([]byte, gamecipher.KeySize)
	for i := range key {
		key[i] = byte(i*31 + 7)
	}
	serverCipher, err := gamecipher.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	peerCipher, err := gamecipher.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	peerCipher.Encrypt(nil) // arm it, as the client does once VersionCheck arrives
	server, client := net.Pipe()
	session := NewSession(newConn(server, zerolog.Nop()), serverCipher)
	t.Cleanup(func() {
		client.Close()
		_ = session.conn.Close() // joins the writer goroutine
	})
	session.EnableCrypt()

	received := make(chan error, 1)
	go func() {
		next := make([]uint32, senders)
		for range senders * perSender {
			payload, err := wire.ReadFrame(client)
			if err != nil {
				received <- err
				return
			}
			peerCipher.Decrypt(payload)
			sender, seq := payload[0], binary.LittleEndian.Uint32(payload[1:5])
			if int(sender) >= senders || seq != next[sender] || len(payload) != orderPayloadLen(seq) {
				received <- fmt.Errorf("decrypted frame sender=%d seq=%d len=%d; stream out of order or corrupt", sender, seq, len(payload))
				return
			}
			for i := 5; i < len(payload); i++ {
				if payload[i] != orderFill(sender, i) {
					received <- fmt.Errorf("sender=%d seq=%d byte %d corrupt; encrypt/queue order diverged", sender, seq, i)
					return
				}
			}
			next[sender]++
		}
		received <- nil
	}()

	var wg sync.WaitGroup
	for s := range senders {
		wg.Go(func() {
			for seq := range uint32(perSender) {
				payload := make([]byte, orderPayloadLen(seq))
				payload[0] = byte(s)
				binary.LittleEndian.PutUint32(payload[1:5], seq)
				for i := 5; i < len(payload); i++ {
					payload[i] = orderFill(byte(s), i)
				}
				bytes, err := wire.FrameBytes(payload)
				if err != nil {
					t.Error(err)
					return
				}
				if !session.SendFrame(wire.BorrowedFrame(bytes)) {
					t.Error("SendFrame rejected a frame on a healthy connection")
					return
				}
			}
		})
	}
	wg.Wait()
	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("peer did not receive every frame")
	}
}

// orderPayloadLen covers payload bytes 24..27, the second span the key roll
// reaches, in every frame.
func orderPayloadLen(seq uint32) int { return 28 + int(seq%13) }

func orderFill(sender byte, i int) byte { return byte(i*7) ^ sender }

// BenchmarkSessionSendFrameParallel measures SendFrame when every CPU sends
// to one session at once — the broadcast fan-out's worst case: all
// contention is Session.mu (one encrypt + one append) and Conn.mu (the
// append, and the writer's once-per-batch swap).
func BenchmarkSessionSendFrameParallel(b *testing.B) {
	session := benchmarkSession(b)
	session.EnableCrypt()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for i := 0; pb.Next(); i++ {
			if i%64 == 0 {
				awaitDrain(session.conn)
			}
			if !session.SendFrame(serverpackets.FrameActionFailed()) {
				b.Error("SendFrame rejected a frame")
				return
			}
		}
	})
}

// awaitDrain yields until c's writer has worked its backlog below half the
// high-water mark. Benchmarks offer unbounded load, which would otherwise
// outrun the writer and trip the stalled-peer abort; pacing them to the
// drain rate measures SendFrame itself.
func awaitDrain(c *Conn) {
	for {
		c.mu.Lock()
		pending := c.pending
		c.mu.Unlock()
		if pending < outboundHighWater/2 {
			return
		}
		runtime.Gosched()
	}
}
