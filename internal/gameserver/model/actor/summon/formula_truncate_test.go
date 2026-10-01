package summon

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// TestSummonCombatStatGettersTruncateTheFinalizedStat pins the (int) cast
// in CreatureStatus.getPAtk / getPDef / getMAtk / getMDef
// (CreatureStatus.java:628-675) for a servitor and in PetStatus's overrides
// (PetStatus.java:141-183) for a pet: the finalized stat reaches every
// caller without its fractional part.
func TestSummonCombatStatGettersTruncateTheFinalizedStat(t *testing.T) {
	const level = 44
	stats := CombatStats{
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 101, PDef: 51, MAtk: 64, MDef: 41,
		MaxHP: 500, MaxMP: 200, BaseRandomDamage: 5,
	}
	lm := (100.0 - 11 + level) / 100.0
	intMod := statbonus.INTBonus[stats.INT]
	raw := map[string]float64{
		"PAtk": stats.PAtk * statbonus.STRBonus[stats.STR] * lm,
		"PDef": stats.PDef * lm,
		"MAtk": stats.MAtk * ((lm * lm) * (intMod * intMod)),
		"MDef": stats.MDef * statbonus.MENBonus[stats.MEN] * lm,
	}
	for kind, a := range map[string]*Actor{
		"servitor": mustServitor(t, ServitorConfig{ObjectID: 1, Level: level, Stats: stats, Roll: zeroSummonRoll}),
		"pet":      mustPet(t, PetConfig{ObjectID: 2, Level: level, Stats: stats, Roll: zeroSummonRoll}),
	} {
		for name, got := range map[string]float64{"PAtk": a.PAtk(), "PDef": a.PDef(), "MAtk": a.MAtk(), "MDef": a.MDef()} {
			if raw[name] == math.Trunc(raw[name]) {
				t.Fatalf("%s fixture finalizes whole (%v); the oracle needs a fraction", name, raw[name])
			}
			if want := math.Trunc(raw[name]); got != want {
				t.Fatalf("%s %s() = %v, want %v (finalized %v)", kind, name, got, want, raw[name])
			}
		}
	}
}
