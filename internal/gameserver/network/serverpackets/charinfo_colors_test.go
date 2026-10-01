package serverpackets

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestFrameCharInfoColors pins CharInfo's name and title colors to the
// character's, the shipped defaults until something sets them.
func TestFrameCharInfoColors(t *testing.T) {
	c := &player.Character{Name: "Observer"}
	colors := func() (uint32, uint32) {
		got := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: &player.Template{}}))
		n := len(got)
		// The tail: name color, heading, pledge class, pledge type, title
		// color, cursed weapon stage.
		return binary.LittleEndian.Uint32(got[n-24:]), binary.LittleEndian.Uint32(got[n-8:])
	}
	if name, title := colors(); name != 0xFFFFFF || title != 0xFFFF77 {
		t.Fatalf("default colors = %#x %#x, want 0xffffff 0xffff77", name, title)
	}
	c.SetColors(0x00CCFF, 0x339933)
	if name, title := colors(); name != 0x00CCFF || title != 0x339933 {
		t.Fatalf("colors = %#x %#x, want 0xccff 0x339933", name, title)
	}
}
