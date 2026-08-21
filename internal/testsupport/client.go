// Package testsupport holds the shared scripted game client used by tests
// that drive the server through the real wire protocol. It deliberately does
// not import the network package: in-package tests of packages that
// gameservertest wires up (notably internal/gameserver/network) must be able
// to import this client without an import cycle.
package testsupport

import (
	"net"
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
	f.conn.SetReadDeadline(time.Now().Add(d))
	payload, err := wire.ReadFrame(f.conn)
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return nil
		}
		f.t.Fatalf("ReadFrame: %v", err)
	}
	if f.cipher != nil {
		f.cipher.Decrypt(payload)
	}
	return payload
}

// SendProtocolVersion performs the cleartext handshake: it sends
// ProtocolVersion carrying revision and consumes the VersionCheck reply,
// arming the rolling cipher if the server enabled crypt.
func (f *ScriptedClient) SendProtocolVersion(revision int32) {
	f.t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
	w.WriteInt32(revision)
	if err := wire.WriteFrame(f.conn, w.Bytes()); err != nil {
		f.t.Fatalf("write ProtocolVersion: %v", err)
	}

	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	raw, err := wire.ReadFrame(f.conn)
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
	buf := append([]byte(nil), payload...)
	if f.cipher != nil {
		f.cipher.Encrypt(buf)
	}
	if err := wire.WriteFrame(f.conn, buf); err != nil {
		f.t.Fatalf("WriteFrame: %v", err)
	}
}

// Read blocks until one frame arrives or the 5s deadline expires, returning
// the decrypted payload.
func (f *ScriptedClient) Read() []byte {
	f.t.Helper()
	if !f.handshaken {
		f.t.Fatal("read called before ProtocolVersion/VersionCheck handshake")
	}
	f.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := wire.ReadFrame(f.conn)
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
	f.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if payload, err := wire.ReadFrame(f.conn); err == nil {
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
	send()
	if reply := c.Read(); reply[0] != wantOpcode {
		t.Fatalf("sync barrier opcode = %#x, want %#x", reply[0], wantOpcode)
	}
}

// Conn exposes the underlying connection for tests that need raw socket
// access: deadline probes, EOF assertions, or writes that must precede the
// scripted handshake.
func (f *ScriptedClient) Conn() net.Conn { return f.conn }

// Close closes the underlying connection.
func (f *ScriptedClient) Close() error { return f.conn.Close() }

// ExpectClosed fails unless the server closes the connection within 2s.
func (f *ScriptedClient) ExpectClosed() {
	f.t.Helper()
	f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if n, err := f.conn.Read(buf); n != 0 || err == nil {
		f.t.Fatalf("expected connection to close, got n=%d err=%v", n, err)
	}
}
