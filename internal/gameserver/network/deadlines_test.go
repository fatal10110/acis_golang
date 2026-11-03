package network

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

type deadlineConn struct {
	net.Conn
	readDeadline  chan time.Time
	writeDeadline chan time.Time
}

func (c *deadlineConn) SetReadDeadline(deadline time.Time) error {
	select {
	case c.readDeadline <- deadline:
	default:
	}
	return c.Conn.SetReadDeadline(deadline)
}

func (c *deadlineConn) SetWriteDeadline(deadline time.Time) error {
	select {
	case c.writeDeadline <- deadline:
	default:
	}
	return c.Conn.SetWriteDeadline(deadline)
}

func TestSessionReadFrameSetsHandshakeDeadline(t *testing.T) {
	server, client := net.Pipe()
	conn := newConn(&deadlineConn{Conn: server, readDeadline: make(chan time.Time, 1)}, zerolog.Nop())
	t.Cleanup(func() {
		_ = client.Close()
		_ = conn.Close()
	})
	cipher, err := gamecipher.NewCipher(make([]byte, gamecipher.KeySize))
	if err != nil {
		t.Fatal(err)
	}

	go func() { _, _ = NewSession(conn, cipher).ReadFrame() }()
	select {
	case deadline := <-conn.Conn.(*deadlineConn).readDeadline:
		if remaining := time.Until(deadline); remaining < 59*time.Second || remaining > 61*time.Second {
			t.Fatalf("read deadline remaining = %s, want about 60s", remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("ReadFrame did not set a handshake deadline")
	}
}

func TestSessionKeepsHandshakeDeadlineUntilExplicitlyCompleted(t *testing.T) {
	server, client := net.Pipe()
	tracked := &deadlineConn{Conn: server, readDeadline: make(chan time.Time, 2)}
	conn := newConn(tracked, zerolog.Nop())
	t.Cleanup(func() {
		_ = client.Close()
		_ = conn.Close()
	})
	cipher, err := gamecipher.NewCipher(make([]byte, gamecipher.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession(conn, cipher)

	first := make(chan error, 1)
	go func() { _, err := session.ReadFrame(); first <- err }()
	if err := wire.WriteFrame(client, []byte{0}); err != nil {
		t.Fatal(err)
	}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	<-tracked.readDeadline

	second := make(chan error, 1)
	go func() { _, err := session.ReadFrame(); second <- err }()
	select {
	case deadline := <-tracked.readDeadline:
		if remaining := time.Until(deadline); remaining < 59*time.Second || remaining > 61*time.Second {
			t.Fatalf("second pre-auth deadline remaining = %s, want about 60s", remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("second pre-auth ReadFrame did not set a deadline")
	}
	if err := wire.WriteFrame(client, []byte{0}); err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}

	session.CompleteHandshake()
	go func() { _, _ = session.ReadFrame() }()
	select {
	case deadline := <-tracked.readDeadline:
		if remaining := time.Until(deadline); remaining < 14*time.Minute || remaining > 16*time.Minute {
			t.Fatalf("established-session deadline remaining = %s, want about 15m", remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("established-session ReadFrame did not set a deadline")
	}
}

func TestSessionReadTimeoutOverride(t *testing.T) {
	override := time.Duration(0)
	session := &Session{handshaking: true, idleTimeout: func() time.Duration { return override }}
	override = time.Second
	if got := session.readTimeout(); got != clientReadHandshakeTimeout {
		t.Fatalf("handshake read timeout = %s, want %s: the override is for established sessions only", got, clientReadHandshakeTimeout)
	}
	session.CompleteHandshake()
	if got := session.readTimeout(); got != time.Second {
		t.Fatalf("established read timeout = %s, want the 1s override", got)
	}
	override = 0
	if got := session.readTimeout(); got != clientReadIdleTimeout {
		t.Fatalf("established read timeout with a zero override = %s, want %s", got, clientReadIdleTimeout)
	}
}

func TestSessionSendFrameSetsWriteDeadline(t *testing.T) {
	server, client := net.Pipe()
	tracked := &deadlineConn{Conn: server, writeDeadline: make(chan time.Time, 1)}
	conn := newConn(tracked, zerolog.Nop())
	t.Cleanup(func() {
		_ = client.Close()
		_ = conn.Close()
	})
	cipher, err := gamecipher.NewCipher(make([]byte, gamecipher.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	if !NewSession(conn, cipher).SendFrame(serverpackets.FrameActionFailed()) {
		t.Fatal("SendFrame returned false")
	}

	select {
	case deadline := <-tracked.writeDeadline:
		if remaining := time.Until(deadline); remaining < 59*time.Second || remaining > 61*time.Second {
			t.Fatalf("write deadline remaining = %s, want about 60s", remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("SendFrame did not set a write deadline")
	}
}

func TestLoginLinkEstablishedReadClearsDeadline(t *testing.T) {
	server, client := net.Pipe()
	tracked := &deadlineConn{Conn: server, readDeadline: make(chan time.Time, 1)}
	t.Cleanup(func() { _ = client.Close() })
	l := &LoginLink{conn: tracked, crypt: crypt.NewLinkCrypt(), frames: wire.NewFrameReader(tracked)}

	go func() { _, _ = l.readFrame() }()
	select {
	case deadline := <-tracked.readDeadline:
		if !deadline.IsZero() {
			t.Fatalf("established link read deadline = %s, want none", deadline)
		}
	case <-time.After(time.Second):
		t.Fatal("established link ReadFrame did not clear its deadline")
	}
}

func TestNormalReadFrameErrorTreatsDeadlineAsExpected(t *testing.T) {
	if !normalReadFrameError(os.ErrDeadlineExceeded) {
		t.Fatal("deadline exceeded is not treated as an expected disconnect")
	}
}
