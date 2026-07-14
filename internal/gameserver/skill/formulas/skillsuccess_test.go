package formulas

import "testing"

// Expected values below are the reference formula's exact arithmetic
// (Rnd.get(...) replaced by an explicit roll parameter) evaluated by hand
// on inputs chosen to make each branch and clamp boundary unambiguous.

func TestSkillSuccessRate(t *testing.T) {
	tests := []struct {
		name string
		in   SkillSuccessInput
		want float64
	}{
		{"identity modifiers", SkillSuccessInput{BaseChance: 50, StatModifier: 1, VulnModifier: 1, MAtkModifier: 1, LevelModifier: 1}, 50},
		{"clamped low", SkillSuccessInput{BaseChance: 1, StatModifier: 0.001, VulnModifier: 1, MAtkModifier: 1, LevelModifier: 1}, 1},
		{"clamped high", SkillSuccessInput{BaseChance: 500, StatModifier: 1, VulnModifier: 1, MAtkModifier: 1, LevelModifier: 1}, 99},
		{"ignores resists, uncapped", SkillSuccessInput{BaseChance: 150, IgnoreResists: true}, 150},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SkillSuccessRate(tt.in); got != tt.want {
				t.Errorf("SkillSuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSkillSucceeds(t *testing.T) {
	if !SkillSucceeds(50, 49) {
		t.Error("SkillSucceeds(50, 49) = false, want true")
	}
	if SkillSucceeds(50, 50) {
		t.Error("SkillSucceeds(50, 50) = true, want false")
	}
}

func TestSkillReflects(t *testing.T) {
	tests := []struct {
		name string
		in   SkillReflectInput
		roll int
		want bool
	}{
		{"magic lands", SkillReflectInput{CanBeReflected: true, Magic: true, ReflectChance: 30}, 29, true},
		{"magic misses", SkillReflectInput{CanBeReflected: true, Magic: true, ReflectChance: 30}, 30, false},
		{"melee within range lands", SkillReflectInput{CanBeReflected: true, Magic: false, CastRange: 40, ReflectChance: 10}, 9, true},
		{"melee beyond range never reflects", SkillReflectInput{CanBeReflected: true, Magic: false, CastRange: 41, ReflectChance: 100}, 0, false},
		{"melee with no cast range never reflects", SkillReflectInput{CanBeReflected: true, Magic: false, CastRange: -1, ReflectChance: 100}, 0, false},
		{"ignores resists never reflects", SkillReflectInput{IgnoreResists: true, CanBeReflected: true, Magic: true, ReflectChance: 100}, 0, false},
		{"cannot be reflected", SkillReflectInput{CanBeReflected: false, Magic: true, ReflectChance: 100}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SkillReflects(tt.in, tt.roll); got != tt.want {
				t.Errorf("SkillReflects() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRevivePower(t *testing.T) {
	tests := []struct {
		name            string
		witBonus, power float64
		want            float64
	}{
		{"zero power passes through", 2.0, 0, 0},
		{"full power passes through", 0.1, 100, 100},
		{"neutral bonus", 1.0, 50, 50},
		{"bonus capped 20 above base", 2.0, 50, 70},
		{"malus floored at base", 0.5, 50, 50},
		{"hard cap at 90", 1.5, 80, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RevivePower(tt.witBonus, tt.power); got != tt.want {
				t.Errorf("RevivePower() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCancelSuccessRate(t *testing.T) {
	tests := []struct {
		name                    string
		effectPeriod, diffLevel int
		baseRate, vuln          float64
		minRate, maxRate        int
		want                    float64
	}{
		{"mid range", 240, 5, 50, 1, 25, 75, 62},
		{"clamped to min", 240, -100, 50, 1, 25, 75, 25},
		{"clamped to max", 240, 100, 50, 1, 25, 75, 75},
		{"integer division floors", 239, 0, 0, 1, 0, 100, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CancelSuccessRate(tt.effectPeriod, tt.diffLevel, tt.baseRate, tt.vuln, tt.minRate, tt.maxRate)
			if got != tt.want {
				t.Errorf("CancelSuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCancelSucceeds(t *testing.T) {
	if !CancelSucceeds(62, 61) {
		t.Error("CancelSucceeds(62, 61) = false, want true")
	}
	if CancelSucceeds(62, 62) {
		t.Error("CancelSucceeds(62, 62) = true, want false")
	}
}
