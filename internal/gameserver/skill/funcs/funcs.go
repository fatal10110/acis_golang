// Package funcs provides the attribute-driven attack/defense/regen/speed
// modifiers that finalize a Creature's base combat stats from its six
// attributes and level, before any item/skill bonus is layered on top. Each
// value in this package is a basefunc.Func running at basefunc.OrderFinalize
// and is meant to be attached once, by default, to every creature's
// calculation chain for its Stat.
package funcs

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

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

// fixed is the embeddable state shared by every Func in this package: they
// all run at basefunc.OrderFinalize and are attached with no owner,
// configured value, or gating Condition.
type fixed struct{ s stat.Stat }

func (f fixed) Stat() stat.Stat          { return f.s }
func (f fixed) Order() int               { return basefunc.OrderFinalize }
func (f fixed) Owner() any               { return nil }
func (f fixed) Value() float64           { return 0 }
func (f fixed) Cond() basefunc.Condition { return nil }
