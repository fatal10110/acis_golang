package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestReduceHPForwardsDamageToCastController(t *testing.T) {
	tmpl := combatTemplate()
	c := liveCharacter(1, tmpl, combatItems())
	c.SetHP(100)
	c.SetRollSource(func(int) int { return 42 })
	spy := &spyCastController{casting: true, magic: true}
	c.SetCastController(spy)

	c.ReduceHP(30, nil, modelskill.Definition{})

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1", len(spy.damageCalls))
	}
	got := spy.damageCalls[0]
	if got.damage != 30 {
		t.Fatalf("damage = %v, want 30", got.damage)
	}
	if got.men != tmpl.MEN {
		t.Fatalf("men = %d, want template MEN %d", got.men, tmpl.MEN)
	}
	if got.roll != 42 {
		t.Fatalf("roll = %d, want the injected roll source's 42", got.roll)
	}
	if got.immune {
		t.Fatal("immune = true for a non-invul character, want false")
	}
}

// TestReduceHPRollsCastBreakOnZeroDamage pins that a skill hit working out
// to no damage (a CHARGEDAM from a damage-denied caster) still rolls the
// cast break, at the zero amount, and leaves HP alone.
func TestReduceHPRollsCastBreakOnZeroDamage(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	spy := &spyCastController{casting: true, magic: true}
	c.SetCastController(spy)

	c.ReduceHP(0, nil, modelskill.Definition{})

	if len(spy.damageCalls) != 1 || spy.damageCalls[0].damage != 0 {
		t.Fatalf("InterruptCastOnDamage calls = %+v, want one at damage 0", spy.damageCalls)
	}
	if got := c.HP(); got != 100 {
		t.Fatalf("HP after a zero-damage hit = %v, want 100", got)
	}
}

// deniedPlayableAttacker is another playable whose damage permission is
// revoked.
type deniedPlayableAttacker struct {
	reduceHPPlayableAttacker
}

func (deniedPlayableAttacker) CanGiveDamage() bool { return false }

// TestReduceHPZeroDamageReportsStatusOnlyForPlayableCPHit pins which zero
// skill hits write anything: every one ends the sleep and rolls the cast
// break, but only another permitted playable's hit that goes through CP
// rewrites CP unchanged and reports the status, once. An NPC's, the
// character's own, a damage-denied attacker's and a direct-to-HP zero hit
// leave CP and HP alone and report nothing.
func TestReduceHPZeroDamageReportsStatusOnlyForPlayableCPHit(t *testing.T) {
	tests := []struct {
		name       string
		attacker   func(c *Character) attackable.Combatant
		def        modelskill.Definition
		broadcasts int
	}{
		{"other playable", func(*Character) attackable.Combatant { return reduceHPPlayableAttacker{} }, modelskill.Definition{}, 1},
		{"npc", func(*Character) attackable.Combatant { return &reduceHPNpcAttacker{} }, modelskill.Definition{}, 0},
		{"direct to HP", func(*Character) attackable.Combatant { return reduceHPPlayableAttacker{} }, modelskill.Definition{DirectHPDamage: true}, 0},
		{"self", func(c *Character) attackable.Combatant { return c }, modelskill.Definition{}, 0},
		{"damage denied", func(*Character) attackable.Combatant { return deniedPlayableAttacker{} }, modelskill.Definition{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 400, MaxCP: 200, CurrentCP: 150})
			attachTestLive(t, c)
			addCharacterEffect(t, c, "Sleep")
			spy := &spyCastController{casting: true, magic: true}
			c.SetCastController(spy)
			rec := recordEvents(c)

			c.ReduceHP(0, tt.attacker(c), tt.def)

			if got := countVitals(rec); got != tt.broadcasts {
				t.Fatalf("status broadcasts = %d, want %d", got, tt.broadcasts)
			}
			if c.CP() != 150 || c.HP() != 400 {
				t.Fatalf("cp/hp = %v/%v, want 150/400 unchanged", c.CP(), c.HP())
			}
			if c.Sleeping() {
				t.Fatal("Sleeping() = true after a zero hit, want the sleep effect stopped")
			}
			if len(spy.damageCalls) != 1 || spy.damageCalls[0].damage != 0 {
				t.Fatalf("InterruptCastOnDamage calls = %+v, want one at damage 0", spy.damageCalls)
			}
		})
	}
}

