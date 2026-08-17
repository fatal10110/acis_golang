// Package funcs provides the attribute-driven attack/defense/regen/speed
// modifiers that finalize a Creature's base combat stats from its six
// attributes and level, before any item/skill bonus is layered on top. Each
// value in this package is a Func meant to run once, as the static finalize
// step of a Creature's per-stat calculation chain (see skill/effect's
// Calculator), attached at construction and never detached: unlike a data
// modifier from an XML FuncTemplate, a builtin here carries no owner and is
// never removed from a live creature.
package funcs

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// Func computes the next running value for one Stat's finalize step, given
// the base value the calculation chain started from and the value computed
// so far.
type Func func(actor stat.Actor, base, value float64) float64

// Equipment slot bits used by the armor and accessory defense penalties.
// These intentionally duplicate model/item's Slot bit values rather than
// importing that package, since this package only ever needs to pass a slot
// identity through PlayerActor.HasEquipped, never interpret one itself.
const (
	SlotLFinger = 1 << iota
	SlotRFinger
	SlotLEar
	SlotREar
	SlotNeck
	SlotHead
	SlotChest
	SlotLegs
	SlotGloves
	SlotFeet
	// FullBodyArmor is not a paperdoll slot; it reports whether the item
	// occupying the chest slot is a full-body piece, which pDefMod also
	// treats like a worn legs item. A full-body piece still occupies the
	// chest slot, so an implementation of HasEquipped must report
	// SlotChest true whenever it reports FullBodyArmor true — the two
	// penalties are independent and both apply.
	FullBodyArmor
)
