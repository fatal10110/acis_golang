package castle

import (
	"cmp"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
)

// ticketSpacing is how close, in 3D, a mercenary ticket may not be placed
// to one already on the castle's grounds.
const ticketSpacing = 25

// DroppedTicket is a mercenary ticket lying on a castle's grounds: the
// ground item's object id, its item id, where it lies, and when the item
// last entered a container (its entry time, in Unix milliseconds).
type DroppedTicket struct {
	ObjectID int32
	ItemID   int
	location.Location
	Time int64
}

// Ticket returns the castle's mercenary ticket for itemID.
func (c *Castle) Ticket(itemID int) (castledata.Ticket, bool) {
	for _, t := range c.Tickets {
		if t.ItemID == itemID {
			return t, true
		}
	}
	return castledata.Ticket{}, false
}

// DroppedTickets is how many mercenary tickets lie on the castle's grounds.
func (c *Castle) DroppedTickets() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.droppedTickets)
}

// DroppedTicketCount is how many tickets of itemID lie on the castle's
// grounds.
func (c *Castle) DroppedTicketCount(itemID int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.droppedTickets {
		if t.ItemID == itemID {
			n++
		}
	}
	return n
}

// TooCloseToDroppedTicket reports whether a ticket lies within 25 units of
// at, in 3D.
func (c *Castle) TooCloseToDroppedTicket(at location.Location) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.droppedTickets {
		if t.In3DRadius(at, ticketSpacing) {
			return true
		}
	}
	return false
}

// AddDroppedTicket records t as lying on the castle's grounds.
func (c *Castle) AddDroppedTicket(t DroppedTicket) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.droppedTickets == nil {
		c.droppedTickets = make(map[int32]DroppedTicket)
	}
	c.droppedTickets[t.ObjectID] = t
}

// RemoveDroppedTicket forgets the ticket objectID, picked up.
func (c *Castle) RemoveDroppedTicket(objectID int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.droppedTickets, objectID)
	delete(c.decayOnLetGo, objectID)
}

// Claimable is a ground item whatever takes it out of the world claims
// first; only one claim ever succeeds, and a claim that took nothing is let
// go (Release).
type Claimable interface {
	Claim() bool
	Release()
}

// DecayDroppedTicket claims g, the ticket objectID on the castle's grounds,
// for the owner's removal, forgets it and reports true: the caller takes g
// out of the world. A ticket a pickup holds stays counted and reports
// false; if that pickup lets it go (LetGoTicket), it leaves then.
func (c *Castle) DecayDroppedTicket(objectID int32, g Claimable) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !g.Claim() {
		if c.decayOnLetGo == nil {
			c.decayOnLetGo = make(map[int32]struct{})
		}
		c.decayOnLetGo[objectID] = struct{}{}
		return false
	}
	delete(c.droppedTickets, objectID)
	delete(c.decayOnLetGo, objectID)
	return true
}

// LetGoTicket lets go of g, the ground item objectID on the castle's
// grounds that a pickup claimed and took nothing from. A ticket the owner's
// removal passed over keeps the claim instead: the castle forgets it and
// reports true, and the caller takes g out of the world.
func (c *Castle) LetGoTicket(objectID int32, g Claimable) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, decay := c.decayOnLetGo[objectID]; decay {
		delete(c.decayOnLetGo, objectID)
		delete(c.droppedTickets, objectID)
		return true
	}
	g.Release()
	return false
}

// DroppedTicketsNewestFirst returns every ticket on the castle's grounds,
// the latest entry time first, then the highest object id.
func (c *Castle) DroppedTicketsNewestFirst() []DroppedTicket {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]DroppedTicket, 0, len(c.droppedTickets))
	for _, t := range c.droppedTickets {
		out = append(out, t)
	}
	slices.SortFunc(out, func(a, b DroppedTicket) int {
		return cmp.Or(cmp.Compare(b.Time, a.Time), cmp.Compare(b.ObjectID, a.ObjectID))
	})
	return out
}

// AddMercenary records the NPC objectID as a mercenary the castle's siege
// hired.
func (c *Castle) AddMercenary(objectID int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mercenaries = append(c.mercenaries, objectID)
}

// TakeMercenaries forgets the castle's hired mercenaries and returns their
// object ids, in hiring order.
func (c *Castle) TakeMercenaries() []int32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.mercenaries
	c.mercenaries = nil
	return out
}
