package npc

import "testing"

// recordingCast is a CastControl that is always casting and records each
// cast-break roll, together with the NPC's HP at the moment of the roll.
type recordingCast struct {
	h     *Hostile
	rolls []castBreakRoll
}

type castBreakRoll struct {
	damage float64
	roll   int
	immune bool
	hp     float64
}

func (*recordingCast) CastingNow() bool          { return true }
func (*recordingCast) CurrentSkillIsMagic() bool { return false }
func (*recordingCast) InterruptCast()            {}
func (*recordingCast) StopCast()                 {}
func (c *recordingCast) InterruptCastOnDamage(damage float64, _ int, _ func(float64) float64, roll int, immune bool) bool {
	c.rolls = append(c.rolls, castBreakRoll{damage: damage, roll: roll, immune: immune, hp: c.h.HP()})
	return !immune
}

func newCastingHostile(t *testing.T) (*Hostile, *recordingCast) {
	t.Helper()
	h := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", Level: 10, HPMax: 500})
	h.SetRollSource(func(int) int { return 7 })
	c := &recordingCast{h: h}
	h.SetCastController(c)
	return h, c
}

// TestHostileTakeDamageRollsCastBreakAfterHPChange pins the auto-attack
// order of CreatureAttack.doHit: the target's HP is reduced first, with no
// cast-break roll of its own; the attacker then rolls the break through
// BreakCastOnDamage, with the target's own roll.
func TestHostileTakeDamageRollsCastBreakAfterHPChange(t *testing.T) {
	h, c := newCastingHostile(t)
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 10, HPMax: 500})

	full := h.HP()

	h.TakeDamage(50, attacker)
	if len(c.rolls) != 0 {
		t.Fatalf("TakeDamage rolled cast breaks %+v, want none", c.rolls)
	}
	h.BreakCastOnDamage(50)

	want := castBreakRoll{damage: 50, roll: 7, hp: full - 50}
	if len(c.rolls) != 1 || c.rolls[0] != want {
		t.Fatalf("cast-break rolls = %+v, want [%+v]", c.rolls, want)
	}
}

// TestHostileTakeDamageWithoutDamagePermissionStillRollsCastBreak pins the
// no-permission branch: CreatureStatus.reduceHp drops the HP change for an
// attacker whose access level cannot give damage, but CreatureAttack still
// runs Formulas.calcCastBreak afterwards, so the cast can break while the
// NPC keeps its full HP.
func TestHostileTakeDamageWithoutDamagePermissionStillRollsCastBreak(t *testing.T) {
	h, c := newCastingHostile(t)
	attacker := deniedLethalCaster{newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 10, HPMax: 500})}
	full := h.HP()

	if h.TakeDamage(50, attacker) {
		t.Fatal("TakeDamage() killed the NPC, want the hit blocked")
	}
	h.BreakCastOnDamage(50)

	if got := h.HP(); got != full {
		t.Fatalf("HP = %v after a hit without damage permission, want untouched %v", got, full)
	}
	want := castBreakRoll{damage: 50, roll: 7, hp: full}
	if len(c.rolls) != 1 || c.rolls[0] != want {
		t.Fatalf("cast-break rolls = %+v, want [%+v]", c.rolls, want)
	}
}
