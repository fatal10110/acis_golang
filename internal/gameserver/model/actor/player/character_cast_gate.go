package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// WearingFormalWear reports whether the chest slot holds a full-body formal
// dress, which forbids item and skill use.
func (c *Character) WearingFormalWear() bool {
	inv := c.Inventory()
	if inv == nil {
		return false
	}
	inst := inv.ItemAt(itemcontainer.Chest)
	if inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	return ok && tmpl.Slot == item.SlotAllDress
}

// SiegeGround answers the castle sieges under way.
type SiegeGround interface {
	// AttackerAt reports whether clanID attacks the siege in progress whose
	// battlefield holds the point (x, y, z).
	AttackerAt(clanID int32, x, y, z int) bool
	// DefenderAt reports whether clanID owns or defends the siege in
	// progress whose battlefield holds the point (x, y, z).
	DefenderAt(clanID int32, x, y, z int) bool
	// SameSideAt reports whether a siege is in progress whose battlefield
	// holds the point (x, y, z), and the clans a and b are not on opposite
	// sides of it.
	SameSideAt(a, b int32, x, y, z int) bool
}

// SealRules answers the Seven Signs seal rules a character's actions read.
type SealRules interface {
	// StrifeForbidsSummoning reports whether the Seal of Strife forbids
	// objectID to call a siege summon.
	StrifeForbidsSummoning(objectID int32) bool
}

// ActiveSiegeAttacker reports whether the character's clan attacks the siege
// in progress where the character stands.
func (c *Character) ActiveSiegeAttacker() bool {
	if c.sieges == nil {
		return false
	}
	x, y, z := c.Position()
	return c.sieges.AttackerAt(c.ClanID(), x, y, z)
}

// StrifeForbidsSummoning reports whether the Seal of Strife forbids the
// character to call a siege summon.
func (c *Character) StrifeForbidsSummoning() bool {
	return c.seals != nil && c.seals.StrifeForbidsSummoning(c.ObjectID())
}

// SetInCastleZone records the live zone engine's current castle zone
// membership (zone.FlagCastle).
func (c *Character) SetInCastleZone(inside bool) { c.insideCastleZone.Store(inside) }

// InCastleZone reports whether the character stands in a castle's grounds.
func (c *Character) InCastleZone() bool { return c.insideCastleZone.Load() }
