package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// CastController is the live cast-controller surface a network-owned cast
// controller exposes back onto the character it casts for. The domain layer
// depends on this narrow interface instead of the cast package itself,
// which already imports player to adapt Character to its own Actor
// contract (PlayerActor) — importing back would cycle.
type CastController interface {
	CastingNow() bool
	CurrentSkillIsMagic() bool
	InterruptCast()
	StopCast()
	// CanAbortCast reports whether the active cast is still inside its
	// interrupt window — the Esc cast-cancel path only fires at all when
	// this is true (RequestTargetCancel.java:26 canAbortCast() gate).
	CanAbortCast() bool
	// InterruptCastOnDamage applies the damage-based cast-break rule to the
	// live cast, using men/attackCancel/roll/immune resolved from the
	// character taking the damage — the reference always resolves these
	// from the interrupted creature, never the attacker.
	InterruptCastOnDamage(damage float64, men int, attackCancel func(float64) float64, roll int, immune bool) bool
}

// SetCastController wires c's live cast controller, called once by the
// network layer when it creates one for c.
func (c *Character) SetCastController(cast CastController) {
	c.castMu.Lock()
	defer c.castMu.Unlock()
	c.cast = cast
}

func (c *Character) castController() CastController {
	c.castMu.RLock()
	defer c.castMu.RUnlock()
	return c.cast
}

// CastingNow reports whether c has an active cast in flight.
func (c *Character) CastingNow() bool {
	cast := c.castController()
	return cast != nil && cast.CastingNow()
}

// CurrentSkillIsMagic reports whether c's active cast is a magic skill,
// satisfying Mute/PhysicalMute's shared magicCastTarget surface.
func (c *Character) CurrentSkillIsMagic() bool {
	cast := c.castController()
	return cast != nil && cast.CurrentSkillIsMagic()
}

// InterruptCast aborts c's in-progress cast if it is still inside its
// interrupt window, sending CASTING_INTERRUPTED — the abort-cast effect's
// path (EffectAbortCast.java uses interrupt(), not stop()).
func (c *Character) InterruptCast() {
	if cast := c.castController(); cast != nil {
		cast.InterruptCast()
	}
}

// StopCast aborts c's in-progress cast unconditionally: no interrupt-window
// gate, no CASTING_INTERRUPTED — the path Mute, PhysicalMute,
// SilenceMagicPhysical and RemoveTarget all use.
func (c *Character) StopCast() {
	if cast := c.castController(); cast != nil {
		cast.StopCast()
	}
}

// CanAbortCast reports whether c's active cast is still inside its
// interrupt window, matching CreatureCast.canAbortCast().
func (c *Character) CanAbortCast() bool {
	cast := c.castController()
	return cast != nil && cast.CanAbortCast()
}

// breakCastOnDamage applies the reference's damage-based cast-interrupt
// check (Formulas.calcCastBreak) to c's own in-progress cast. MEN and
// ATTACK_CANCEL are always resolved from c, the creature taking the
// damage, never the attacker — matching Formulas.calcCastBreak(target, dmg)
// reading everything off target. calcCastBreak has no damage guard: even a
// 0-damage hit rolls at rate clamped to [1,99], so this never short-circuits
// on damage <= 0.
func (c *Character) breakCastOnDamage(damage float64) {
	cast := c.castController()
	if cast == nil {
		return
	}
	cast.InterruptCastOnDamage(damage, c.MEN(), func(base float64) float64 {
		return c.CalcStat(stat.AttackCancel, base)
	}, c.rollValue(100), c.Invul())
}
