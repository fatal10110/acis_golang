package loginserver

import (
	"errors"
	"net"
	"testing"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	logincrypt "github.com/fatal10110/acis_golang/internal/loginserver/crypt"
)

// ---- from clientlink_test.go ----
func TestClientConnSendRejectsOversizedPayloadBeforeFirstEncryption(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})

	crypt, err := logincrypt.NewLoginCrypt([]byte("0123456789abcdef"))
	if err != nil {
		t.Fatalf("NewLoginCrypt: %v", err)
	}
	c := &clientConn{conn: server, crypt: crypt}
	if err := c.send(make([]byte, wire.MaxFrameLength)); err == nil {
		t.Fatal("send(oversized) error = nil, want frame length error")
	}

	sent := make(chan error, 1)
	go func() { sent <- c.send([]byte{0x01}) }()

	payload, err := wire.ReadFrame(client)
	if err != nil {
		t.Fatalf("read first frame: %v", err)
	}
	if want := commoncrypt.PaddedSize(1 + 8); len(payload) != want {
		t.Fatalf("first encrypted payload length = %d, want %d", len(payload), want)
	}
	if err := <-sent; err != nil {
		t.Fatalf("send(first) error = %v", err)
	}
}

// A requested close wins over anything the handler sends or closes with
// afterwards: the requested packet is the last frame, then EOF.
func TestClientConnRequestedCloseReplacesLaterWrites(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	crypt, err := logincrypt.NewLoginCrypt(testSessionKey)
	if err != nil {
		t.Fatalf("NewLoginCrypt: %v", err)
	}
	cipher, err := commoncrypt.NewBlowfishCipher(testSessionKey)
	if err != nil {
		t.Fatalf("NewBlowfishCipher: %v", err)
	}
	c := &clientConn{conn: server, crypt: crypt}

	// The first frame is the Init, under the static-key scheme; discard it.
	go func() { _ = c.send([]byte{0x00}) }()
	if _, err := wire.ReadFrame(client); err != nil {
		t.Fatalf("read Init frame: %v", err)
	}

	// The pipe has no reader now: requestClose must not write.
	if !c.requestClose([]byte{0x01, 0xaa}) {
		t.Fatal("first requestClose = false")
	}
	if c.requestClose([]byte{0x01, 0xbb}) {
		t.Fatal("second requestClose = true, want the first request to win")
	}

	sent := make(chan error, 1)
	go func() {
		err := c.send([]byte{0x04})
		c.closeWith([]byte{0x06})
		sent <- err
	}()

	payload, err := wire.ReadFrame(client)
	if err != nil {
		t.Fatalf("read final frame: %v", err)
	}
	commoncrypt.DecryptBlocks(cipher, payload)
	if payload[0] != 0x01 || payload[1] != 0xaa {
		t.Fatalf("final frame starts % X, want the requested packet 01 AA", payload[:2])
	}
	if _, err := wire.ReadFrame(client); err == nil {
		t.Fatal("frame after the requested final packet")
	}
	if err := <-sent; !errors.Is(err, net.ErrClosed) {
		t.Fatalf("send after requestClose = %v, want net.ErrClosed", err)
	}
}
