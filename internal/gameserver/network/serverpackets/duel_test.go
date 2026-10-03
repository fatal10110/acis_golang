package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// The fixtures follow the duel packets' writeImpl in the reference
// (ExDuelAskStart, ExDuelReady, ExDuelStart, ExDuelEnd and
// ExDuelUpdateUserInfo): 0xfe, the little-endian sub-opcode, then each
// field as written, a string as UTF-16LE closed by a zero character.
func TestDuelPacketBytes(t *testing.T) {
	cases := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"ExDuelAskStart party", framePayload(t, FrameExDuelAskStart("Ab", true)), []byte{
			0xfe, 0x4b, 0x00,
			'A', 0, 'b', 0, 0, 0,
			1, 0, 0, 0,
		}},
		{"ExDuelAskStart one on one", framePayload(t, FrameExDuelAskStart("Ab", false)), []byte{
			0xfe, 0x4b, 0x00,
			'A', 0, 'b', 0, 0, 0,
			0, 0, 0, 0,
		}},
		{"ExDuelReady", framePayload(t, FrameExDuelReady(false)), []byte{0xfe, 0x4c, 0x00, 0, 0, 0, 0}},
		{"ExDuelStart", framePayload(t, FrameExDuelStart(true)), []byte{0xfe, 0x4d, 0x00, 1, 0, 0, 0}},
		{"ExDuelEnd", framePayload(t, FrameExDuelEnd(true)), []byte{0xfe, 0x4e, 0x00, 1, 0, 0, 0}},
		{"ExDuelUpdateUserInfo", framePayload(t, FrameExDuelUpdateUserInfo(DuelUserInfo{
			Name: "Ab", ObjectID: 0x10203040, ClassID: 88, Level: 76,
			HP: 1, MaxHP: 0x0102, MP: 3, MaxMP: 4, CP: 5, MaxCP: 6,
		})), []byte{
			0xfe, 0x4f, 0x00,
			'A', 0, 'b', 0, 0, 0,
			0x40, 0x30, 0x20, 0x10,
			88, 0, 0, 0,
			76, 0, 0, 0,
			1, 0, 0, 0,
			0x02, 0x01, 0, 0,
			3, 0, 0, 0,
			4, 0, 0, 0,
			5, 0, 0, 0,
			6, 0, 0, 0,
		}},
	}
	for _, tc := range cases {
		if !bytes.Equal(tc.got, tc.want) {
			t.Errorf("%s = % x, want % x", tc.name, tc.got, tc.want)
		}
	}
}

// TestFrameUserAndCharInfoCarryDuelTeam pins the team byte UserInfo and
// CharInfo write: a duellist's side colour (BLUE 1, RED 2), alone in the
// payload changing with it. Spawn protection still shows BLUE in UserInfo.
func TestFrameUserAndCharInfoCarryDuelTeam(t *testing.T) {
	tmpl := &player.Template{}
	c := &player.Character{Name: "M"}
	encode := map[string]func() []byte{
		"UserInfo": func() []byte { return framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl})) },
		"CharInfo": func() []byte { return framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl})) },
	}
	for name, frame := range encode {
		c.SetDuelTeam(duel.TeamNone)
		none := frame()
		c.SetDuelTeam(duel.TeamRed)
		red := frame()
		if len(none) != len(red) {
			t.Fatalf("%s lengths differ: %d vs %d", name, len(none), len(red))
		}
		at := -1
		for i := range none {
			if none[i] != red[i] {
				if at >= 0 {
					t.Fatalf("%s: more than one byte follows the team", name)
				}
				at = i
			}
		}
		if at < 0 {
			t.Fatalf("%s: no byte follows the team", name)
		}
		if none[at] != 0 || red[at] != 2 {
			t.Fatalf("%s team byte at %d: none %d red %d, want 0 and 2", name, at, none[at], red[at])
		}
	}
	c.SetDuelTeam(duel.TeamRed)
	protected := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl, SpawnProtectedTeam: true}))
	c.SetDuelTeam(duel.TeamBlue)
	blue := framePayload(t, FrameUserInfo(UserInfoSnapshot{Character: c, Template: tmpl}))
	if !bytes.Equal(protected, blue) {
		t.Fatal("a spawn-protected duellist's UserInfo differs from a blue one's, want BLUE shown")
	}
}
