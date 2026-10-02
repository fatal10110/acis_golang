package formulas

import "testing"

// The expected values below come from a standalone Java 21 probe that
// copies the reference arithmetic operation for operation:
// Formulas.calcCubicSkillSuccess (Formulas.java:1052-1089, the cubic's int
// M.Atk, x4 under blessed spiritshot, Math.clamp(rate, 1, 99)) and
// Formulas.calcMagicDam(Cubic, ...) (Formulas.java:644-694). None was
// derived from the Go code.

func TestCubicMAtkModifierMatchesReference(t *testing.T) {
	tests := []struct {
		mAtk, mDef float64
		bss        bool
		want       float64
	}{
		{282, 100, false, 1.847214118612133},
		{282, 100, true, 3.694428237224266},
		{1975, 385, true, 2.5394841192330255},
		{434, 1, false, 229.15933321599624},
		{1026, 733, false, 0.48068701543933595},
		{820, 57, true, 11.052353101476484},
	}
	for _, tt := range tests {
		if got := CubicMAtkModifier(tt.mAtk, tt.mDef, tt.bss); got != tt.want {
			t.Errorf("CubicMAtkModifier(%v, %v, %v) = %v, want %v", tt.mAtk, tt.mDef, tt.bss, got, tt.want)
		}
	}
}

func TestCubicSkillSuccessRateMatchesReference(t *testing.T) {
	tests := []struct {
		base, stat, vuln, mAtk, lvl float64
		want                        float64
	}{
		{80, 0.8, 1.0, CubicMAtkModifier(282, 100, false), 1.04, 99},
		{80, 1.2, 1.3, CubicMAtkModifier(1975, 385, true), 1.0, 99},
		{15, 0.5, 0.5, CubicMAtkModifier(282, 733, false), 0.9, 1},
		{70, 1, 1, 1, 1, 70},
		{20, 2, 2, CubicMAtkModifier(1975, 50, true), 1.2, 99},
	}
	for _, tt := range tests {
		in := SkillSuccessInput{BaseChance: tt.base, StatModifier: tt.stat, VulnModifier: tt.vuln, MAtkModifier: tt.mAtk, LevelModifier: tt.lvl}
		if got := SkillSuccessRate(in); got != tt.want {
			t.Errorf("SkillSuccessRate(%+v) = %v, want %v", in, got, tt.want)
		}
	}
}

func TestCubicMagicDamageMatchesReference(t *testing.T) {
	tests := []struct {
		name string
		in   CubicMagicDamageInput
		want float64
		hit  int
	}{
		{"plain", CubicMagicDamageInput{MDef: 385, SkillPower: 2399, ElementalMul: 1}, 567.0363636363636, 567},
		{"critical", CubicMagicDamageInput{MDef: 385, SkillPower: 2399, MagicCrit: true, ElementalMul: 1}, 2268.1454545454544, 2268},
		{"shield adds its defence", CubicMagicDamageInput{MDef: 385 + 120, SkillPower: 2399, Shield: ShieldSuccess, ElementalMul: 1}, 432.2950495049505, 432},
		{"half resist skips the critical", CubicMagicDamageInput{MDef: 733, SkillPower: 436, MagicCrit: true, Failure: MagicFailureHalf, ElementalMul: 1}, 27.06412005457026, 27},
		{"full resist then elemental", CubicMagicDamageInput{MDef: 733, SkillPower: 436, MagicCrit: true, Failure: MagicFailureFull, ElementalMul: 1.3}, 1.3, 1},
		{"perfect block", CubicMagicDamageInput{MDef: 91, SkillPower: 1000, MagicCrit: true, Shield: ShieldPerfect, ElementalMul: 1}, 1, 1},
		{"elemental resist", CubicMagicDamageInput{MDef: 57, SkillPower: 543, ElementalMul: 0.8}, 693.5157894736843, 693},
		{"critical on 1 M.Def", CubicMagicDamageInput{MDef: 1, SkillPower: 2262, MagicCrit: true, ElementalMul: 1}, 823368, 823368},
	}
	for _, tt := range tests {
		got := CubicMagicDamage(tt.in)
		if got != tt.want {
			t.Errorf("%s: CubicMagicDamage() = %v, want %v", tt.name, got, tt.want)
		}
		if hit := int(got); hit != tt.hit {
			t.Errorf("%s: int(CubicMagicDamage()) = %d, want %d", tt.name, hit, tt.hit)
		}
	}
}
