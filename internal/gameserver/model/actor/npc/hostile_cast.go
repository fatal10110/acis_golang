package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// CastControl is the live cast controller surface an NPC's damage and
// crowd-control paths drive. Its cancel broadcast and AI notification come
// from the controller's own event sink.
type CastControl interface {
	CastingNow() bool
	CurrentSkillIsMagic() bool
	// InterruptCast aborts the cast only while it is still inside its
	// interrupt window.
	InterruptCast()
	// StopCast aborts the cast unconditionally.
	StopCast()
	// InterruptCastOnDamage applies the damage cast-break rule with the
	// inputs of the creature taking the damage.
	InterruptCastOnDamage(damage float64, men int, attackCancel func(float64) float64, roll int, immune bool) bool
}

// SetCastController wires h's live cast controller. An NPC without one has
// no cast to stop or break.
func (h *Hostile) SetCastController(c CastControl) {
	h.cast.Store(&c)
}

// CastControl returns h's cast controller, nil when it has none.
func (h *Hostile) CastControl() CastControl { return h.castControl() }

func (h *Hostile) castControl() CastControl {
	if p := h.cast.Load(); p != nil {
		return *p
	}
	return nil
}

// CastingNow reports whether h has a cast in flight.
func (h *Hostile) CastingNow() bool {
	c := h.castControl()
	return c != nil && c.CastingNow()
}

// CurrentSkillIsMagic reports whether h's cast in flight is a magic skill.
func (h *Hostile) CurrentSkillIsMagic() bool {
	c := h.castControl()
	return c != nil && c.CurrentSkillIsMagic()
}

// InterruptCast aborts h's cast while it is still inside its interrupt
// window; observers see MagicSkillCanceled.
func (h *Hostile) InterruptCast() {
	if c := h.castControl(); c != nil {
		c.InterruptCast()
	}
}

// StopCast aborts h's cast unconditionally; observers see MagicSkillCanceled
// when a cast was in flight.
func (h *Hostile) StopCast() {
	if c := h.castControl(); c != nil {
		c.StopCast()
	}
}

// BreakCastOnDamage rolls whether a landed auto-attack hit, or a skill hit
// whose handler rolls the break ahead of other per-hit work (then applying
// the hit through ReduceHPWithoutCastBreak), of damage breaks h's cast. A
// dead NPC draws no roll.
func (h *Hostile) BreakCastOnDamage(damage float64) {
	if h.AlikeDead() {
		return
	}
	h.breakCastOnDamage(damage)
}

// breakCastOnDamage rolls whether damage h takes breaks its cast, reading
// MEN, ATTACK_CANCEL and the roll from h itself. A raid-related or
// invulnerable NPC is never broken. An NPC with no cast in flight draws no
// roll.
func (h *Hostile) breakCastOnDamage(damage float64) {
	c := h.castControl()
	if c == nil || !c.CastingNow() {
		return
	}
	c.InterruptCastOnDamage(damage, h.MEN(), func(base float64) float64 {
		return h.CalcStat(stat.AttackCancel, base)
	}, h.Roll(100), h.RaidRelated() || h.Invul())
}
