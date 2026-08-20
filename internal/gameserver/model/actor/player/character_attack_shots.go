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

// ChargedShot reports whether kind is currently charged on the active weapon.
func (c *Character) ChargedShot(kind item.ShotKind) bool {
	weapon := c.activeWeapon()
	return weapon.inst != nil && weapon.inst.ChargedShot(kind)
}

// SetChargedShot charges or discharges kind on the active weapon.
func (c *Character) SetChargedShot(kind item.ShotKind, charged bool) {
	weapon := c.activeWeapon()
	if weapon.inst != nil {
		weapon.inst.SetChargedShot(kind, charged)
	}
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

// ChargeSoulshot evaluates whether the active weapon can accept a soulshot
// charge of shotCrystal grade, using reducedRoll (a 0-99 percentile roll)
// to decide whether the weapon's reduced-consumption count applies. Checks
// run capacity, then grade, then already-charged — the reference's own
// order for this shot kind, which differs from ChargeSpiritshot's order
// (SoulShots.java:27-45). It does not mark the weapon charged: the
// reference destroys the item stack first and only calls setChargedShot
// after that succeeds (SoulShots.java:49-62), so the caller commits the
// charge itself via SetChargedShot(item.ShotSoul, true) once the item is
// destroyed. On ChargeShotOK, consume is the count to destroy.
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
	return consume, ChargeShotOK
}

// ChargeSpiritshot evaluates whether the active weapon can accept a
// spiritshot charge of shotCrystal grade (kind is ShotSpirit or
// ShotBlessedSpirit; both draw from the weapon's same spiritshot capacity).
// Checks run capacity, then already-charged, then grade — the reference's
// own order for this shot kind, which differs from ChargeSoulshot's order
// (SpiritShots.java:25-43). It does not mark the weapon charged: the
// reference destroys the item stack first and only calls setChargedShot
// after that succeeds (SpiritShots.java:45-57), so the caller commits the
// charge itself via SetChargedShot(kind, true) once the item is destroyed.
// On ChargeShotOK, consume is the count to destroy.
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
	return consume, ChargeShotOK
}
