package summon

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// CastControl is the live cast controller surface a summon's damage and
// crowd-control paths drive. Its cancel broadcast and the owner's
// CASTING_INTERRUPTED come from the controller's own event sink.
type CastControl interface {
	CastingNow() bool
	CurrentSkillIsMagic() bool
	Now() time.Time
	// Interrupt aborts the cast only while it is still inside its interrupt
	// window at now, and reports whether it did.
	Interrupt(now time.Time) bool
	// StopInFlight aborts the cast unconditionally, and reports whether
	// one was in flight.
	StopInFlight() bool
	// InterruptCastOnDamage applies the damage cast-break rule with the
	// inputs of the creature taking the damage, and reports whether it
	// broke the cast.
	InterruptCastOnDamage(damage float64, men int, attackCancel func(float64) float64, roll int, immune bool) bool
}

// CastingNow reports whether a has a cast in flight.
func (a *Actor) CastingNow() bool {
	return a.cast != nil && a.cast.CastingNow()
}

// CurrentSkillIsMagic reports whether a's cast in flight is a magic skill.
func (a *Actor) CurrentSkillIsMagic() bool {
	return a.cast != nil && a.cast.CurrentSkillIsMagic()
}

// InterruptCast aborts a's cast while it is still inside its interrupt
// window. The aborted cast's end moves the AI on and sends a idle (see
// CastStopped).
func (a *Actor) InterruptCast() {
	if a.cast != nil {
		a.cast.Interrupt(a.cast.Now())
	}
}

// StopCast aborts a's cast unconditionally and sends a idle, whether or not
// a cast was in flight: an aborted cast's end does both (see CastStopped).
func (a *Actor) StopCast() {
	if a.cast != nil && a.cast.StopInFlight() {
		return
	}
	a.TryToIdle()
}

// BreakCastOnDamage rolls whether a landed auto-attack hit, or a skill hit
// whose handler rolls the break ahead of other per-hit work (then applying
// the hit through ReduceHPWithoutCastBreak), of damage breaks a's cast. A
// dead summon draws no roll.
func (a *Actor) BreakCastOnDamage(damage float64) {
	if a.Dead() {
		return
	}
	a.breakCastOnDamage(damage)
}

// breakCastOnDamage rolls whether damage a takes breaks its cast, reading
// MEN, ATTACK_CANCEL and the roll from a itself; a broken cast's end moves
// the AI on and sends a idle (see CastStopped). An invulnerable summon is
// never broken, and a summon with no cast in flight draws no roll.
func (a *Actor) breakCastOnDamage(damage float64) {
	if a.cast == nil || !a.cast.CastingNow() {
		return
	}
	a.cast.InterruptCastOnDamage(damage, a.MEN(), func(base float64) float64 {
		return a.CalcStat(stat.AttackCancel, base)
	}, a.Roll(100), a.Invul())
}
