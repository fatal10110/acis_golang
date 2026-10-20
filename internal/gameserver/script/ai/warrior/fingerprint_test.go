package warrior

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// npcStrings are the NpcString ids the classes say: a named constant in the
// reference, a number in Go.
var npcStrings = []int{1000286, 1000287, 1000288, 1000290, 1000291, 1000292, 1000294, 1000307, 1000381, 1000382, 1000383, 1000384, 1000385, 1000386, 1000387, 1000388, 1000389, 1000390, 1000391, 1000392, 1000393, 1000394, 1000395, 1000396, 1000397, 1000398, 1000399, 1000400, 1000401, 1000402, 1000403, 1000404, 1000405, 1000406, 1000407, 1000408, 1000409, 1000410, 1000411, 1000412, 1000413, 1000414, 1000427, 1000428, 1000452, 1800011}

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The panic of a berserker
// whose template lists no fury skill stands for the reference's null
// dereference; the tyrannosaurus's failed capture message is a named
// constant in the reference, a number in Go.
func TestMatchesReferenceFingerprints(t *testing.T) {
	exempt := []string{`string "warrior: a berserker's template lists no fury skill"`, "number 2002"}
	for _, id := range npcStrings {
		exempt = append(exempt, fmt.Sprintf("number %d", id))
	}
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.Monster.WarriorBase.WarriorBase",
		"script.ai.individual.Monster.WarriorBase.Warrior.Warrior",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.WarriorHold",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFishingBlock",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarrior",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateWarrior",
		"script.ai.individual.Monster.WarriorBase.Chests.TreasureChest",
		"script.ai.individual.Monster.WarriorBase.Chests.TreasureChestMimic",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderSplit",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderSplitAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserker",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecialAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecialStoneAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBomb",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBombAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorTeleportBombAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchant1Of4",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchant1Of4Aggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseGhostPhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseGhostPhysicalSpecialAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushBomb",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb2",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBossBasic",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss1",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss2",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss4",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss5",
		"script.ai.individual.Monster.WarriorBase.Warrior.TyrannoPrimeval.TyrannoPrimeval",
		"script.ai.individual.Monster.WarriorBase.Warrior.TyrannoPrimeval.TyrannoPrimevalWalker",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecialAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecialBuffAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantPhysicalPoison",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantPhysicalPoisonAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastDDMagicEnchantPhysicalPoisonAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantRangePhysicalPoisonAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.CorePrivatePremo",
		"script.ai.individual.Monster.WarriorBase.Warrior.RamphoPrimeval",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushStrongMan1",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushStrongMan2",
		"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushWarriorPhysicalSpecial2",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressiveImmediate",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast2DebuffsAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast2SkillsApproachAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSelfRangeDDMagicAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSelfRangeDDMagicBuffAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseNecroLongAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorDisguise",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorFlee",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorRunAwayPhysicalSpecialGolemAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorTimerBomb",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorUseBow",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorUseBowNotFleeAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.CorePrivateWarriorAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.Medusa",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagic",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.Agit02DoomServantAgit",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.CorePrivateWarriorCastDDMagicAggressive.CorePrivateWarriorCastDDMagicAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.CorePrivateWarriorCastDDMagicAggressive.CorePrivatePerum",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicSelfBuff",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicSelfBuffAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurse",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.Agit02DoomGuardAgit",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.Agit02DoomKnightAgitParty",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.Agit02VampireAgitParty",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorHereticSelfRangePhysical",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorHereticSelfRangePhysicalAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.WarriorCastingCurseAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.Agit02DoomKnightAgit",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.CorePrivateDicor",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.SpecialBloodyQueen",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchant",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClan",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClanAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgit",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgitParty",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.OrfenPrivateRaikel",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomWarriorAgit",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantSelfAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.ZakenPrivateDollBlader",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorPhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorStone",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.SaintNinja",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampire",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireMobHate",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderBombSpawn",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDD",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDMagicStoneAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCouplePhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrol",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummon",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummonAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarriorAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaGardener",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaPet",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaSecretaryLeader",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCannibaliAggressiva",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastCurseAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.FreyaBear",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.OrfenPrivateRaikelLeos",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCannibaliAggressive",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCast3SkillsMagical",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastCurse",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastDDHeal",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastDDMagicCurseStone",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateHereticCastCurse",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastEnchant",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastHeal",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastHealCurse",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCouplePhysicalSpecial",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoward",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePabelgolem",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalStone",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePriest",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePriestBuffer",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSlowType2",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSlowTypeMagic",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSplit",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.QueenAntPrivateRoyalGuardAnt",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.RoyalRushPartyPrivateBomb",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.RoyalRushPartyPrivateHate",
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoupleFollower.PartyPrivateCoupleFollower",
	}, exempt...)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
