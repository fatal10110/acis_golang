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
// TestFrameUserAndCharInfoCarryDuelTeam pins the team byte UserInfo and
// CharInfo write: a duellist's side colour (BLUE 1, RED 2), alone in the
// payload changing with it. Spawn protection still shows BLUE in both.
func TestFrameUserAndCharInfoCarryDuelTeam(t *testing.T) {
	tmpl := &player.Template{}
	c := &player.Character{}
	c.SetName("M")
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
	c.SetDuelTeam(duel.TeamRed)
	protected = framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl, SpawnProtectedTeam: true}))
	c.SetDuelTeam(duel.TeamBlue)
	blue = framePayload(t, FrameCharInfo(CharInfoSnapshot{Character: c, Template: tmpl}))
	if !bytes.Equal(protected, blue) {
		t.Fatal("a spawn-protected duellist's CharInfo differs from a blue one's, want BLUE shown")
	}
}
