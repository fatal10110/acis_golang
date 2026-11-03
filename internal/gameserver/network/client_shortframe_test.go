package network

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// lockedLog is a log sink the server's read goroutine writes while the test
// reads it.
type lockedLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *lockedLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *lockedLog) lines() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Split(strings.TrimSpace(w.buf.String()), "\n")
}

// TestPreAuthShortFrameClosesWithServerClose drives a buffer-underflow frame
// through the live read loop before authentication: the first short frame
// closes the connection with ServerClose as its last frame, and the decode
// failure is logged once as a warning carrying the decoder's error.
func TestPreAuthShortFrameClosesWithServerClose(t *testing.T) {
	var logged lockedLog
	addr, _, _, _ := newTestGameClientLinkWithLog(t, nil, nil, zerolog.New(&logged))
	c := testsupport.Dial(t, addr)
	c.SendProtocolVersion(746)

	short := encodeSingleOpcode(clientpackets.OpcodeAuthLogin)
	c.Send(short)
	if got := c.Read(); got[0] != serverpackets.OpcodeServerClose {
		t.Fatalf("close opcode = %#x, want ServerClose (%#x)", got[0], serverpackets.OpcodeServerClose)
	}
	c.ExpectClosed()

	_, decodeErr := clientpackets.DecodeAuthLogin(short)
	if decodeErr == nil {
		t.Fatal("DecodeAuthLogin(opcode only) succeeded; the fixture must underflow")
	}
	quotedErr, err := json.Marshal(decodeErr.Error())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"level":"warn","error":` + string(quotedErr) + `,"message":"game client"}`
	var found int
	for _, line := range logged.lines() {
		if strings.Contains(line, `"message":"game client"`) {
			found++
			if line != want {
				t.Errorf("decode log = %s, want %s", line, want)
			}
		}
	}
	if found != 1 {
		t.Errorf("decode log lines = %d, want 1; log:\n%s", found, strings.Join(logged.lines(), "\n"))
	}
}
