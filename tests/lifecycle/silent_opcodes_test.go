package lifecycle

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSilentOpcodesKeepTheSession sends, in the world, ten frames of each
// opcode the client may send without asking for anything: two no request is
// built for, and two that read a field and do nothing. None is answered or
// counted as an unknown packet, which would end the session at the sixth, so
// the next request's answer is the first frame the client reads.
func TestSilentOpcodesKeepTheSession(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	for _, frame := range [][]byte{
		{clientpackets.OpcodeRequestDismissParty},
		{clientpackets.OpcodeSnoopQuit, 0, 0, 0, 0},
		{clientpackets.OpcodeMessengerDialog},
		{clientpackets.OpcodeExtended, 0x20, 0x00, 0x00, 0x00},
	} {
		for range 10 {
			c.Send(frame)
		}
	}
	c.Send(encodeRequestItemList())
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeItemList, "ItemList after the silent opcodes")
}

// TestRejectedOpcodeLogLineIsBounded sends an almost maximal frame of an
// opcode no state accepts before authentication: the connection closes as
// for any rejected opcode (ServerClose, then the close), and its log line
// names the opcode, the state and the frame's length but shows only the head
// of the frame.
func TestRejectedOpcodeLogLineIsBounded(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCapturedLog())
	probe := testsupport.Dial(t, srv.Addr())
	frame := make([]byte, 60000)
	frame[0] = 0xfe
	if err := wire.WriteFrame(probe.Conn(), frame); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	closing, err := wire.ReadFrame(probe.Conn())
	if err != nil {
		t.Fatalf("read ServerClose: %v", err)
	}
	assertFrameOpcode(t, closing, serverpackets.OpcodeServerClose, "ServerClose")
	probe.ExpectClosed()

	var line string
	for _, l := range strings.Split(srv.LogText(), "\n") {
		if strings.Contains(l, `"Accept opcode"`) {
			line = l
		}
	}
	if line == "" {
		t.Fatal("no Accept opcode log line")
	}
	for _, want := range []string{`"opcode":"0xfe"`, `"state":"connected"`, `"length":60000`, `"head":"fe000000000000000000000000000000"`} {
		if !strings.Contains(line, want) {
			t.Errorf("log line %q lacks %s", line, want)
		}
	}
	if len(line) > 256 {
		t.Errorf("log line is %d bytes, want at most 256", len(line))
	}
}
