package clan

import (
	"slices"
	"testing"
	"time"
)

// goneClanID holds a Rainbow Springs registration but is no clan.
const goneClanID int32 = 0x7f0011ff

// TestRainbowSpringsPassesOverSurplusClanWithLeaderAway pins the parity
// exception of #5022: with more than four clans registered, a clan left
// over whose leader is out of the world, and one that no longer exists,
// are passed over without a word, and the choice goes on. Springers,
// Bathers, Soakers and Waders attack; the Attacker is told the
// registrations ended and Springers is registered; an hour later the game
// starts.
func TestRainbowSpringsPassesOverSurplusClanWithLeaderAway(t *testing.T) {
	t.Parallel()
	fifthLeader := rainbowMember{castleRival: castleRival{0x7f001011, "fifth", "FifthLeader", fifthClanID}, leads: true}
	w := bootRainbow(t, rainbowSetup{
		date: 2 * time.Hour,
		decrees: map[int32]int{
			springersClanID: 9, bathersClanID: 5, soakersClanID: 3, waders4ClanID: 2, fifthClanID: 1, goneClanID: 1,
		},
		members: []rainbowMember{fifthLeader},
	})

	w.srv.Advance(t, time.Hour)
	drainFrames(t, w.c)
	w.srv.Advance(t, time.Hour)
	w.srv.Advance(t, 0)
	if got := w.siege.Attackers(); !slices.Equal(got, []int32{springersClanID, bathersClanID, soakersClanID, waders4ClanID}) {
		t.Fatalf("attackers after the choice = %#x, want Springers, Bathers, Soakers, Waders", got)
	}
	if got := sysMsgIDs(t, drainFrames(t, w.c)); !slices.Equal(got, []string{"1823", "1824"}) {
		t.Fatalf("the Attacker got %v, want CLANHALL_WAR_REGISTRATION_PERIOD_ENDED, REGISTERED_FOR_CLANHALL_WAR", got)
	}

	w.srv.Advance(t, time.Hour)
	w.srv.Advance(t, 0)
	if !w.siege.InProgress() {
		t.Fatal("the game did not start")
	}
}
