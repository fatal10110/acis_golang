package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// SoulshotCharged reports whether a soulshot charge is currently active.
func (c *Character) SoulshotCharged() bool {
	weapon := c.activeWeapon()
	return weapon.inst != nil && weapon.inst.ChargedShot(item.ShotSoul)
}

// SpiritshotCharged reports whether a spiritshot charge is currently active.
func (c *Character) SpiritshotCharged() bool {
	weapon := c.activeWeapon()
	return weapon.inst != nil && weapon.inst.ChargedShot(item.ShotSpirit)
}

// BlessedSpiritshotCharged reports whether a blessed spiritshot charge is currently active.
func (c *Character) BlessedSpiritshotCharged() bool {
	weapon := c.activeWeapon()
	return weapon.inst != nil && weapon.inst.ChargedShot(item.ShotBlessedSpirit)
}

// ChargeShotResult distinguishes why a direct-use shot charge attempt did
// or didn't take, so the network layer can pick the matching client
// message (or suppress it for an auto-shot-enabled item, the way the
// reference does).
type ChargeShotResult uint8

const (
	// ChargeShotOK means the weapon accepted the charge.
	ChargeShotOK ChargeShotResult = iota
	// ChargeShotNoCapacity means no real weapon is equipped, or it can't
	// carry this shot kind at all.
	ChargeShotNoCapacity
	// ChargeShotGradeMismatch means the shot's crystal grade doesn't match
	// the weapon's.
	ChargeShotGradeMismatch
	// ChargeShotAlreadyCharged means the weapon already carries this
	// charge; the reference answers this case with total silence, not a
	// system message.
	ChargeShotAlreadyCharged
)

// ChargeSoulshot attempts to charge the active weapon with a soulshot of
// shotCrystal grade, using reducedRoll (a 0-99 percentile roll) to decide
// whether the weapon's reduced-consumption count applies. Checks run
// capacity, then grade, then already-charged — the reference's own order
// for this shot kind, which differs from ChargeSpiritshot's order. On
// ChargeShotOK the weapon is marked charged and consume is the count to
// destroy from the item stack.
func (c *Character) ChargeSoulshot(shotCrystal item.CrystalType, reducedRoll int) (consume int32, result ChargeShotResult) {
	w := c.activeWeapon()
	if w.inst == nil || w.tmpl == nil || w.tmpl.Weapon == nil || w.tmpl.Weapon.SoulshotCount == 0 {
		return 0, ChargeShotNoCapacity
	}
	if w.tmpl.Crystal != shotCrystal {
		return 0, ChargeShotGradeMismatch
	}
	if w.inst.ChargedShot(item.ShotSoul) {
		return 0, ChargeShotAlreadyCharged
	}
	consume, _ = w.tmpl.Weapon.EvaluateSoulshot(w.tmpl.Crystal, shotCrystal, false, reducedRoll)
	w.inst.SetChargedShot(item.ShotSoul, true)
	return consume, ChargeShotOK
}

// ChargeSpiritshot attempts to charge the active weapon with a spiritshot
// of shotCrystal grade (kind is ShotSpirit or ShotBlessedSpirit; both draw
// from the weapon's same spiritshot capacity). Checks run capacity, then
// already-charged, then grade — the reference's own order for this shot
// kind, which differs from ChargeSoulshot's order. On ChargeShotOK the
// weapon is marked charged with kind and consume is the count to destroy
// from the item stack.
func (c *Character) ChargeSpiritshot(kind item.ShotKind, shotCrystal item.CrystalType) (consume int32, result ChargeShotResult) {
	w := c.activeWeapon()
	if w.inst == nil || w.tmpl == nil || w.tmpl.Weapon == nil || w.tmpl.Weapon.SpiritshotCount == 0 {
		return 0, ChargeShotNoCapacity
	}
	if w.inst.ChargedShot(kind) {
		return 0, ChargeShotAlreadyCharged
	}
	if w.tmpl.Crystal != shotCrystal {
		return 0, ChargeShotGradeMismatch
	}
	consume, _ = w.tmpl.Weapon.EvaluateSpiritshot(w.tmpl.Crystal, shotCrystal, false)
	w.inst.SetChargedShot(kind, true)
	return consume, ChargeShotOK
}
