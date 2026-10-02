package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestFrameCharInfoNobleFlag pins CharInfo's noble byte, which follows the
// large crest id: 1 for a noble, 0 otherwise.
func TestFrameCharInfoNobleFlag(t *testing.T) {
	c := &player.Character{Name: "Observed"}
	noble := func() byte {
		got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: &player.Template{}}))
		// The tail after the noble byte: hero, fishing, fishing stance x/y/z,
		// name color, heading, pledge class, pledge type, title color,
		// cursed weapon stage.
		return got[len(got)-39]
	}
	if got := noble(); got != 0 {
		t.Fatalf("noble byte = %d, want 0", got)
	}
	c.SetNoble(true)
	if got := noble(); got != 1 {
		t.Fatalf("noble byte of a noble = %d, want 1", got)
	}
}
