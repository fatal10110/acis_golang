package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// teleportSummon is a script's ordinary teleport of actor to at, on its
// queue: the summon stops what it was doing and drops its target, lands on
// the ground at at unless at lies in water, its observers see the jump with
// its fade, it leaves the world grid and re-enters it at the landing point,
// follows its owner again, and the NPCs around the landing that see it
// notice it. A summon in a teleport already, or gone from the world, is
// left as it is.
func (l *GameClientLink) teleportSummon(actor *summon.Actor, at location.Location) {
	if l.world == nil || !actor.Spawned() || !actor.SetTeleporting(true) {
		return
	}
	target := l.landing(location.Jump{To: at}, l.inWater)
	owner, _ := l.currentSummonOwner(actor)
	landed := l.jumpSummon(owner, actor, target)
	actor.SetTeleporting(false)
	if landed {
		npc.ForceSeeCreature(l.world, actor)
	}
}

// jumpSummon moves actor to destination in a teleport, its own or the one
// that follows its owner's, and reports whether it landed; owner, nil when
// gone, is shown its crest on landing.
func (l *GameClientLink) jumpSummon(owner *livePlayer, actor *summon.Actor, destination location.Location) bool {
	// The summon leaves the zones around its old position before the jump
	// and enters those at the destination once it has landed.
	actor.LeaveZones()
	// Leaving its zones takes a siege summon off its battlefield, so its
	// jump dismisses it wherever it lands, and its owner hears it vanished.
	vanishes := actor.OffBattlefield()
	actor.AbortForTeleport()
	// The summon's jump is a teleport of its own: hostiles around its old
	// position forget it too.
	npc.DropThreatAround(l.world, actor)
	l.broadcastSummonFrame(actor, serverpackets.FrameTeleportToLocation(actor.ObjectID(), destination, false))
	if vanishes {
		// Dismissed before it lands: nobody at its landing point, its owner
		// included, is ever shown it there. A corpse cannot be dismissed,
		// so it only leaves the grid; its owner is told it vanished all
		// the same. The dismissal its battlefield exit posted then finds it
		// gone.
		if actor.Dead() {
			l.world.Leave(actor)
		}
		actor.Vanish()
		return false
	}
	actor.Move().SetPosition(destination)
	_ = l.world.Teleport(actor, destination.X, destination.Y, destination.Z)
	actor.EnterZones()
	actor.OnTeleported()
	l.showSummonCrestToOwner(owner, actor)
	return true
}
