package player

// The siege states: what a player's UserInfo and relations show of the
// side its clan fights on in a siege under way.
const (
	SiegeStateNone     int32 = 0
	SiegeStateAttacker int32 = 1
	SiegeStateDefender int32 = 2
)

// SiegeState is the character's siege state.
func (c *Character) SiegeState() int32 { return c.siegeState.Load() }

// SetSiegeState sets the character's siege state. It sends nothing by
// itself: the caller resends the UserInfo and the relations.
func (c *Character) SetSiegeState(s int32) { c.siegeState.Store(s) }
