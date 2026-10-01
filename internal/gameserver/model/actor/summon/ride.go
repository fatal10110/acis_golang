package summon

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"

// Hungry reports whether this pet is fed below its hungry limit. A servitor
// is never hungry.
func (a *Actor) Hungry() bool { return a.hungryHalved() }

// Rooted reports whether a root effect holds this summon in place.
func (a *Actor) Rooted() bool { return a.effects.IsAffected(effect.FlagRooted) }
