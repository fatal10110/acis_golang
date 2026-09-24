// Package testsupport holds the shared scripted game client used by tests
// that drive the server through the real wire protocol. It deliberately does
// not import the network package: in-package tests of packages that
// gameservertest wires up (notably internal/gameserver/network) must be able
// to import this client without an import cycle.
package testsupport

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// ScriptedClient drives the game wire protocol from the client side against
// a live server listener.
//
// A real game client speaks first: it sends ProtocolVersion cleartext,
// receives VersionCheck cleartext, then arms the rolling XOR cipher from
// VersionCheck's 8 random bytes plus the fixed static key half.
type ScriptedClient struct {
	t          *testing.T
	conn       net.Conn
	handshaken bool
	cipher     *gamecipher.Cipher

	// sent and received count whole frames written and read.
	sent, received atomic.Int64
	// await, when set, stands in for waiting up to d for a frame, and now
	// reads the clock it advances; see SetAwait.
	await func(d time.Duration) bool
	now   func() time.Time
}

// frameInFlight is how long a read waits for a frame await reported as
// already on its way.
const frameInFlight = 5 * time.Second

// SetAwait makes every read wait through await instead of the wall clock:
// await(d) lets up to d pass on the clock now reads and reports whether a
// frame is on its way to this client. A read that finds none returns as a
// wall-clock read that timed out would. Set it before the client is used.
func (f *ScriptedClient) SetAwait(await func(d time.Duration) bool, now func() time.Time) {
	f.await, f.now = await, now
}

// Now reads the clock reads wait on: SetAwait's clock, else the wall clock.
// Bound a loop of reads on it, not on time.Now: on a driven clock each read
// that finds no frame moves this clock, not the wall clock.
func (f *ScriptedClient) Now() time.Time {
	if f.now != nil {
		return f.now()
	}
	return time.Now()
}

// Sent is the number of frames this client has written.
func (f *ScriptedClient) Sent() int64 { return f.sent.Load() }

// Received is the number of frames this client has read.
func (f *ScriptedClient) Received() int64 { return f.received.Load() }

// LocalAddr is the client end of the connection, the server's remote
// address for it.
func (f *ScriptedClient) LocalAddr() net.Addr { return f.conn.LocalAddr() }

// readFrame reads one raw frame, waiting up to d for it to start, and counts
// it. A frame whose first byte has arrived is read to the end within
// frameInFlight: a deadline that ran out between its header and its payload
// would report a timeout with part of the frame already consumed, and every
// later read would start mid-frame.
func (f *ScriptedClient) readFrame(d time.Duration) ([]byte, error) {
	if f.await != nil {
		if !f.await(d) {
			return nil, os.ErrDeadlineExceeded
		}
		d = frameInFlight
	}
	var first [1]byte
	f.conn.SetReadDeadline(time.Now().Add(d))
	if _, err := io.ReadFull(f.conn, first[:]); err != nil {
		return nil, err
	}
	f.conn.SetReadDeadline(time.Now().Add(frameInFlight))
	payload, err := wire.ReadFrame(io.MultiReader(bytes.NewReader(first[:]), f.conn))
	if err != nil {
		// Formatted, not wrapped: a caller that tolerates a timeout must
		// not find one in here, however it inspects the error, because the
		// stream is already misaligned.
		return nil, fmt.Errorf("frame cut off after its first byte: %v", err)
	}
	f.received.Add(1)
	return payload, nil
}

// writeFrame writes one raw frame and counts it.
func (f *ScriptedClient) writeFrame(payload []byte) error {
	if err := wire.WriteFrame(f.conn, payload); err != nil {
		return err
	}
	f.sent.Add(1)
	return nil
}

// Dial connects to the server at addr and registers connection cleanup with t.
func Dial(t *testing.T, addr string) *ScriptedClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })

	return &ScriptedClient{t: t, conn: conn}
}

// ReadWithTimeout reads one frame within d, returning nil on timeout instead
// of failing the test. Used to drain however many frames a rejected request
// produces (a system message plus ActionFailed, or ActionFailed alone)
// without hard-coding an exact count, while still treating "nothing at all"
// as a failure.
func (f *ScriptedClient) ReadWithTimeout(d time.Duration) []byte {
	f.t.Helper()
	payload, err := f.TryRead(d)
	if err != nil {
		f.t.Fatalf("ReadFrame: %v", err)
	}
	return payload
}

// TryRead is ReadWithTimeout for drivers that count a failed connection
// instead of failing the test: nil, nil on timeout, the read error otherwise.
// Safe to call from a goroutine other than the test's.
func (f *ScriptedClient) TryRead(d time.Duration) ([]byte, error) {
	payload, err := f.readFrame(d)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil, nil
		}
		return nil, err
	}
	if f.cipher != nil {
		f.cipher.Decrypt(payload)
	}
	return payload, nil
}

// AwaitClose reports whether the server closes the connection within d,
// draining any frames it sends first. Use it to assert a disconnect that
// closes without a reply frame.
func (f *ScriptedClient) AwaitClose(d time.Duration) bool {
	f.t.Helper()
	// One budget across every frame drained, on the clock reads wait on.
	end := f.Now().Add(d)
	for {
		_, err := f.readFrame(end.Sub(f.Now()))
		if err == nil {
			continue
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return false
		}
		return true
	}
}

