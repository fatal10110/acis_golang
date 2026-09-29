package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestOwnerFollowRangeIsStrict pins the 2000-unit follow-toggle leash: an
// owner exactly 2000 units away is already out of range.
func TestOwnerFollowRangeIsStrict(t *testing.T) {
	for _, tc := range []struct {
		dx   int
		want bool
	}{
		{1999, true},
		{2000, false},
	} {
		a, _, _, _ := newAvoidFixture(t, location.Location{X: 1000 + tc.dx, Y: 1000})
		if got := a.ownerWithinFollowRange(); got != tc.want {
			t.Fatalf("owner %d units away: ownerWithinFollowRange() = %v, want %v", tc.dx, got, tc.want)
		}
	}
}

// TestPetKillRewardOwnerRangeIsStrict pins the pet's share of a kill reward
// to an owner strictly inside the party range.
func TestPetKillRewardOwnerRangeIsStrict(t *testing.T) {
	const partyRange = 1600
	for _, tc := range []struct {
		dx   int
		want bool
	}{
		{partyRange - 1, true},
		{partyRange, false},
	} {
		owner := &avoidOwner{fakeSummonOwner: fakeSummonOwner{id: 42}, x: 1000, y: 1000}
		pet := mustPet(t, PetConfig{
			ObjectID: 5,
			Owner:    owner,
			Level:    1,
			Growth:   &npc.PetData{Levels: map[int]npc.PetLevelStats{petMaxLevel: {MaxExp: 1_000_000}}},
			Stats:    CombatStats{MaxHP: 100, MaxMP: 100},
			Roll:     zeroSummonRoll,
		})
		SpawnBesideOwner(world.New(), pet, owner, location.Location{X: tc.dx})
		if x, y, _ := pet.Position(); x != 1000+tc.dx || y != 1000 {
			t.Fatalf("pet at (%d, %d), want (%d, 1000)", x, y, 1000+tc.dx)
		}
		if got := pet.CanReceiveKillReward(partyRange); got != tc.want {
			t.Fatalf("owner %d units away: CanReceiveKillReward(%d) = %v, want %v", tc.dx, partyRange, got, tc.want)
		}
	}
}
