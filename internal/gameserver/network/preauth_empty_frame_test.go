package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// TestPreAuthEmptyFramesCountTowardCap sends an unauthenticated connection
// only frames with no body, one a full header and one a header shorter than
// itself: each is skipped unanswered but counts toward the pre-auth packet
// cap, so they cannot hold the connection open, and the one over the cap
// closes it with ServerClose as its last frame.
func TestPreAuthEmptyFramesCountTowardCap(t *testing.T) {
	addr, _, _, _ := newTestGameClientLinkWithLog(t, nil, nil, zerolog.Nop())
	c := testsupport.Dial(t, addr)
	c.SendProtocolVersion(746) // processed packet 1

	c.Send(nil) // processed packet 2
	// processed packet 3
	if _, err := c.Conn().Write([]byte{0x01, 0x00}); err != nil {
		t.Fatalf("write short header: %v", err)
	}
	c.Send(nil) // processed packet 4: over the cap
	if got := c.Read(); got[0] != serverpackets.OpcodeServerClose {
		t.Fatalf("first frame after empty frames: opcode = %#x, want ServerClose (%#x)", got[0], serverpackets.OpcodeServerClose)
	}
	c.ExpectClosed()
}
