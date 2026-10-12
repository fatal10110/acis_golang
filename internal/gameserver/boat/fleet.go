package boat

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// MoveInterval is how often a sailing boat's position updates.
const MoveInterval = 100 * time.Millisecond

// updatesPerStep is how many position updates pass between two schedule
// steps: the schedules count in seconds.
const updatesPerStep = int(time.Second / MoveInterval)

// IDAllocator hands out world object ids.
type IDAllocator interface {
	NextID() (int32, error)
}

// Fleet owns every scheduled boat and the docks they share. Tick, and so
// every schedule and movement change, runs under mu: on the fleet's ticker
// in production, on the test in a suite. A reload retires the boats and
// launches new ones under it too.
type Fleet struct {
	ids     IDAllocator
	state   *world.State
	newSink func(*Boat) event.Sink

	mu      sync.Mutex
	engines []*engine
	docks   *docks
	updates int
	// launches numbers the launch the last Retire handed out; using it or
	// retiring again moves it on.
	launches uint64
}

// New spawns one boat per itinerary at its first dock, facing the
// itinerary's heading, with newSink's sink attached when newSink is non-nil.
// Every schedule starts waiting out its first delay. No itinerary leaves the
// fleet empty, ready to Launch.
func New(itineraries []route.BoatItinerary, ids IDAllocator, state *world.State, newSink func(*Boat) event.Sink) (*Fleet, error) {
	if ids == nil {
		return nil, errors.New("boat: nil id allocator")
	}
	if state == nil {
		return nil, errors.New("boat: nil world state")
	}
	f := &Fleet{ids: ids, state: state, newSink: newSink, docks: newDocks()}
	if err := f.launch(itineraries); err != nil {
		return nil, err
	}
	return f, nil
}

// launch spawns itineraries' boats onto fresh docks, every one free but the
// first dock of each itinerary, which its boat holds. Its caller holds f.mu
// or owns f alone.
func (f *Fleet) launch(itineraries []route.BoatItinerary) error {
	f.docks = newDocks()
	legs := make([][]leg, len(itineraries))
	for i, it := range itineraries {
		if len(it.Routes) == 0 {
			return fmt.Errorf("boat: itinerary %d has no route", i)
		}
		for _, r := range it.Routes {
			l, err := newLeg(r, f.docks)
			if err != nil {
				return fmt.Errorf("boat: itinerary %d from %s: %w", i, r.Dock, err)
			}
			legs[i] = append(legs[i], l)
		}
	}
	for i, it := range itineraries {
		id, err := f.ids.NextID()
		if err != nil {
			return fmt.Errorf("boat: itinerary %d: %w", i, err)
		}
		b := newBoat(id, f.state)
		if f.newSink != nil {
			b.Attach(f.newSink(b))
		}
		at := dockSites[legs[i][0].dock].at
		f.engines = append(f.engines, newEngine(b, legs[i], f.docks, at))
		f.state.Spawn(b, at.X, at.Y, at.Z, it.Heading)
	}
	return nil
}

// Retire takes every boat out of the fleet: its schedule and its ticket
// collection due end, it takes no one else aboard, and it stops where it
// stands, showing its observers so. Its passengers read after Retire are
// all it will ever carry. The boats stay in the world for the caller to put their passengers
// ashore and Remove them. The returned launch is what Launch takes to put
// a new fleet out once they are gone; a later Retire voids it.
func (f *Fleet) Retire() (retired []*Boat, launch uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.engines {
		b := e.boat
		b.engine = nil
		b.mu.Lock()
		b.fare = fareDue{}
		b.retired = true
		b.mu.Unlock()
		b.stop()
		retired = append(retired, b)
	}
	f.engines = nil
	f.launches++
	return retired, f.launches
}

// Launch spawns one boat per itinerary, as New does, into the fleet the
// Retire that returned launch emptied. A launch already used, or voided by a
// later Retire, spawns nothing.
func (f *Fleet) Launch(launch uint64, itineraries []route.BoatItinerary) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if launch != f.launches {
		return nil
	}
	f.launches++
	return f.launch(itineraries)
}

// Boats returns the fleet's boats in itinerary order.
func (f *Fleet) Boats() []*Boat {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*Boat, len(f.engines))
	for i, e := range f.engines {
		out[i] = e.boat
	}
	return out
}

// Tick collects the fares that fall due, advances every sailing boat by one
// position update, ending the legs that arrive, and every updatesPerStep-th
// call takes each schedule's step once its delay has run out.
func (f *Fleet) Tick() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.engines {
		e.boat.tickFare()
	}
	for _, e := range f.engines {
		b := e.boat
		if _, _, _, sailing := b.Departure(); sailing && b.updatePosition() {
			b.arrive()
		}
	}
	f.updates++
	if f.updates < updatesPerStep {
		return
	}
	f.updates = 0
	for _, e := range f.engines {
		if e.canRun() {
			e.run()
		}
	}
}

// Start launches the fleet's ticker.
func (f *Fleet) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(MoveInterval, f.Tick, log)
}
