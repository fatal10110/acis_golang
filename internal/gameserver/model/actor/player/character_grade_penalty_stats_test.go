package player

import (
	"math"
	"testing"
)

func TestGradePenaltyAppliesToDependentStats(t *testing.T) {
	tmpl := combatTemplate()
	c := liveCharacter(1, tmpl, combatItems())

	baseRunSpeed := c.RunSpeed()
	baseSwimSpeed := c.SwimSpeed()
	baseMAtkSpd := c.MagicAttackSpeed()
	baseEvasion := c.Evasion()
	baseAccuracy := c.Accuracy()

	c.armorGradePenalty = 4
	c.weaponGradePenalty = true

	wantRunSpeed := baseRunSpeed * math.Pow(0.84, 4)
	if got := c.RunSpeed(); !closeFloat(got, wantRunSpeed) {
		t.Fatalf("RunSpeed() with armor penalty 4 = %v, want %v", got, wantRunSpeed)
	}

	wantSwimSpeed := baseSwimSpeed * math.Pow(0.84, 4)
	if got := c.SwimSpeed(); !closeFloat(got, wantSwimSpeed) {
		t.Fatalf("SwimSpeed() with armor penalty 4 = %v, want %v", got, wantSwimSpeed)
	}

	wantMAtkSpd := int(float64(baseMAtkSpd) * math.Pow(0.84, 4))
	if got := c.MagicAttackSpeed(); got != wantMAtkSpd {
		t.Fatalf("MagicAttackSpeed() with armor penalty 4 = %v, want %v", got, wantMAtkSpd)
	}

	wantEvasion := baseEvasion - 8
	if got := c.Evasion(); got != wantEvasion {
		t.Fatalf("Evasion() with armor penalty 4 = %v, want %v", got, wantEvasion)
	}

	wantAccuracy := baseAccuracy - 20
	if got := c.Accuracy(); got != wantAccuracy {
		t.Fatalf("Accuracy() with weapon penalty = %v, want %v", got, wantAccuracy)
	}

	c.armorGradePenalty = 0
	c.weaponGradePenalty = false

	if got := c.RunSpeed(); !closeFloat(got, baseRunSpeed) {
		t.Fatalf("RunSpeed() with no penalty = %v, want %v", got, baseRunSpeed)
	}
	if got := c.SwimSpeed(); !closeFloat(got, baseSwimSpeed) {
		t.Fatalf("SwimSpeed() with no penalty = %v, want %v", got, baseSwimSpeed)
	}
	if got := c.MagicAttackSpeed(); got != baseMAtkSpd {
		t.Fatalf("MagicAttackSpeed() with no penalty = %v, want %v", got, baseMAtkSpd)
	}
	if got := c.Evasion(); got != baseEvasion {
		t.Fatalf("Evasion() with no penalty = %v, want %v", got, baseEvasion)
	}
	if got := c.Accuracy(); got != baseAccuracy {
		t.Fatalf("Accuracy() with no penalty = %v, want %v", got, baseAccuracy)
	}
}
