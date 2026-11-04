package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestUserInfoSiegeRelation pins UserInfo's relation field
// (UserInfo.java): 0x40 for a clan leader, or-ed with 0x180 for a siege
// attacker and 0x80 for a defender.
func TestUserInfoSiegeRelation(t *testing.T) {
	for _, tc := range []struct {
		leader bool
		state  int32
		want   int32
	}{
		{false, player.SiegeStateNone, 0},
		{true, player.SiegeStateNone, 0x40},
		{false, player.SiegeStateAttacker, 0x180},
		{true, player.SiegeStateAttacker, 0x1c0},
		{false, player.SiegeStateDefender, 0x80},
		{true, player.SiegeStateDefender, 0xc0},
	} {
		if got := userInfoRelation(tc.leader, tc.state); got != tc.want {
			t.Errorf("userInfoRelation(%t, %d) = %#x, want %#x", tc.leader, tc.state, got, tc.want)
		}
	}
}
