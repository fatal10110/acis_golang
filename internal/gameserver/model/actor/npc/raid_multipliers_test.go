package npc

import (
	"math"
	"testing"
)

// raidStatTemplate is level 20 (level mod 1.09) with CON 21 (bonus 0.82) and
// MEN 20 (bonus 1.22).
func raidStatTemplate(kind string) *Template {
	return &Template{
		ID: 1, Type: kind, Level: 20, STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 20, MEN: 20,
		PDef: 51, MDef: 41, HPRegen: 3.5, MPRegen: 1.7, HPMax: 500, MPMax: 200,
	}
}

// TestRaidMultipliersScaleRaidRelatedBaseStats pins CreatureStatus.getPDef /
// getMDef / getRegenHp / getRegenMp (CreatureStatus.java:610-675): a
// raid-related NPC's template base is multiplied by RaidDefenceMultiplier,
// RaidHpRegenMultiplier and RaidMpRegenMultiplier before the stat funcs run,
// and the defences are truncated afterwards; any other NPC keeps the raw
// base. Expected values are worked by hand from the reference funcs:
// P.Def = base * levelMod, M.Def = base * MEN_BONUS * levelMod, HP regen =
// base * CON_BONUS * levelMod, MP regen = base * MEN_BONUS * levelMod.
func TestRaidMultipliersScaleRaidRelatedBaseStats(t *testing.T) {
	mults := RaidMultipliers{Defence: 2, HPRegen: 3, MPRegen: 0.5}
	for _, tc := range []struct {
		name                 string
		kind                 string
		minion               bool // a plain monster marked raid related at spawn
		install              bool
		pDef, mDef, hpR, mpR float64
	}{
		// 51*1.09 = 55.59; 41*1.22*1.09 = 54.52; 3.5*0.82*1.09; 1.7*1.22*1.09.
		{name: "plain monster ignores the multipliers", kind: "Monster", install: true, pDef: 55, mDef: 54, hpR: 3.1283, mpR: 2.26066},
		// 102*1.09 = 111.18; 82*1.22*1.09 = 109.04; 10.5*0.82*1.09; 0.85*1.22*1.09.
		{name: "raid boss", kind: "RaidBoss", install: true, pDef: 111, mDef: 109, hpR: 9.3849, mpR: 1.13033},
		{name: "grand boss", kind: "GrandBoss", install: true, pDef: 111, mDef: 109, hpR: 9.3849, mpR: 1.13033},
		{name: "raid minion", kind: "Monster", minion: true, install: true, pDef: 111, mDef: 109, hpR: 9.3849, mpR: 1.13033},
		{name: "raid boss without installed multipliers", kind: "RaidBoss", pDef: 55, mDef: 54, hpR: 3.1283, mpR: 2.26066},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := NewHostile(&Instance{ObjectID: 1, Template: raidStatTemplate(tc.kind), Kind: InstanceKind(tc.kind)}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
			if err != nil {
				t.Fatal(err)
			}
			if tc.install {
				h.SetRaidMultipliers(mults)
			}
			if tc.minion {
				h.SetRaidRelated(true)
			}
			if got := h.PDef(); got != tc.pDef {
				t.Errorf("PDef() = %v, want %v", got, tc.pDef)
			}
			if got := h.MDef(); got != tc.mDef {
				t.Errorf("MDef() = %v, want %v", got, tc.mDef)
			}
			if got := h.HPRegenRate(); math.Abs(got-tc.hpR) > 1e-9 {
				t.Errorf("HPRegenRate() = %v, want %v", got, tc.hpR)
			}
			if got := h.MPRegenRate(); math.Abs(got-tc.mpR) > 1e-9 {
				t.Errorf("MPRegenRate() = %v, want %v", got, tc.mpR)
			}
		})
	}
}

// TestHostileMagicAttackSpeedStartsFrom333 pins CreatureStatus.getMAtkSpd
// (CreatureStatus.java:636-639, not overridden by NpcStatus): the casting
// speed finalizes from 333, not from the template's P.Atk.Spd, through
// FuncMAtkSpeed's WIT bonus (WIT 20: 1.00, WIT 30: 1.63).
func TestHostileMagicAttackSpeedStartsFrom333(t *testing.T) {
	for _, tc := range []struct {
		wit  int
		want int
	}{
		{wit: 20, want: 333},
		{wit: 30, want: 542}, // int(333 * 1.63) = int(542.79)
	} {
		tpl := raidStatTemplate("Monster")
		tpl.AtkSpd, tpl.WIT = 253, tc.wit
		h := newCombatHostile(t, 1, tpl)
		if got := h.MagicAttackSpeed(); got != tc.want {
			t.Errorf("WIT %d: MagicAttackSpeed() = %d, want %d", tc.wit, got, tc.want)
		}
	}
}
