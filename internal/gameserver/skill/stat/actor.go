package stat

// Actor is the live combat data every basefunc.Func/Condition reads from
// its effector. It stands in for the not-yet-built creature runtime; a
// future concrete actor type satisfies it structurally.
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
// player's class is a mage class. Funcs that need player-only data simply
// skip their player branch when effector does not satisfy this interface.
type PlayerActor interface {
	Actor

	IsMageClass() bool

	// HennaBonus returns the flat bonus the player's applied hennas grant
	// for the six base attributes (s must be one of StatSTR..StatMEN; any
	// other Stat returns 0).
	HennaBonus(s Stat) float64

	// HasEquipped reports whether some item currently occupies any of the
	// given equipment slot bits. Callers may OR paired slots together.
	HasEquipped(slotMask int) bool

	// HasWeaponEquipped reports whether the player currently wields a
	// weapon (an empty-handed player is treated distinctly by a couple of
	// funcs).
	HasWeaponEquipped() bool
}
