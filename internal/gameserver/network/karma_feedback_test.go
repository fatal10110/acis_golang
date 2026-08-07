package network

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSendKarmaChangeFramesOrdersMessageBeforeStatus pins the reference's
// order: SystemMessage(YOUR_KARMA_HAS_BEEN_CHANGED_TO_S1) before
// StatusUpdate(KARMA), matching Player.setKarma (Player.java:1076-1080).
func TestSendKarmaChangeFramesOrdersMessageBeforeStatus(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)

	sendKarmaChangeFrames(live, 240)

	sent := frames.frames
	if len(sent) != 2 {
		t.Fatalf("frames sent = %d, want 2", len(sent))
	}
	if sent[0][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("frame[0] opcode = %#x, want SystemMessage (%#x)", sent[0][0], serverpackets.OpcodeSystemMessage)
	}
	if sent[1][0] != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("frame[1] opcode = %#x, want StatusUpdate (%#x)", sent[1][0], serverpackets.OpcodeStatusUpdate)
	}
}
