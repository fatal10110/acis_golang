package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// spyCastController records InterruptCastOnDamage calls and StopCast/
// InterruptCast invocations, so a test can pin exactly what a damage- or
// actor-state-driven abort trigger passes onto the live cast controller
// without depending on the real cast package (which already imports this
// one).
type spyCastController struct {
	casting bool
	magic   bool

	damageCalls []spyDamageCall
	damageBreak bool

	stopCalls      int
	interruptCalls int
}

type spyDamageCall struct {
	damage       float64
	men          int
	attackCancel float64
	roll         int
	immune       bool
}

func (s *spyCastController) CastingNow() bool         { return s.casting }
func (s *spyCastController) CurrentSkillIsMagic() bool { return s.magic }
func (s *spyCastController) InterruptCast()           { s.interruptCalls++ }
func (s *spyCastController) StopCast()                { s.stopCalls++ }

func (s *spyCastController) InterruptCastOnDamage(damage float64, men int, attackCancel func(float64) float64, roll int, immune bool) bool {
	cancelled := 0.0
	if attackCancel != nil {
		cancelled = attackCancel(0)
	}
	s.damageCalls = append(s.damageCalls, spyDamageCall{damage: damage, men: men, attackCancel: cancelled, roll: roll, immune: immune})
	return s.damageBreak
}

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

func TestReduceHPSkipsCastControllerOnZeroDamage(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	spy := &spyCastController{casting: true, magic: true}
	c.SetCastController(spy)

	c.ReduceHP(0, nil, modelskill.Definition{})

	if len(spy.damageCalls) != 0 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 0 for zero damage", len(spy.damageCalls))
	}
}

func TestTakeDamageForwardsDamageToCastController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	c.SetRollSource(zeroRoll)
	spy := &spyCastController{casting: true, magic: false}
	c.SetCastController(spy)

	c.TakeDamage(15, nil)

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1", len(spy.damageCalls))
	}
	if got := spy.damageCalls[0].damage; got != 15 {
		t.Fatalf("damage = %v, want 15", got)
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
