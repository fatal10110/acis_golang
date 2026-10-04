package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestFishingStanceInUserInfoAndCharInfo pins the fishing flag and the bait
// location both packets carry after the hero byte: the flag follows the
// fishing state and the location the line, each on its own.
func TestFishingStanceInUserInfoAndCharInfo(t *testing.T) {
	c := &player.Character{Name: "Fisher"}
	tails := func() (user, char []byte) {
		u := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: &player.Template{}}))
		ch := framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: &player.Template{}}))
		// UserInfo ends with name color, running, pledge class, pledge
		// type, title color and cursed weapon stage after the bait z;
		// CharInfo with name color, heading, pledge class, pledge type,
		// title color and cursed weapon stage.
		return u[len(u)-34 : len(u)-21], ch[len(ch)-37 : len(ch)-24]
	}
	idle := make([]byte, 13)
	if u, ch := tails(); !bytes.Equal(u, idle) || !bytes.Equal(ch, idle) {
		t.Fatalf("idle fishing fields = %x / %x, want zeros", u, ch)
	}
	c.SetFishing(true)
	c.SetFishingBait(location.Location{X: 260, Y: -20, Z: 10})
	want := []byte{0x01, 0x04, 0x01, 0x00, 0x00, 0xec, 0xff, 0xff, 0xff, 0x0a, 0x00, 0x00, 0x00}
	if u, ch := tails(); !bytes.Equal(u, want) || !bytes.Equal(ch, want) {
		t.Fatalf("fishing fields = %x / %x, want %x", u, ch, want)
	}
}
