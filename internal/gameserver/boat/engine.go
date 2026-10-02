package boat

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// scheduleStep is where a boat stands in its schedule.
type scheduleStep uint8

const (
	// statePreparing announces the departure schedule at the dock.
	statePreparing scheduleStep = iota
	// stateExecuteRoute sets the boat off on its route.
	stateExecuteRoute
	// stateSealing sails: the schedule waits for the boat to arrive.
	stateSealing
	// stateReadyToMoveToDock waits off shore for the destination dock.
	stateReadyToMoveToDock
	// stateDocked ties the boat up at its destination.
	stateDocked
)

// announceRadius is how far from either dock of an itinerary its schedule
// is heard.
const announceRadius = 20000

// busyShoutPeriod is how many busy-dock checks, five seconds apart, pass
// between two announcements that the destination dock is held.
const busyShoutPeriod = 36

// Boat sounds.
const (
	soundLeaveFiveMinutes = "itemsound.ship_5min"
	soundLeaveOneMinute   = "itemsound.ship_1min"
	soundArrivalDeparture = "itemsound.ship_arrival_departure"
)

// engine runs one itinerary's boat through its schedule. It is owned by its
// fleet's tick goroutine.
type engine struct {
	boat     *Boat
	docks    *docks
	legs     []leg // one leg for a one-way itinerary, two for a round trip
	audience event.BoatAudience

	// soundAt is where the boat's sounds play: where the boat was spawned.
	soundAt location.Location

	waitDelay  int
	delay      int
	shoutCount int

	state       scheduleStep
	current     route.Dock
	destination route.Dock
	messages    []announcement
	next        int
}

// newEngine builds the schedule of b, about to be spawned at spawn.
func newEngine(b *Boat, legs []leg, d *docks, spawn location.Location) *engine {
	e := &engine{boat: b, docks: d, legs: legs, soundAt: spawn}
	e.current = legs[0].dock
	e.destination = e.current
	if len(legs) > 1 {
		e.destination = legs[1].dock
	}
	e.waitDelay = 300
	if e.current == route.DockRune && e.destination == route.DockPrimeval {
		e.waitDelay = 0
	}
	e.delay = e.waitDelay
	for _, l := range legs {
		e.audience.Centers = append(e.audience.Centers, dockSites[l.dock].at)
	}
	e.audience.Radius = announceRadius
	e.messages = e.leg(e.current).schedule
	b.engine = e
	return e
}

func (e *engine) leg(d route.Dock) *leg {
	for i := range e.legs {
		if e.legs[i].dock == d {
			return &e.legs[i]
		}
	}
	return nil
}

// announce tells the itinerary's audience the system messages ids, if any.
func (e *engine) announce(ids []int) {
	if len(ids) == 0 {
		return
	}
	e.boat.emit(event.BoatAnnounced{Audience: e.audience, MessageIDs: ids})
}

func (e *engine) sound(file string) {
	e.boat.emit(event.BoatSounded{Audience: e.audience, Sound: file, At: e.soundAt})
}

// canRun counts one second off the engine's delay and reports whether the
// delay has run out.
func (e *engine) canRun() bool {
	if e.delay > 0 {
		e.delay--
	}
	return e.delay == 0
}

// run takes the schedule's next step.
func (e *engine) run() {
	switch e.state {
	case statePreparing:
		if e.next >= len(e.messages) {
			e.state = stateExecuteRoute
			return
		}
		step := e.messages[e.next]
		e.next++
		e.announce(step.messages)
		e.delay = step.delay
		switch e.delay {
		case 240:
			e.sound(soundLeaveFiveMinutes)
		case 40, 20:
			e.sound(soundLeaveOneMinute)
		}

	case stateExecuteRoute:
		l := e.leg(e.current)
		// ponytail: fare collection from passengers lands with boarding
		// (#602); until a player can board, a departure has no one to charge.
		e.boat.executePath(l.path)
		e.docks.setBusy(l.dock, false)
		e.sound(soundArrivalDeparture)
		e.state = stateSealing

	case stateReadyToMoveToDock:
		if e.docks.busy(e.destination) {
			e.delay = 5
			if e.shoutCount == 0 {
				e.announceBusy()
			}
			e.shoutCount++
			if e.shoutCount >= busyShoutPeriod {
				e.shoutCount = 0
			}
			e.shoutCount = (e.shoutCount + 1) % busyShoutPeriod
			return
		}
		e.docks.setBusy(e.destination, true)
		path := e.leg(e.current).path
		e.boat.moveTo(path[len(path)-1])
		e.state = stateSealing

	case stateDocked:
		e.delay = e.waitDelay
		e.sound(soundArrivalDeparture)
		e.current, e.destination = e.destination, e.current
		e.messages, e.next = e.leg(e.current).schedule, 0
		e.boat.emit(event.BoatShown{})
		e.state = statePreparing
	}
}

// announceBusy tells the audience the boat waits for its destination dock,
// with the line its route sets for that.
func (e *engine) announceBusy() {
	if busy := e.leg(e.destination).busy; busy != 0 {
		e.announce([]int{busy})
	}
}
