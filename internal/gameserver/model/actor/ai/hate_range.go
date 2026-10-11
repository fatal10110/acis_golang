package ai

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// RemoveHatesOutOfRange drops every skill-cast hate entry whose creature
// stands distance or farther from the actor, in 3D, centre to centre, and
// the cast desires aimed at those creatures. Nothing else changes: the
// attack desires, the current intention and the threat table stay as they
// are.
func (a *Attackable) RemoveHatesOutOfRange(distance int) {
	if a.hates.IsEmpty() {
		return
	}
	ox, oy, oz := a.actor.Position()
	origin := location.Location{X: ox, Y: oy, Z: oz}
	far := func(c attackable.Combatant) bool {
		x, y, z := c.Position()
		return origin.Distance3D(location.Location{X: x, Y: y, Z: z}) >= float64(distance)
	}
	for _, e := range a.hates.Snapshot() {
		if far(e.Attacker) {
			a.desires.Remove(IntentionCast, e.Attacker)
		}
	}
	a.hates.RemoveIf(far)
}
