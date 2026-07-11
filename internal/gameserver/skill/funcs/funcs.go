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

// Actor is the live combat data every Func in this package reads from its
// effector. It stands in for the not-yet-built creature runtime; a future
// concrete actor type satisfies it structurally.
type Actor interface {
	STR() int
	CON() int
	DEX() int
	INT() int
	WIT() int
	MEN() int
	Level() int

	// LevelMod is the level-scaling factor (Creature status computes it
	// from Level; it is not derivable from Level alone without the
	// player-level data table, so it is asked for directly here).
	LevelMod() float64

	// IsSummon reports whether this actor is a player's summon, which a
	// few funcs treat differently from either a bare player or an NPC.
	IsSummon() bool
}

// PlayerActor narrows Actor to the extra data only a player-controlled
// actor carries: henna stat bonuses, worn-equipment checks, and whether the
// player's class is a mage class. A func type-asserts effector to this
// interface exactly where the reference implementation checks
// "effector instanceof Player".
type PlayerActor interface {
	Actor

	IsMageClass() bool

	// HennaBonus returns the flat bonus the player's applied hennas grant
	// for the six base attributes (s must be one of stat.StatSTR..StatMEN;
	// any other Stat returns 0).
	HennaBonus(s stat.Stat) float64

	// HasEquipped reports whether some item currently occupies any of the
	// given paperdoll slot bits (a caller ORs slot bits together the same
	// way the reference Paperdoll enum groups a paired slot).
	HasEquipped(slotMask int) bool

	// HasWeaponEquipped reports whether the player currently wields a
	// weapon (an empty-handed player is treated distinctly by a couple of
	// funcs).
	HasWeaponEquipped() bool
}

// Paperdoll slot bits, matching the reference Paperdoll enum's ordinal
// positions that FuncMDefMod/FuncPDefMod key off of. These intentionally
// duplicate model/item's Slot bit values rather than importing that
// package, since this package only ever needs to pass a slot identity
// through PlayerActor.HasEquipped, never interpret one itself.
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
// all run at basefunc.OrderFinalize, are attached with no owner, no
// configured value, and no gating Condition — matching the reference
// classes, which are all constructed as
// super(null, <stat>, 10, 0, null).
type fixed struct{ s stat.Stat }

func (f fixed) Stat() stat.Stat          { return f.s }
func (f fixed) Order() int               { return basefunc.OrderFinalize }
func (f fixed) Owner() any               { return nil }
func (f fixed) Value() float64           { return 0 }
func (f fixed) Cond() basefunc.Condition { return nil }

// actorOf asserts effector to Actor. Every func in this package is only
// ever attached to a real creature's calculation chain, so a failed
// assertion indicates a wiring bug, not a legitimate runtime case — it
// panics, matching how the reference implementation would throw a
// ClassCastException/NullPointerException from the equivalent misuse.
func actorOf(effector any) Actor {
	return effector.(Actor)
}
