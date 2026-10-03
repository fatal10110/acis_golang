package boat

import (
	"errors"
	"fmt"
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
// every schedule and movement change, runs on one goroutine at a time: the
// fleet's ticker in production, the test in a suite.
type Fleet struct {
	engines []*engine
	docks   *docks
	updates int
}

// New spawns one boat per itinerary at its first dock, facing the
// itinerary's heading, with newSink's sink attached when newSink is non-nil.
// Every schedule starts waiting out its first delay.
func New(itineraries []route.BoatItinerary, ids IDAllocator, state *world.State, newSink func(*Boat) event.Sink) (*Fleet, error) {
	if ids == nil {
		return nil, errors.New("boat: nil id allocator")
	}
	if state == nil {
		return nil, errors.New("boat: nil world state")
	}
	f := &Fleet{docks: newDocks()}
	legs := make([][]leg, len(itineraries))
	for i, it := range itineraries {
		if len(it.Routes) == 0 {
			return nil, fmt.Errorf("boat: itinerary %d has no route", i)
		}
		for _, r := range it.Routes {
			l, err := newLeg(r, f.docks)
			if err != nil {
				return nil, fmt.Errorf("boat: itinerary %d from %s: %w", i, r.Dock, err)
			}
			legs[i] = append(legs[i], l)
		}
	}
	for i, it := range itineraries {
		id, err := ids.NextID()
		if err != nil {
			return nil, fmt.Errorf("boat: itinerary %d: %w", i, err)
		}
		b := newBoat(id, state)
		if newSink != nil {
			b.Attach(newSink(b))
		}
		at := dockSites[legs[i][0].dock].at
		f.engines = append(f.engines, newEngine(b, legs[i], f.docks, at))
		state.Spawn(b, at.X, at.Y, at.Z, it.Heading)
	}
	return f, nil
}

// Boats returns the fleet's boats in itinerary order.
func (f *Fleet) Boats() []*Boat {
	out := make([]*Boat, len(f.engines))
	for i, e := range f.engines {
		out[i] = e.boat
	}
	return out
}

// Tick advances every sailing boat by one position update, ending the legs
// that arrive, and every updatesPerStep-th call takes each schedule's step
// once its delay has run out.
func (f *Fleet) Tick() {
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
