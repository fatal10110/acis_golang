package npc

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// TestHostileCombatStatGettersTruncateTheFinalizedStat pins the (int) cast
// in CreatureStatus.getPAtk / getPDef / getMAtk / getMDef
// (CreatureStatus.java:628-675) for an NPC: the finalized stat reaches every
// caller without its fractional part.
func TestHostileCombatStatGettersTruncateTheFinalizedStat(t *testing.T) {
	tpl := &Template{
		ID: 1, Type: "Monster", Level: 20, STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 101, PDef: 51, MAtk: 64, MDef: 41, HPMax: 500, MPMax: 200,
	}
	h := newCombatHostile(t, 1, tpl)
	lm := (100.0 - 11 + float64(tpl.Level)) / 100.0
	intMod := statbonus.INTBonus[tpl.INT]
	for _, tc := range []struct {
		name string
		got  float64
		raw  float64 // the finalized double, from the reference funcs
	}{
		{"PAtk", h.PAtk(), tpl.PAtk * statbonus.STRBonus[tpl.STR] * lm},
		{"PDef", h.PDef(), tpl.PDef * lm},
		{"MAtk", h.MAtk(), tpl.MAtk * ((lm * lm) * (intMod * intMod))},
		{"MDef", h.MDef(), tpl.MDef * statbonus.MENBonus[tpl.MEN] * lm},
	} {
		if tc.raw == math.Trunc(tc.raw) {
			t.Fatalf("%s fixture finalizes whole (%v); the oracle needs a fraction", tc.name, tc.raw)
		}
		if want := math.Trunc(tc.raw); tc.got != want {
			t.Fatalf("%s() = %v, want %v (finalized %v)", tc.name, tc.got, want, tc.raw)
		}
	}
}
