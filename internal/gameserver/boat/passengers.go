package boat

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// fareDelay is how many position updates pass between a departure and its
// ticket collection: five seconds.
const fareDelay = 5 * updatesPerStep

// fareDue is a ticket collection waiting to run: in ticks position updates,
// every passenger gives up one itemID ticket or is put ashore at oust.
// ticks 0 means none is due.
type fareDue struct {
	ticks  int
	itemID int
	oust   location.Location
}

// Dock returns the dock b serves: the one it is tied up at, or the one it
// last left.
func (b *Boat) Dock() *Dock {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dock
}

// OustLocation is the shore point of the dock b serves.
func (b *Boat) OustLocation() location.Location { return b.Dock().OustLocation() }

// Moving reports whether b is under way.
func (b *Boat) Moving() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.moving
}

func (b *Boat) setDock(d *Dock) {
	b.mu.Lock()
	b.dock = d
	b.mu.Unlock()
}

// AddPassenger takes the player objectID aboard and reports whether it was
// not aboard already.
func (b *Boat) AddPassenger(objectID int32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if slices.Contains(b.passengers, objectID) {
		return false
	}
	b.passengers = append(b.passengers, objectID)
	return true
}

// RemovePassenger puts the player objectID off b. Any passenger leaving
// calls off the ticket collection due.
func (b *Boat) RemovePassenger(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fare = fareDue{}
	b.passengers = slices.DeleteFunc(b.passengers, func(id int32) bool { return id == objectID })
}

// DropPassenger forgets the player objectID, who left the world aboard,
// leaving any ticket collection due in place.
func (b *Boat) DropPassenger(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.passengers = slices.DeleteFunc(b.passengers, func(id int32) bool { return id == objectID })
}

// Passengers returns the object ids of the players aboard, in boarding
// order.
func (b *Boat) Passengers() []int32 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.passengers)
}

// chargeFare calls off any ticket collection due and, when the leg sets a
// ticket, schedules the next one, fareDelay updates from now.
func (b *Boat) chargeFare(itemID int, oust location.Location) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fare = fareDue{}
	if itemID <= 0 {
		return
	}
	b.fare = fareDue{ticks: fareDelay, itemID: itemID, oust: oust}
}

// tickFare counts one position update off the ticket collection due, and
// runs it when its time comes.
func (b *Boat) tickFare() {
	b.mu.Lock()
	if b.fare.ticks == 0 {
		b.mu.Unlock()
		return
	}
	b.fare.ticks--
	if b.fare.ticks > 0 {
		b.mu.Unlock()
		return
	}
	due := b.fare
	b.fare = fareDue{}
	passengers := slices.Clone(b.passengers)
	b.mu.Unlock()
	if len(passengers) > 0 {
		b.emit(event.BoatFareDue{Passengers: passengers, ItemID: due.itemID, Oust: due.oust})
	}
}

// carryPassengers tells the players aboard where b now stands.
func (b *Boat) carryPassengers() {
	passengers := b.Passengers()
	if len(passengers) == 0 {
		return
	}
	b.emit(event.BoatCarried{Passengers: passengers, At: b.location(), Heading: b.Heading()})
}
