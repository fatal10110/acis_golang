package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// raidCurse has n curse c when c stands more than 8 levels above it.
func raidCurse(n *script.NPC, c script.Creature) {
	if c.Level() > n.Level()+8 {
		n.AddCastDesire(c, skill.Ref{ID: 4515, Level: 1}, 1000000)
	}
}

// raidMute has n mute c when c stands more than 8 levels above it, and
// reports whether it did.
func raidMute(n *script.NPC, c script.Creature) bool {
	if c.Level() > n.Level()+8 {
		n.AddCastDesire(c, skill.Ref{ID: 4215, Level: 1}, 1000000)
		return true
	}
	return false
}
