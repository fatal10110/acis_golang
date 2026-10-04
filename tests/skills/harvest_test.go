package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Harvest.java. A player harvesting a Monster corpse is told
// THE_HARVEST_FAILED_BECAUSE_THE_SEED_WAS_NOT_SOWN (893) when it was never
// sown, THE_HARVEST_HAS_FAILED (892) when its crop was taken, and
// YOU_ARE_NOT_AUTHORIZED_TO_HARVEST (891) unless it is the sower or in the
// sower's party (SeedState.isAllowedToHarvest). A harvest that lands earns
// the seed's crop through addEarnedItem, its count times RateDropManor, and
// every other party member hears S1_HARVESTED_S3_S2S (1137) or
// S1_HARVESTED_S2S (1138).

const harvestSkillID = 2098

func harvestSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: harvestSkillID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpseMob, SkillType: "HARVEST",
		CastRange: 600, HitTime: 500, ReuseDelay: 500,
		StaticHitTime: true, StaticReuse: true,
	}
}

// harvestCropID is a suite fixture item the test seed yields as its crop.
const harvestCropID = 20

// harvestSeed is a level 20 seed whose crop is harvestCropID: on a level 20
// monster a harvest yields one crop before RateDropManor.
var harvestSeed = manor.Seed{CropID: harvestCropID, SeedID: 5016, Level: 20, CastleID: 1}

// harvestRate is the scene's RateDropManor.
const harvestRate = 2

func harvestMonsterTemplate() *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 20, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20, Seedable: true,
	}
}

// harvestScene boots the social scene with Caster knowing the harvest skill
// and spawns a monster beside it for Caster to select, sown by sowerID
// unless that is 0, then has a guard kill it.
func harvestScene(t *testing.T, party bool, sowerID func(s *socialScene) int32) (*socialScene, *npc.Hostile) {
	t.Helper()
	s := bootSocialScene(t, harvestSkill(), gameservertest.WithManor(network.ManorConfig{Allowed: true, CropRate: harvestRate}))
	if party {
		s.formParty(t)
	}
	mob := s.srv.SpawnHostileNPCTemplateAt(t, harvestMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	s.quiet(t)
	targetHostile(t, s.caster, mob.ObjectID())
	s.quiet(t)
	if id := sowerID(s); id != 0 && !mob.SeedState().Sow(id, harvestSeed) {
		t.Fatal("fresh monster already sown")
	}
	guard := s.srv.SpawnHostileNPCKindAt(t, "Guard", location.Location{X: hostileX + 40, Y: hostileY, Z: hostileZ})
	if !mob.TakeDamage(1_000_000, guard) {
		t.Fatal("lethal hit did not kill the monster")
	}
	mob.SetCorpseDeadline(time.Now().Add(time.Minute))
	s.quiet(t)
	return s, mob
}

// harvest has Caster cast the harvest skill on its selected corpse and
// returns what Caster and Mate receive.
func harvest(t *testing.T, s *socialScene, mob *npc.Hostile) (casterFrames, mateFrames [][]byte) {
	t.Helper()
	s.caster.Send(encodeRequestMagicSkillUse(harvestSkillID, false, false))
	readCastStartFrames(t, s.caster, s.casterID, harvestSkillID, 1, 500, 500, mob.ObjectID())
	first := s.caster.ReadWithTimeout(2 * time.Second)
	if first == nil {
		t.Fatal("nothing followed the harvest's launch")
	}
	return append([][]byte{first}, collectUntilQuiet(t, s.caster)...), collectUntilQuiet(t, s.mate)
}

// TestPartyMemberHarvestsSowersCrop has Caster harvest the corpse its party
// member Mate sowed: Caster earns the crop, twice over at RateDropManor 2,
// and Mate hears Caster harvested it.
func TestPartyMemberHarvestsSowersCrop(t *testing.T) {
	t.Parallel()
	s, mob := harvestScene(t, true, func(s *socialScene) int32 { return s.mateID })

	casterFrames, mateFrames := harvest(t, s, mob)
	requireLootMessage(t, "Caster", casterFrames, serverpackets.SystemMessageEarnedS2S1S, int32(harvestCropID), int32(harvestRate))
	requireLootMessage(t, "Mate", mateFrames, serverpackets.SystemMessageS1HarvestedS3S2, "Caster", int32(harvestCropID), int32(harvestRate))
	if got := s.srv.PlayerInventory(t, s.casterID).ItemCount(harvestCropID, -1, true); got != harvestRate {
		t.Fatalf("Caster carries %d crops, want %d", got, harvestRate)
	}
	if got := s.srv.PlayerInventory(t, s.mateID).ItemCount(harvestCropID, -1, true); got != 0 {
		t.Fatalf("Mate carries %d crops, want none", got)
	}
}

// TestHarvestRefusals pins Harvest.java's answers to a harvest that takes
// nothing: each is the caster's alone, and no crop moves.
func TestHarvestRefusals(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		party   bool
		sower   func(s *socialScene) int32
		before  func(t *testing.T, s *socialScene, mob *npc.Hostile)
		message int
	}{
		{"never sown", true, func(*socialScene) int32 { return 0 }, nil, serverpackets.SystemMessageHarvestFailedSeedNotSown},
		{"sown by a stranger", false, func(s *socialScene) int32 { return s.mateID }, nil, serverpackets.SystemMessageNotAuthorizedToHarvest},
		{"crop already taken", true, func(s *socialScene) int32 { return s.casterID }, func(t *testing.T, s *socialScene, mob *npc.Hostile) {
			if claim := mob.SeedState().ClaimHarvest(s.casterID, nil); claim != npc.HarvestClaimed {
				t.Fatalf("first claim = %v, want claimed", claim)
			}
		}, serverpackets.SystemMessageHarvestHasFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, mob := harvestScene(t, tc.party, tc.sower)
			if tc.before != nil {
				tc.before(t, s, mob)
			}
			casterFrames, mateFrames := harvest(t, s, mob)
			requireLootMessage(t, "Caster", casterFrames, tc.message)
			if len(decodeLootMessages(mateFrames)) != 0 {
				t.Fatalf("Mate heard %+v, want nothing", decodeLootMessages(mateFrames))
			}
			if got := s.srv.PlayerInventory(t, s.casterID).ItemCount(harvestCropID, -1, true); got != 0 {
				t.Fatalf("Caster carries %d crops, want none", got)
			}
		})
	}
}
