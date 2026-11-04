package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// TestFrameExShowQuestMark pins the quest mark's rendering against the line
// the engine-contract goldens write for it; its bytes are pinned by the
// ExShowQuestMark rows of testdata/packets.golden.
func TestFrameExShowQuestMark(t *testing.T) {
	line, err := scriptcontract.Packet(framePayload(t, FrameExShowQuestMark(258)), nil)
	if err != nil || line != "S ExShowQuestMark quest=258" {
		t.Fatalf("rendered = %q, %v", line, err)
	}
}