// SendProtocolVersion performs the cleartext handshake: it sends
// ProtocolVersion carrying revision and consumes the VersionCheck reply,
// arming the rolling cipher if the server enabled crypt.
func (f *ScriptedClient) SendProtocolVersion(revision int32) {
	f.t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
	w.WriteInt32(revision)
	if err := f.writeFrame(w.Bytes()); err != nil {
		f.t.Fatalf("write ProtocolVersion: %v", err)
	}

	raw, err := f.readFrame(5 * time.Second)
	if err != nil {
		f.t.Fatalf("read VersionCheck: %v", err)
	}
	if raw[0] != serverpackets.OpcodeVersionCheck {
		f.t.Fatalf("first packet opcode = %#x, want VersionCheck (%#x)", raw[0], serverpackets.OpcodeVersionCheck)
	}
	if len(raw) != 18 {
		f.t.Fatalf("VersionCheck payload size = %d, want 18", len(raw))
	}
	if enabled := wire.NewReader(raw[10:14]).ReadInt32(); enabled != 0 {
		key := make([]byte, gamecipher.KeySize)
		copy(key[:8], raw[2:10])
		copy(key[8:], gamecipher.StaticKey[:])

		c, err := gamecipher.NewCipher(key)
		if err != nil {
			f.t.Fatalf("NewCipher: %v", err)
		}
		c.Encrypt(nil)
		f.cipher = c
	}
	f.handshaken = true
}

// Send writes one encrypted payload frame to the server.
func (f *ScriptedClient) Send(payload []byte) {
	f.t.Helper()
	if !f.handshaken {
		f.t.Fatal("send called before ProtocolVersion/VersionCheck handshake")
	}
	if err := f.TrySend(payload); err != nil {
		f.t.Fatalf("WriteFrame: %v", err)
	}
}

// TrySend is Send for drivers that count a failed connection instead of
// failing the test. The handshake must already be done. Safe to call from a
// goroutine other than the test's.
func (f *ScriptedClient) TrySend(payload []byte) error {
	buf := append([]byte(nil), payload...)
	if f.cipher != nil {
		f.cipher.Encrypt(buf)
	}
	return f.writeFrame(buf)
}

// Read blocks until one frame arrives or the 5s deadline expires, returning
// the decrypted payload.
func (f *ScriptedClient) Read() []byte {
	f.t.Helper()
	if !f.handshaken {
		f.t.Fatal("read called before ProtocolVersion/VersionCheck handshake")
	}
	payload, err := f.readFrame(5 * time.Second)
	if err != nil {
		f.t.Fatalf("ReadFrame: %v", err)
	}
	if f.cipher != nil {
		f.cipher.Decrypt(payload)
	}
	return payload
}

// ExpectNoFrame fails the test if any frame arrives within 100ms.
func (f *ScriptedClient) ExpectNoFrame() {
	f.t.Helper()
	if payload, err := f.readFrame(100 * time.Millisecond); err == nil {
		if f.cipher != nil {
			f.cipher.Decrypt(payload)
		}
		f.t.Fatalf("unexpected frame: %x", payload)
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		f.t.Fatalf("ReadFrame: %v", err)
	}
}

// SyncBarrier sends a request guaranteed to be answered with wantOpcode and
// reads that reply. A connection's dispatch loop handles requests strictly
// in order, so reading it proves everything sent before it has already been
// processed server-side — used before driving a batching task's tick in a
// test whose triggering request has no synchronous reply of its own to
// block on.
func SyncBarrier(t *testing.T, c *ScriptedClient, send func(), wantOpcode byte) {
	t.Helper()
	if frames := SyncBarrierFrames(t, c, send, wantOpcode); len(frames) != 0 {
		t.Fatalf("sync barrier received %d frame(s) before reply: %x", len(frames), frames)
	}
}

// SyncBarrierFrames sends a request guaranteed to be answered with wantOpcode
// and returns every earlier frame in wire order. Callers must assert the
// returned frames; the helper never discards them. It is only safe when no
// earlier request can itself produce wantOpcode.
func SyncBarrierFrames(t *testing.T, c *ScriptedClient, send func(), wantOpcode byte) [][]byte {
	t.Helper()
	send()
	var frames [][]byte
	for i := 0; i < 100; i++ {
		frame := c.Read()
		if len(frame) == 0 {
			t.Fatal("sync barrier received an empty frame")
		}
		if frame[0] == wantOpcode {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("sync barrier did not receive its reply within 100 frames")
	return nil
}

// Conn exposes the underlying connection for tests that need raw socket
// access: deadline probes, EOF assertions, or writes that must precede the
// scripted handshake.
func (f *ScriptedClient) Conn() net.Conn { return f.conn }

// Close closes the underlying connection.
func (f *ScriptedClient) Close() error { return f.conn.Close() }

// ExpectClosed fails unless the server closes the connection within 2s. A
// timeout fails too: a connection still open is not closed.
func (f *ScriptedClient) ExpectClosed() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	n, err := f.conn.Read(buf)
	if ne, ok := err.(net.Error); n != 0 || err == nil || ok && ne.Timeout() {
		f.t.Fatalf("expected connection to close, got n=%d err=%v", n, err)
	}
}
