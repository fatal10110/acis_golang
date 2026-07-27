package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func TestSummonChargedShotStateAndCounts(t *testing.T) {
	servitor := NewServitor(ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200, SSCount: 5, SPSCount: 3}, Roll: zeroSummonRoll})

	if servitor.SoulshotCharged() || servitor.SpiritshotCharged() || servitor.BlessedSpiritshotCharged() {
		t.Fatal("new summon should carry no shot charge")
	}
	if servitor.SSCount() != 5 {
		t.Fatalf("SSCount() = %d, want 5", servitor.SSCount())
	}
	if servitor.SPSCount() != 3 {
		t.Fatalf("SPSCount() = %d, want 3", servitor.SPSCount())
	}

	servitor.SetChargedShot(item.ShotSoul, true)
	if !servitor.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = false after SetChargedShot(ShotSoul, true)")
	}
	if servitor.SpiritshotCharged() || servitor.BlessedSpiritshotCharged() {
		t.Fatal("charging soulshot must not charge spiritshot kinds")
	}

	servitor.SetChargedShot(item.ShotSoul, false)
	if servitor.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = true after SetChargedShot(ShotSoul, false)")
	}
}
