package lifecycle

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestUnsupportedProtocolRevisionClosesWithoutAPacket sends ProtocolVersion
// with a revision outside the four the server speaks: the connection closes
// at once and no packet, not even ServerClose, crosses the wire. Each
// supported revision is answered, so the set is exactly those four.
func TestUnsupportedProtocolRevisionClosesWithoutAPacket(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t)
	for _, revision := range []int32{737, 740, 744, 746} {
		t.Run(fmt.Sprintf("supported %d", revision), func(t *testing.T) {
			t.Parallel()
			testsupport.Dial(t, srv.Addr()).SendProtocolVersion(revision)
		})
	}
	for _, revision := range []int32{-2, 0, 1, 736, 738, 741, 745, 747, 746 << 8} {
		t.Run(fmt.Sprintf("unsupported %d", revision), func(t *testing.T) {
			t.Parallel()
			c := testsupport.Dial(t, srv.Addr())
			w := wire.NewPacketWriter(clientpackets.OpcodeProtocolVersion)
			w.WriteInt32(revision)
			if err := wire.WriteFrame(c.Conn(), w.Bytes()); err != nil {
				t.Fatalf("write ProtocolVersion: %v", err)
			}
			c.ExpectClosed()
		})
	}
}

// TestShortSendTimeCheckCountsAsInGameUnderflow sends a time check too short
// for its two ints while in the world: it answers nothing and counts as one
// buffer underflow, which the window tolerates once; the second closes the
// connection with ServerClose as the last frame. A full time check in
// between answers nothing and does not reset the count.
func TestShortSendTimeCheckCountsAsInGameUnderflow(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithAccount("timecheck"), gameservertest.WithCharacter("Ticker", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)

	short := []byte{clientpackets.OpcodeSendTimeCheck, 1, 0, 0, 0}
	full := []byte{clientpackets.OpcodeSendTimeCheck, 1, 0, 0, 0, 2, 0, 0, 0}
	c.Send(full)
	c.Send(short)
	c.Send(full)
	// Still dispatching: the manor list answers.
	c.Send([]byte{clientpackets.OpcodeExtended, byte(clientpackets.OpcodeRequestManorList), 0})
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeExtended, "manor list after one short time check")

	c.Send(short)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeServerClose, "close after the second short time check")
	c.ExpectClosed()
}
