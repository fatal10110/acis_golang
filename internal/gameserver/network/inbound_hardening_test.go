package network

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamecipher "github.com/fatal10110/acis_golang/internal/gameserver/network/cipher"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSessionEmptyFrameKeepsReadDeadline pins that a frame with no body, a
// full header or one shorter than itself, does not re-arm the read
// deadline: only the read after a frame with a body sets a fresh one, so
// empty frames cannot hold a connection open past it.
func TestSessionEmptyFrameKeepsReadDeadline(t *testing.T) {
	server, client := net.Pipe()
	tracked := &deadlineConn{Conn: server, readDeadline: make(chan time.Time, 8)}
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

	writeErr := make(chan error, 1)
	go func() {
		if _, err := client.Write([]byte{0x01, 0x00}); err != nil {
			writeErr <- err
			return
		}
		if err := wire.WriteFrame(client, nil); err != nil {
			writeErr <- err
			return
		}
		writeErr <- wire.WriteFrame(client, []byte{0x2a})
	}()
	for i, want := range [][]byte{{}, {}, {0x2a}} {
		got, err := session.ReadFrame()
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("read %d payload = %x, want %x", i, got, want)
		}
	}
	if err := <-writeErr; err != nil {
		t.Fatal(err)
	}
	if n := len(tracked.readDeadline); n != 1 {
		t.Fatalf("deadlines set over two empty frames and one with a body = %d, want 1", n)
	}
	<-tracked.readDeadline

	go func() { _, _ = session.ReadFrame() }()
	select {
	case <-tracked.readDeadline:
	case <-time.After(5 * time.Second):
		t.Fatal("the read after a frame with a body did not set a fresh deadline")
	}
}

// TestAuthedEmptyFramesCountTowardFloodGate pins that an authenticated
// client's frames with no body count toward the packet flood gate: with the
// accounting clock frozen, enough empty frames make the next real request
// the flood onset, answered ActionFailed and dropped instead of dispatched.
func TestAuthedEmptyFramesCountTowardFloodGate(t *testing.T) {
	testLinkNow = func() time.Time { return time.Unix(0, 0) }
	defer func() { testLinkNow = nil }()
	c, _, _, _ := newLinkedGameClient(t)

	// The two handshake frames and these empty ones fill the window to
	// maxPacketsPerSecond; the create after them is packet
	// maxPacketsPerSecond+1, the flood onset.
	for range maxPacketsPerSecond - 2 {
		c.Send(nil)
	}
	c.Send(encodeRequestCharacterCreate("!!", 0, 0, 0, 1, 0, 0))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("reply to the request after the empty frames = %#x, want ActionFailed (%#x)", frame[0], serverpackets.OpcodeActionFailed)
	}
	c.ExpectNoFrame()
}

// TestDecodeErrorsLoggedUpToWindowLimit pins that malformed packets whose
// decode fails other than on a short buffer are dropped without a
// disconnect, and only the first maxLoggedDecodeErrorsPerMin of a window are
// logged, plus one line saying the rest go unlogged.
func TestDecodeErrorsLoggedUpToWindowLimit(t *testing.T) {
	var logged bytes.Buffer
	l := newBareGameClientLink(t, GameClientLinkConfig{Log: zerolog.New(&logged)})
	client := newDispatchTestClient(t)
	client.SetAuthenticated("acc", client.SessionKey())
	bad := errors.New("field out of range")

	for i := range maxLoggedDecodeErrorsPerMin + 5 {
		_, err := decodeClientPacket(l, client, []byte{0x01}, func([]byte) (struct{}, error) { return struct{}{}, bad })
		if !errors.Is(err, bad) {
			t.Fatalf("decode %d err = %v, want the decode error (dropped, no disconnect)", i, err)
		}
	}
	lines := strings.Split(strings.TrimSpace(logged.String()), "\n")
	if len(lines) != maxLoggedDecodeErrorsPerMin+1 {
		t.Fatalf("logged %d lines, want %d:\n%s", len(lines), maxLoggedDecodeErrorsPerMin+1, logged.String())
	}
	if last := lines[len(lines)-1]; !strings.Contains(last, "further ones this minute are dropped unlogged") {
		t.Fatalf("last line = %s, want the limit notice", last)
	}
	if client.State() != StateAuthed {
		t.Fatalf("state = %s, want authed (no disconnect)", client.State())
	}
}
