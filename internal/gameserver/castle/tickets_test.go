package castle

import (
	"slices"
	"testing"
)

// TestDroppedTicketsNewestFirstOrder pins the order a castle's dropped
// tickets hire their mercenaries in: the item that entered a container last
// first, then, at the same entry time, the higher object id first. Listing
// them keeps them counted.
func TestDroppedTicketsNewestFirstOrder(t *testing.T) {
	var c Castle
	for _, d := range []DroppedTicket{
		{ObjectID: 10, Time: 2000},
		{ObjectID: 30, Time: 1000},
		{ObjectID: 20, Time: 2000},
		{ObjectID: 40, Time: 500},
	} {
		c.AddDroppedTicket(d)
	}
	var got []int32
	for _, d := range c.DroppedTicketsNewestFirst() {
		got = append(got, d.ObjectID)
	}
	if want := []int32{20, 10, 30, 40}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if n := c.DroppedTickets(); n != 4 {
		t.Fatalf("%d tickets left, want all 4", n)
	}
}

// fakeGround is a Claimable whose single claim is a flag.
type fakeGround struct{ claimed bool }

func (g *fakeGround) Claim() bool {
	if g.claimed {
		return false
	}
	g.claimed = true
	return true
}

func (g *fakeGround) Release() { g.claimed = false }

// TestDecayDroppedTicketHeldLeavesOnLetGo: the owner's removal claims and
// forgets a free ticket, passes over a held one, which stays counted until
// its pickup lets it go; the let-go then keeps the claim and forgets it.
// A ticket the removal never passed over is put back on the ground.
func TestDecayDroppedTicketHeldLeavesOnLetGo(t *testing.T) {
	var c Castle
	free, held, other := &fakeGround{}, &fakeGround{claimed: true}, &fakeGround{claimed: true}
	c.AddDroppedTicket(DroppedTicket{ObjectID: 1})
	c.AddDroppedTicket(DroppedTicket{ObjectID: 2})
	c.AddDroppedTicket(DroppedTicket{ObjectID: 3})

	if !c.DecayDroppedTicket(1, free) || !free.claimed {
		t.Fatal("a free ticket was not claimed for the removal")
	}
	if c.DecayDroppedTicket(2, held) {
		t.Fatal("a held ticket was claimed for the removal")
	}
	if n := c.DroppedTickets(); n != 2 {
		t.Fatalf("%d tickets after the removal, want the held one and the other", n)
	}
	if !c.LetGoTicket(2, held) || !held.claimed {
		t.Fatal("the held ticket's let-go did not keep the claim for its removal")
	}
	if c.LetGoTicket(3, other) || other.claimed {
		t.Fatal("a ticket the removal never passed over was not put back")
	}
	if n := c.DroppedTickets(); n != 1 {
		t.Fatalf("%d tickets after the let-go, want only the other", n)
	}
}

// TestRemoveDroppedTicketClearsDecay: a held ticket its pickup takes is
// forgotten, and a later let-go of the same id puts it back.
func TestRemoveDroppedTicketClearsDecay(t *testing.T) {
	var c Castle
	held := &fakeGround{claimed: true}
	c.AddDroppedTicket(DroppedTicket{ObjectID: 1})
	c.DecayDroppedTicket(1, held)
	c.RemoveDroppedTicket(1)
	if c.LetGoTicket(1, held) || held.claimed {
		t.Fatal("a picked-up ticket still left the world on a let-go")
	}
}
