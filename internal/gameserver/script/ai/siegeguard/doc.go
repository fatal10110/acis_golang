// Package siegeguard contains the castle siege guard behaviors: the
// clerics that heal a hurt clan mate, the wizards that cast at whoever they
// see, GludioHold, the guard that holds the castle grounds against anyone
// but its castle's defenders and goes back to its post when nothing is left
// to do, GludioStand, the guard that fights from its post, and the guards
// derived from them.
package siegeguard

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The page directories of the siege guard behaviors.
const (
	dir       = "ai/siegeguards"
	holdDir   = "ai/siegeguards/GludioHold"
	standDir  = "ai/siegeguards/GludioStand"
	wizardDir = "ai/siegeguards/GludioWizard"
)

// defender is where a player stands towards the guard's castle when its
// clan defends or owns it (defaultnpc.PledgeCastleState): the guards spare
// such a player.
const defender = 2

// sealOfRuler is the skill a clan leader engraves a castle's artefact with.
func sealOfRuler() skill.Ref { return skill.Ref{ID: 246, Level: 1} }

// spared reports whether n spares c: c's player defends or owns n's
// castle.
func spared(s *script.Script, n *script.NPC, c script.Creature) bool {
	return defaultnpc.PledgeCastleState(s, n, c) == defender
}

// above reports whether c stands more than 100 above n.
func above(c script.Creature, n *script.NPC) bool {
	return script.LocOf(c).Z > script.LocOf(n).Z+100
}

// damageWeight is the attack desire weight of points against n: scale for
// each twentieth of n's maximum hit points they come to.
func damageWeight(n *script.NPC, points int, scale float64) float64 {
	return float64(points) / float64(n.MaxHP()) / 0.05 * scale
}
