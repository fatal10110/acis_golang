package boat

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// fleetDeadlockBound is how long a fleet call may take before the test
// reads it as a lock-order deadlock.
const fleetDeadlockBound = 30 * time.Second

type sinkFunc func(event.Event)

func (f sinkFunc) Emit(ev event.Event) { f(ev) }

// counterIDs hands out object ids from a counter.
type counterIDs struct{ next atomic.Int32 }

func (c *counterIDs) NextID() (int32, error) { return 1_000_000 + c.next.Add(1), nil }

// within runs fn on its own goroutine and fails the test when it has not
// returned within fleetDeadlockBound.
func within(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(fleetDeadlockBound):
		t.Fatalf("%s did not finish: lock-order deadlock", what)
	}
}

// ticksToDeparture sails runeRoundTrip's boat past its departure at second
// 303, two seconds into its first leg (TestBoatRoundTrip).
const ticksToDeparture = 3050

// A boat's events, sent while the fleet ticks, may read the fleet's boats
// and itineraries: the tick holds only the fleet's step lock across them.
func TestBoatEventsMayReadTheFleet(t *testing.T) {
	t.Parallel()
	itinerary, _ := runeRoundTrip()
	var fleet atomic.Pointer[boat.Fleet]
	var reads atomic.Int32
	newSink := func(*boat.Boat) event.Sink {
		return sinkFunc(func(event.Event) {
			if f := fleet.Load(); f != nil {
				f.Boats()
				f.Itineraries()
				reads.Add(1)
			}
		})
	}
	f, err := boat.New([]route.BoatItinerary{itinerary}, &counterIDs{}, world.New(), newSink)
	if err != nil {
		t.Fatalf("boat.New: %v", err)
	}
	fleet.Store(f)

	within(t, "sailing to the departure", func() {
		for range ticksToDeparture {
			f.Tick()
		}
	})
	if reads.Load() == 0 {
		t.Fatal("no boat event read the fleet")
	}
	if !f.Boats()[0].Moving() {
		t.Fatal("the boat is not under way after its departure schedule")
	}
}

// The fleet ticks on its own goroutine while others read it, retire it and
// launch a new one, under -race. A boat retired never moves again once
// Retire returns, and the launch puts the new boat out.
func TestFleetTickRacesRetireLaunchAndReads(t *testing.T) {
	t.Parallel()
	itinerary, _ := runeRoundTrip()
	itineraries := []route.BoatItinerary{itinerary}
	state := world.New()
	f, err := boat.New(itineraries, &counterIDs{}, state, nil)
	if err != nil {
		t.Fatalf("boat.New: %v", err)
	}
	for range ticksToDeparture {
		f.Tick()
	}

	var stop atomic.Bool
	var ticks atomic.Int64
	var wg sync.WaitGroup
	wg.Go(func() {
		for !stop.Load() {
			f.Tick()
			ticks.Add(1)
		}
	})
	wg.Go(func() {
		for !stop.Load() {
			for _, b := range f.Boats() {
				b.Position()
			}
			f.Itineraries()
		}
	})
	// awaitTicks lets the ticking goroutine run n more ticks.
	awaitTicks := func(n int64) {
		for from := ticks.Load(); ticks.Load() < from+n; {
			runtime.Gosched()
		}
	}

	within(t, "retire and launch", func() {
		awaitTicks(5)
		retired, launch := f.Retire()
		if len(retired) != 1 {
			t.Errorf("retired %d boats, want 1", len(retired))
			return
		}
		x, y, z := retired[0].Position()
		awaitTicks(20)
		if nx, ny, nz := retired[0].Position(); nx != x || ny != y || nz != z || retired[0].Moving() {
			t.Errorf("retired boat moved from (%d,%d,%d) to (%d,%d,%d), moving %v", x, y, z, nx, ny, nz, retired[0].Moving())
		}
		retired[0].Remove()
		if err := f.Launch(launch, itineraries); err != nil {
			t.Errorf("Launch: %v", err)
		}
		awaitTicks(5)
		stop.Store(true)
		wg.Wait()
		boats := f.Boats()
		if len(boats) != 1 || boats[0] == retired[0] || len(f.Itineraries()) != 1 {
			t.Errorf("fleet after the launch = %d boats, %d itineraries; want a new boat on 1", len(boats), len(f.Itineraries()))
		}
	})
}
