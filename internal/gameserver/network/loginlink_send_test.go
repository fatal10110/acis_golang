package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/crypt"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// TestLoginLinkOversizedSendKeepsLinkOpen sends a payload too large for a
// link frame: the send fails, and the shared link stays open for every
// other client's messages.
func TestLoginLinkOversizedSendKeepsLinkOpen(t *testing.T) {
	conn := newWriteFailBlockingReadConn()
	l := &LoginLink{
		conn:   conn,
		crypt:  crypt.NewLinkCrypt(),
		log:    zerolog.Nop(),
		done:   make(chan struct{}),
		frames: wire.NewFrameReader(conn),
	}
	go l.readLoop(LoginLinkHandlers{})
	t.Cleanup(func() { l.Close() })

	if err := l.send(make([]byte, wire.MaxFrameLength)); err == nil {
		t.Fatal("send of an oversized payload: want error, got nil")
	}
	select {
	case <-conn.closed:
		t.Fatal("oversized send closed the link")
	default:
	}
}
