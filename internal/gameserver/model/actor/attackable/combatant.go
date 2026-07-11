package attackable

import "github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"

// Combatant is the subset of a creature's behavior the threat and hate
// tables need. Both the NPC that owns a table and the creatures registered
// in it satisfy this interface.
type Combatant interface {
	worldobject.Object

	// SiegeGuard reports whether this combatant is a defensive siege guard.
	// Guards never build threat against each other.
	SiegeGuard() bool

	// AlikeDead reports whether this combatant is dead or in a
	// dead-equivalent state (e.g. fake death) and should no longer be
	// selected as a threat target.
	AlikeDead() bool
}