// TestReduceHPWithoutCastBreakSkipsOnlyTheBreakRoll pins the one-roll-per-hit
// contract DRAIN relies on: ReduceHP rolls the cast break once on the raw
// damage (before CP absorbs any of it), while ReduceHPWithoutCastBreak — used
// after the handler already rolled the break ahead of the effects — never
// rolls it again, but still takes CP/HP and still kills on a lethal amount.
func TestReduceHPWithoutCastBreakSkipsOnlyTheBreakRoll(t *testing.T) {
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	withBreak := liveCharacter(1, combatTemplate(), combatItems())
	withBreak.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 30})
	withBreak.SetRollSource(zeroRoll)
	breakSpy := &spyCastController{casting: true, magic: true}
	withBreak.SetCastController(breakSpy)

	withBreak.ReduceHP(50, attacker, modelskill.Definition{})

	if len(breakSpy.damageCalls) != 1 {
		t.Fatalf("ReduceHP InterruptCastOnDamage calls = %d, want 1", len(breakSpy.damageCalls))
	}
	if got := breakSpy.damageCalls[0].damage; got != 50 {
		t.Fatalf("ReduceHP break damage = %v, want the raw 50 (not the 20 left after CP)", got)
	}
	if withBreak.CP() != 0 || withBreak.HP() != 480 {
		t.Fatalf("ReduceHP cp/hp = %v/%v, want 0/480", withBreak.CP(), withBreak.HP())
	}

	noBreak := liveCharacter(3, combatTemplate(), combatItems())
	noBreak.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 30})
	noBreak.SetRollSource(zeroRoll)
	noBreakSpy := &spyCastController{casting: true, magic: true}
	noBreak.SetCastController(noBreakSpy)

	noBreak.ReduceHPWithoutCastBreak(50, attacker, modelskill.Definition{})

	if len(noBreakSpy.damageCalls) != 0 {
		t.Fatalf("ReduceHPWithoutCastBreak InterruptCastOnDamage calls = %d, want 0", len(noBreakSpy.damageCalls))
	}
	if noBreak.CP() != 0 || noBreak.HP() != 480 {
		t.Fatalf("ReduceHPWithoutCastBreak cp/hp = %v/%v, want 0/480", noBreak.CP(), noBreak.HP())
	}
	if noBreak.Dead() {
		t.Fatal("ReduceHPWithoutCastBreak killed on a non-lethal hit")
	}

	noBreak.ReduceHPWithoutCastBreak(10000, attacker, modelskill.Definition{})

	if len(noBreakSpy.damageCalls) != 0 {
		t.Fatalf("lethal ReduceHPWithoutCastBreak InterruptCastOnDamage calls = %d, want 0", len(noBreakSpy.damageCalls))
	}
	if !noBreak.Dead() || noBreak.HP() != 0 {
		t.Fatalf("lethal ReduceHPWithoutCastBreak dead/hp = %v/%v, want true/0", noBreak.Dead(), noBreak.HP())
	}
	if noBreakSpy.stopCalls != 1 {
		t.Fatalf("StopCast calls on the lethal hit = %d, want 1 (Die ran)", noBreakSpy.stopCalls)
	}
}

func TestTakeDamageForwardsDamageToCastController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	c.SetRollSource(zeroRoll)
	spy := &spyCastController{casting: true, magic: false}
	c.SetCastController(spy)

	c.TakeDamage(15, nil)
	if len(spy.damageCalls) != 0 {
		t.Fatalf("TakeDamage InterruptCastOnDamage calls = %d, want 0: the attacker rolls the break", len(spy.damageCalls))
	}
	c.BreakCastOnDamage(15)

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1", len(spy.damageCalls))
	}
	if got := spy.damageCalls[0].damage; got != 15 {
		t.Fatalf("damage = %v, want 15", got)
	}
}

// TestTakeDamageForwardsZeroDamageToCastController pins
// Formulas.calcCastBreak (Formulas.java:725-753), which has no damage
// guard: a 0-damage break roll still rolls the break chance, clamped to a
// 1% floor, instead of being skipped.
func TestTakeDamageForwardsZeroDamageToCastController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	c.SetRollSource(zeroRoll)
	spy := &spyCastController{casting: true, magic: false}
	c.SetCastController(spy)

	c.BreakCastOnDamage(0)

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1 (zero damage must still roll)", len(spy.damageCalls))
	}
	if got := spy.damageCalls[0].damage; got != 0 {
		t.Fatalf("damage = %v, want 0", got)
	}
}

func TestCharacterInterruptCastDelegatesToController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	spy := &spyCastController{}
	c.SetCastController(spy)

	c.InterruptCast()
	if spy.interruptCalls != 1 {
		t.Fatalf("InterruptCast delegated %d times, want 1", spy.interruptCalls)
	}

	c.StopCast()
	if spy.stopCalls != 1 {
		t.Fatalf("StopCast delegated %d times, want 1", spy.stopCalls)
	}
}

func TestCharacterCastDelegatesAreNoOpsWithoutAController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())

	// None of these may panic on a character with no cast controller wired
	// yet (e.g. an NPC/summon actor type, or a player before its network
	// session attaches one).
	c.InterruptCast()
	c.StopCast()
	if c.CastingNow() {
		t.Fatal("CastingNow() = true with no controller wired, want false")
	}
	if c.CurrentSkillIsMagic() {
		t.Fatal("CurrentSkillIsMagic() = true with no controller wired, want false")
	}
}

func TestCharacterDieStopsInFlightCast(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(1)
	spy := &spyCastController{casting: true}
	c.SetCastController(spy)

	if !c.Die(nil) {
		t.Fatal("Die() = false on a live character, want true")
	}
	if spy.stopCalls != 1 {
		t.Fatalf("StopCast calls on death = %d, want 1", spy.stopCalls)
	}
}
