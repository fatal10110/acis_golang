package loginserver

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

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

// partialWriteConn passes the first partial bytes of its first Write to the
// pipe and then fails it, like a write that timed out mid-frame; it counts
// every Write and never touches the pipe again.
type partialWriteConn struct {
	net.Conn
	partial int

	mu     sync.Mutex
	writes int
}

func (c *partialWriteConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writes++
	first := c.writes == 1
	c.mu.Unlock()
	if !first {
		return 0, errors.New("write after a failed write")
	}
	n, _ := c.Conn.Write(p[:c.partial])
	return n, os.ErrDeadlineExceeded
}

func (c *partialWriteConn) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes
}

// A write that fails partway through a frame closes the connection at
// once: no requested or handler final packet may follow the partial frame.
func TestClientConnFailedWriteClosesWithoutFinalPacket(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	crypt, err := logincrypt.NewLoginCrypt(testSessionKey)
	if err != nil {
		t.Fatalf("NewLoginCrypt: %v", err)
	}
	conn := &partialWriteConn{Conn: server, partial: 3}
	c := &clientConn{conn: conn, crypt: crypt}

	sent := make(chan error, 1)
	go func() { sent <- c.send([]byte{0x04, 0x01, 0x02}) }()

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	partial := make([]byte, conn.partial)
	if _, err := io.ReadFull(client, partial); err != nil {
		t.Fatalf("read partial frame: %v", err)
	}
	if err := <-sent; !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("send = %v, want the write's timeout", err)
	}
	if n, err := client.Read(make([]byte, 1)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("after the failed write: n=%d err=%v, want EOF", n, err)
	}

	c.requestClose([]byte{0x01, 0xaa})
	c.closeWith([]byte{0x01, 0xbb})
	if err := c.send([]byte{0x04}); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("send after the failed write = %v, want net.ErrClosed", err)
	}
	if n := conn.writeCount(); n != 1 {
		t.Fatalf("writes = %d, want only the failed one", n)
	}
}
