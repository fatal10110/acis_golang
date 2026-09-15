package attackable

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"

// Combatant is any creature that can take part in combat: the NPC that owns
// a threat or hate table, the creatures registered in it, and every attack,
// AI and skill target. Players, NPCs and summons implement every method; a
// method that does not apply to a kind returns the documented neutral value
// at the implementation.
type Combatant interface {
	ObjectID() int32
	Kind() actor.Kind
	Position() (x, y, z int)
	Heading() int
	CollisionRadius() float64
	CollisionHeight() float64
	// CharacterName is the display name system messages use for the combatant.
	CharacterName() string
	Level() int

	// Karma is the combatant's PK karma; 0 for kinds that carry none.
	Karma() int

	Dead() bool
	// AlikeDead reports whether this combatant is dead or in a
	// dead-equivalent state (e.g. fake death) and should no longer be
	// selected as a threat target.
	AlikeDead() bool
	// FakeDeath reports whether the combatant is currently feigning death.
	FakeDeath() bool
	// RecentFakeDeath reports whether the combatant is inside the stand-up
	// grace period that follows fake death.
	RecentFakeDeath() bool

	IsMoving() bool
	MovementDisabled() bool
	InPeaceZone() bool
	// EffectRangeInPeaceZone reports whether an effect of effectRange centered
	// on (x, y, z) overlaps a peace zone in the combatant's region.
	EffectRangeInPeaceZone(x, y, z, effectRange int) bool
	// SilentMoving reports whether the combatant moves unseen by aggressive
	// NPCs outside their close range.
	SilentMoving() bool
	// Knows reports whether other is in this combatant's known list.
	Knows(other Combatant) bool
	SpawnProtected() bool
	// CanGiveDamage reports whether the combatant may damage others; only
	// access-level restrictions revoke it.
	CanGiveDamage() bool
	RaidRelated() bool

	// SiegeGuard reports whether this combatant is a defensive siege guard.
	// Guards never build threat against each other.
	SiegeGuard() bool
	// Guard reports whether this combatant is a town guard.
	Guard() bool

	// Owner returns the controlling player of a summon; (nil, false) for
	// every other kind.
	Owner() (Combatant, bool)
}
