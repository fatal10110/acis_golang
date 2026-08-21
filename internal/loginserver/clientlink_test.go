package loginserver

import (
	"net"
	"testing"

	commoncrypt "github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	logincrypt "github.com/fatal10110/acis_golang/internal/loginserver/crypt"
)

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
