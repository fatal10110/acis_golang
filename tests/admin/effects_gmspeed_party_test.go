package admin

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestAdminEffectsGMSpeedParty pins the icon pass after //gmspeed in a
// party: each party member is shown the GM's icon list, Super Haste on
// after a level, gone after 0, and shown again after a usage or a level
// that puts nothing on.
func TestAdminEffectsGMSpeedParty(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, effectsSkills(t))
	gm := srv.Client
	enterWorld(t, gm)
	user, _ := addPlayer(t, srv, "player2", "Player", userLevel)
	user.Send(encodeJoinParty("Admin", 0))
	drain(t, gm)
	gm.Send(encodeAnswerJoinParty(1))
	drain(t, gm)
	drain(t, user)
	// spelled returns the skill levels of the last PartySpelled of the GM
	// frames show, failing without one.
	spelled := func(what string, frames [][]byte) map[int32]int {
		t.Helper()
		var icons map[int32]int
		for _, f := range frames {
			if f[0] != serverpackets.OpcodePartySpelled {
				continue
			}
			r := wire.NewReader(f[1:])
			if r.ReadInt32() != 0 || r.ReadInt32() != gmID {
				continue
			}
			icons = map[int32]int{}
			for range r.ReadInt32() {
				id, level := r.ReadInt32(), r.ReadUint16()
				r.ReadInt32()
				icons[id] = int(level)
			}
		}
		if icons == nil {
			t.Fatalf("%s member frames = %x, want the GM's PartySpelled", what, testsupport.FrameOpcodes(frames))
		}
		return icons
	}

	for _, tc := range []struct {
		command string
		level   int
	}{{"gmspeed 2", 2}, {"gmspeed 0", 0}, {"gmspeed x", 0}, {"gmspeed 5", 0}} {
		exchange(t, gm, encodeBuildCmd(tc.command))
		if got := spelled("//"+tc.command, settle(t, user))[effectsHaste]; got != tc.level {
			t.Fatalf("//%s member sees haste level %d, want %d", tc.command, got, tc.level)
		}
	}
}
