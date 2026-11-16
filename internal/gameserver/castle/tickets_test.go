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
