package network

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// mercenaryTicketsHandler is the etc-item handler name of the mercenary
// posting tickets.
const mercenaryTicketsHandler = "MercenaryTickets"

// castleAt returns the castle whose siege zone holds the point (x, y, z),
// being besieged or not.
func (l *GameClientLink) castleAt(x, y, z int) (*castle.Castle, bool) {
	s, ok := l.sieges.At(x, y, z)
	if !ok {
		return nil, false
	}
	return s.Castle(), true
}

// castleLord reports whether live leads the clan owning c.
func castleLord(live *livePlayer, c *castle.Castle) bool {
	return live.IsClanLeader() && c.OwnerID() != 0 && c.OwnerID() == live.ClanID()
}

// castleTicketAt reports whether itemID is a mercenary ticket of the castle
// whose grounds hold the point (x, y, z).
func (l *GameClientLink) castleTicketAt(itemID int32, x, y, z int) bool {
	c, ok := l.castleAt(x, y, z)
	if !ok {
		return false
	}
	_, ok = c.Ticket(int(itemID))
	return ok
}

// wireCastleTickets keeps a castle's mercenary tickets on its grounds until
// picked up, and gives every castle the tickets restored on its grounds at
// boot.
func (l *GameClientLink) wireCastleTickets() {
	if l.groundItems == nil || l.sieges == nil {
		return
	}
	l.groundItems.SetKept(l.castleTicketAt)
	l.groundItems.Each(func(ground *grounditem.Item) {
		x, y, z := ground.Position()
		c, ok := l.castleAt(x, y, z)
		if !ok {
			return
		}
		if _, ok := c.Ticket(int(ground.ItemID())); ok {
			c.AddDroppedTicket(castle.DroppedTicket{ObjectID: ground.ObjectID(), ItemID: int(ground.ItemID()), Location: location.Location{X: x, Y: y, Z: z}})
		}
	})
}

// useMercenaryTicket places live's mercenary ticket inst on the grounds of
// the castle live stands in, and reports whether inst is one at all. Only
// the castle's lord places, only the castle's own tickets, never while the
// castle is besieged, only in the seal validation period, only a ticket
// open to the Seal of Strife's owner, no more of one ticket than it allows
// and never within 25 units of another ticket. One ticket then drops at
// live's feet and the castle counts it.
//
// Outside any castle, and when the drop fails, nothing answers: the use
// leaves the client waiting on nothing, and the reference is silent too.
func (l *GameClientLink) useMercenaryTicket(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) bool {
	if tmpl.EtcItem == nil || tmpl.EtcItem.Handler != mercenaryTicketsHandler {
		return false
	}
	x, y, z := live.Position()
	c, ok := l.castleAt(x, y, z)
	if !ok {
		return true
	}
	if !castleLord(live, c) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNoAuthorityToPositionMercenaries))
		return true
	}
	itemID := int(inst.TemplateID)
	ticket, ok := c.Ticket(itemID)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMercenariesCannotBePositionedHere))
		return true
	}
	if s, ok := l.castleSiege(int32(c.ID)); ok && s.InProgress() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMercenaryCannotBePositionedAnymore))
		return true
	}
	if !l.SealValidationPeriod() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMercCanBeAssigned))
		return true
	}
	strifeOwner := sevensigns.NoCabal
	if l.sevenSigns != nil {
		strifeOwner = l.sevenSigns.SealOwners()[sealStrifeIndex]
	}
	if !slices.Contains(ticket.SSQ, strifeOwner.String()) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMercCantBeAssignedUsingStrife))
		return true
	}
	if c.DroppedTicketCount(itemID) >= ticket.MaxAmount {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMercenaryCannotBePositionedAnymore))
		return true
	}
	if c.TooCloseToDroppedTicket(location.Location{X: x, Y: y, Z: z}) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMercenariesTooCloseToPosition))
		return true
	}
	if l.groundItems == nil {
		return true
	}
	ground, ok := l.dropToGround(live, inv, inst.ObjectID, 1, x, y, z)
	if !ok {
		return true
	}
	gx, gy, gz := ground.Position()
	c.AddDroppedTicket(castle.DroppedTicket{ObjectID: ground.ObjectID(), ItemID: itemID, Location: location.Location{X: gx, Y: gy, Z: gz}})
	live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessagePlaceS1InCurrentLocationAndDirection, inst.TemplateID))
	return true
}

// refuseTicketPickup answers live's click on a castle guard item it may not
// pick up, and reports whether it did. Such an item is picked up only out
// of a party, on the grounds of a castle it is a ticket of, by that
// castle's lord.
func (l *GameClientLink) refuseTicketPickup(live *livePlayer, ground *grounditem.Item) bool {
	if ground.Template == nil || ground.Template.EtcItem == nil || ground.Template.EtcItem.Type != item.EtcItemCastleGuard {
		return false
	}
	if l.parties != nil && l.parties.InParty(live.ObjectID()) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	x, y, z := live.Position()
	c, ok := l.castleAt(x, y, z)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if _, ok := c.Ticket(int(ground.ItemID())); !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if !castleLord(live, c) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotMercenaryOfOwnCastleCannotCancelPositioning))
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	return false
}

// forgetPickedTicket has the castle whose grounds held ground, picked up,
// forget it as a mercenary ticket.
func (l *GameClientLink) forgetPickedTicket(ground *grounditem.Item) {
	if l.sieges == nil {
		return
	}
	x, y, z := ground.Position()
	if c, ok := l.castleAt(x, y, z); ok {
		c.RemoveDroppedTicket(ground.ObjectID())
	}
}

// decayDroppedTickets takes every mercenary ticket on c's grounds out of
// the world. A ticket being picked up meanwhile is left to its picker.
func (l *GameClientLink) decayDroppedTickets(c *castle.Castle) {
	for _, t := range c.TakeDroppedTickets() {
		ground, ok := l.resolveTarget(t.ObjectID).(*grounditem.Item)
		if !ok || !ground.Claim() {
			continue
		}
		l.despawnGround(ground)
	}
}
