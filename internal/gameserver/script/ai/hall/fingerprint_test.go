package hall

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The reference names the
// shouts and lines by their NpcString constants and Dietrich's healer and
// buffer classes by their class id constants; the port writes their ids
// (30, the Elven Elder's, is a literal of the reference already). The game
// managers' slowing area names are never read (their area is left out),
// and the party reads (Player.PartyMembers) share their name with the quest
// helper.
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.siegablehall.Agit01PartisanSorcerer1",
		"script.ai.siegablehall.Agit02Dietrich",
		"script.ai.siegablehall.Agit02Gustav",
		"script.ai.siegablehall.Agit02Mikhail",
		"script.ai.siegablehall.Agit03Alfred",
		"script.ai.siegablehall.Agit03Giselle",
		"script.ai.siegablehall.Agit03Lidia",
		"script.ai.siegablehall.AzitWateringCrazyYeti",
		"script.ai.siegablehall.AzitWateringGroud",
		"script.ai.siegablehall.AzitWateringMimic",
		"script.ai.siegablehall.AzitWateringMimicMagical",
		"script.ai.siegablehall.AzitWateringRecycleCan",
		"script.ai.siegablehall.AzitWateringTreasureBox",
		"script.ai.siegablehall.Agit01Move1.Agit01Move1",
		"script.ai.siegablehall.Agit01Move1.Agit01PartisanSorcerer2",
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer1",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManager",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle1",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle2",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle3",
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle4",
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer2",
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand1",
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand2",
		"script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka1",
	},
		"number 1000275", "number 1000276", "number 1000277", "number 1000278", "number 1000279", "number 1000280",
		"number 1010624", "number 1010627", "number 1010625", "number 1010636", "number 1010637", "number 1010638",
		"number 1800023", "number 1010639",
		"number 1010628", "number 1010633", "number 1010634", "number 1010635", "number 1010640", "number 1010641",
		"string \"azit_watering_game_manager_default\"",
		"string \"rainbow_slow_1\"", "string \"rainbow_slow_2\"", "string \"rainbow_slow_3\"", "string \"rainbow_slow_4\"", "call Quest.getPartyMembers",
		"number 16", "number 43", "number 51", "number 52",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
