package hall

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/scriptfp"
)

// TestMatchesReferenceFingerprints compares the package with the reference
// classes: literals, engine calls and hook shapes. The reference names the
// shouts and lines by their NpcString constants and Dietrich's healer and
// buffer classes by their class id constants; the port writes their ids
// (30, the Elven Elder's, is a literal of the reference already).
func TestMatchesReferenceFingerprints(t *testing.T) {
	problems, err := scriptfp.Check(".", []string{
		"script.ai.siegablehall.Agit01PartisanSorcerer1",
		"script.ai.siegablehall.Agit02Dietrich",
		"script.ai.siegablehall.Agit02Gustav",
		"script.ai.siegablehall.Agit02Mikhail",
		"script.ai.siegablehall.Agit03Alfred",
		"script.ai.siegablehall.Agit03Giselle",
		"script.ai.siegablehall.Agit03Lidia",
		"script.ai.siegablehall.AzitWateringGroud",
		"script.ai.siegablehall.AzitWateringMimic",
		"script.ai.siegablehall.AzitWateringMimicMagical",
		"script.ai.siegablehall.AzitWateringRecycleCan",
		"script.ai.siegablehall.AzitWateringTreasureBox",
		"script.ai.siegablehall.Agit01Move1.Agit01Move1",
		"script.ai.siegablehall.Agit01Move1.Agit01PartisanSorcerer2",
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer1",
	},
		"number 1000275", "number 1000276", "number 1000277", "number 1000278", "number 1000279", "number 1000280",
		"number 1010624", "number 1010625", "number 1010636", "number 1010637", "number 1010638",
		"number 1800023", "number 1010639",
		"number 16", "number 17", "number 43", "number 51", "number 52",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}
