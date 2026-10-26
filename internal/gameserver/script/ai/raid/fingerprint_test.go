package raid

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The reference names the
// gigantic golem's lines by their NpcString constants, which the port
// writes as ids, and Sailren's party read
// (Player.PartyMembers) shares its name with the quest helper.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.individual.Monster.RaidBoss.RaidBossStandard",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Gordon",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossAloneSummonPrivate",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossForTeleportDungeon",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Sailren",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.RaidBossType1Aggressive",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.DrChaosGiganticGolem",
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.FreyaRoyalGuard",
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossParty",
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType2",
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.IceFairySirra",
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3",
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3AnaisAggressive",
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4",
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4AndreasAggressive",
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType5",
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateStandard",
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateArcher",
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateFighter",
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateHealer",
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateWizard",
	}, "number 1010582", "number 1010583", "number 1010643", "number 1121002", "call Quest.getPartyMembers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
