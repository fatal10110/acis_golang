// Package linkpeer is a scripted login server for the game server's link:
// it completes the handshake, confirms the registration, and hands a test
// every frame the game server sends afterwards, so a test sees the bytes on
// the wire instead of their effect on a real login server.
package linkpeer

import (
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/link"
)

// frameTimeout bounds each wait for a frame.
const frameTimeout = 10 * time.Second

// Peer is one scripted login server accepting one game server connection.
type Peer struct {
	ln     net.Listener
	frames chan []byte
	// done ends the serving goroutine at the test's cleanup, even while
	// frames nobody reads fill the queue.
	done chan struct{}
}

// Start listens on an ephemeral port and answers the first connection: it
// sends InitLS, takes the game server's key, answers its GameServerAuth with
// AuthResponse (serverID, name), then queues every later frame.
func Start(t testing.TB, serverID byte, name string) *Peer {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("linkpeer: GenerateKey: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("linkpeer: listen: %v", err)
	}
	p := &Peer{ln: ln, frames: make(chan []byte, 64), done: make(chan struct{})}
	t.Cleanup(func() {
		close(p.done)
		_ = ln.Close()
	})
	go p.serve(priv, serverID, name)
	return p
}

// Addr is the address the game server dials.
func (p *Peer) Addr() string { return p.ln.Addr().String() }

// Next returns the next frame the game server sent after registering,
// decrypted, with the checksum and padding the link adds still on its end.
// It fails the test when none arrives in time.
func (p *Peer) Next(t testing.TB) []byte {
	t.Helper()
	select {
	case frame, ok := <-p.frames:
		if !ok {
			t.Fatal("linkpeer: the link closed before the next frame")
		}
		return frame
	case <-time.After(frameTimeout):
		t.Fatal("linkpeer: no frame from the game server")
		return nil
	}
}

func (p *Peer) serve(priv *rsa.PrivateKey, serverID byte, name string) {
	defer close(p.frames)
	conn, err := p.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	bootstrap := crypt.NewLinkCrypt()
	if wire.WriteFrame(conn, bootstrap.Encrypt(link.EncodeInitLS(priv.PublicKey.N.Bytes()))) != nil {
		return
	}
	payload, err := wire.ReadFrame(conn)
	if err != nil || bootstrap.Decrypt(payload) != nil {
		return
	}
	key, err := link.DecodeBlowFishKey(payload, priv)
	if err != nil {
		return
	}
	session := crypt.NewLinkCrypt()
	if session.SetKey(key) != nil {
		return
	}
	if payload, err = wire.ReadFrame(conn); err != nil || session.Decrypt(payload) != nil {
		return
	}
	if wire.WriteFrame(conn, session.Encrypt(link.EncodeAuthResponse(serverID, name))) != nil {
		return
	}
	for {
		payload, err := wire.ReadFrame(conn)
		if err != nil || session.Decrypt(payload) != nil {
			return
		}
		select {
		case p.frames <- payload:
		case <-p.done:
			return
		}
	}
}
