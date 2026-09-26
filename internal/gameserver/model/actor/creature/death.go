package creature

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// Rewarder computes and applies kill rewards (drops, experience, karma) for
// a defeated actor. Concrete actor types own their reward logic — the drop
// table and experience/SP systems land separately — so a nil Rewarder makes
// the reward step a no-op rather than an error.
type Rewarder interface {
	CalculateRewards(killer attackable.Combatant)
}
