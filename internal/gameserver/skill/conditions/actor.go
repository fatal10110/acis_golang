// Package conditions provides the checks that gate whether a skill or item
// effect is allowed to apply: level/hp/mp/weight thresholds, player state
// (moving, riding, resting, …), target identity/race/hp, and the day/night
// or dice-roll gates a few skills use. Each concrete condition here is a
// basefunc.Condition; combine them with And/Or/Not the same way skill/item
// data attaches more than one requirement to the same effect.
package conditions

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Actor is the live combat/identity data every condition in this package
// can read from either side (effector or effected) of a test — a creature,
// in the reference implementation's terms. It stands in for the not-yet-
// built creature runtime; a future concrete actor type satisfies it
// structurally.
type Actor interface {
	Level() int
	HPRatio() float64 // current HP / max HP, in [0,1]
	MPRatio() float64 // current MP / max MP, in [0,1]

	X() int
	Y() int
	Z() int

	IsMoving() bool
	IsRunning() bool
	IsRiding() bool
	IsFlying() bool
	IsBehind(other Actor) bool
	IsInFrontOf(other Actor) bool

	// ActiveSkillLevel looks up a skill of id known/active on this actor,
	// returning its level and whether it was found at all.
	ActiveSkillLevel(id int) (level int, ok bool)

	// ActiveEffectLevel looks up the level of the skill backing an active
	// effect of id on this actor, returning it and whether one is active.
	ActiveEffectLevel(effectID int) (level int, ok bool)
}

// PlayerActor narrows Actor to the extra identity/state data only a
// player-controlled actor carries. A condition type-asserts effector or
// effected to this interface exactly where the reference implementation
// checks "instanceof Player".
type PlayerActor interface {
	Actor

	IsSitting() bool
	IsInOlympiadMode() bool
	IsHero() bool
	PkKills() int
	PledgeClass() int
	IsClanLeader() bool

	// HasClan, ClanCastleID, ClanHasAnyCastle, ClanHallID and
	// ClanHasAnyClanHall answer the clan-ownership checks
	// ConditionPlayerHasCastle/HasClanHall need without requiring a full
	// clan model here: ClanCastleID/ClanHallID are 0 when the clan owns
	// none of that kind.
	HasClan() bool
	ClanCastleID() int
	ClanHasAnyCastle() bool
	ClanHallID() int
	ClanHasAnyClanHall() bool

	Race() player.Race
	Sex() player.Sex

	// WeightPenalty is the ordinal of the player's current weight-penalty
	// tier (0 = none, increasing with how overloaded the inventory is).
	WeightPenalty() int

	InventorySize() int
	InventoryLimit() int

	Charges() int

	// IsWearingType reports whether an equipped item's type mask
	// intersects mask (a bitwise-OR of weapon/armor type bits).
	IsWearingType(mask int) bool
}

// Skill is what ConditionSkillStats needs from the skill under test.
type Skill interface {
	Stat() stat.Stat
}
