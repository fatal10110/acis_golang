package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// positioned is an actor whose place in the world is known.
type positioned interface {
	Position() (x, y, z int)
}

// siegeZoneMember is an actor that tracks whether it stands in an active
// siege zone.
type siegeZoneMember interface {
	InSiegeZone() bool
}

// DefendsSiegeAt reports whether c's clan owns or defends the siege in
// progress whose battlefield holds the point (x, y, z). A siege guard may
// not attack a playable of c's standing there.
func (c *Character) DefendsSiegeAt(x, y, z int) bool {
	clanID := c.ClanID()
	return clanID != 0 && c.sieges != nil && c.sieges.DefenderAt(clanID, x, y, z)
}

// inSameActiveSiegeSide reports whether self, a playable acting through c,
// and the player acting through other both stand in an active siege zone,
// a siege is in progress where self stands, and their clans are not on
// opposite sides of it. A player without a clan, or of a clan registered on
// no side, is on the opposite side of nobody.
func (c *Character) inSameActiveSiegeSide(self attackable.ArenaMember, other target.Actor) bool {
	if c.sieges == nil || self == nil || !self.InSiegeZone() {
		return false
	}
	peer, ok := socialPeerOf(other)
	if !ok {
		return false
	}
	if zoned, ok := peer.(siegeZoneMember); !ok || !zoned.InSiegeZone() {
		return false
	}
	at, ok := self.(positioned)
	if !ok {
		return false
	}
	x, y, z := at.Position()
	return c.sieges.SameSideAt(c.ClanID(), peer.ClanID(), x, y, z)
}
