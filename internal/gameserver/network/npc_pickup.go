package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/cursedweapon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
)

var _ npc.GroundTaker = (*GameClientLink)(nil)

// TakeGround implements npc.GroundTaker: an NPC picked up ground, which it
// has claimed. Any item but a cursed weapon leaves the world, as one the
// ground cleanup expires does. A cursed weapon instead ends its life
// wherever it is, and every player is told it disappeared: the ground item
// leaves the world when it is the weapon that lay there, and stays on the
// ground otherwise (a weapon held, or not out at all, ends without it).
func (l *GameClientLink) TakeGround(ground *grounditem.Item) {
	if !l.isCursedWeapon(ground.ItemID()) || l.cursed == nil {
		l.despawnGround(ground)
		return
	}
	end, ok := l.cursed.Expire(ground.ItemID())
	if !ok {
		end = cursedweapon.EndOfLife{ItemID: ground.ItemID()}
	}
	if end.GroundObjectID == ground.ObjectID() {
		// The NPC holds the claim the end's own removal would take.
		l.despawnGround(ground)
		end.GroundObjectID = 0
	} else {
		ground.Release()
	}
	l.endCursedWeapon(end, nil)
}

// despawnGround takes the claimed ground item out of the world.
func (l *GameClientLink) despawnGround(ground *grounditem.Item) {
	if l.belongings.groundItems != nil {
		l.belongings.groundItems.Remove(ground)
	}
	if l.world != nil {
		l.world.Despawn(ground)
	}
}
