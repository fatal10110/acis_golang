package move

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/rs/zerolog"
)

func TestArrivalTimerRecoversPanickingHookAndRunsNextMove(t *testing.T) {
	buf := &syncMoveBuffer{}
	mover, err := NewCreatureMove(location.Location{}, 1_000_000, &recordingGeo{canMove: true})
	if err != nil {
		t.Fatal(err)
	}
	mover.SetLogger(zerolog.New(buf))
	mover.SetArrivedHook(func() { panic("move boom") })
	if _, err := mover.MoveToLocation(location.Location{X: 1}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(buf.String(), "move boom") {
		if time.Now().After(deadline) {
			t.Fatalf("recovered panic log = %q, want panic value", buf.String())
		}
		time.Sleep(time.Millisecond)
	}

	done := make(chan struct{})
	mover.SetArrivedHook(func() { close(done) })
	if _, err := mover.MoveToLocation(location.Location{X: 2}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("move scheduled after panicking arrival hook did not complete")
	}
}

type syncMoveBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncMoveBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncMoveBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestCreatureMove_MoveToLocationFiresArrivedOnceDurationElapses(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	target := location.Location{X: 100, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(target)
	if err != nil {
		t.Fatal(err)
	}
	if !mover.Moving() {
		t.Fatal("Moving() = false immediately after an accepted move, want true")
	}
	if got := mover.Position(); got != origin {
		t.Fatalf("Position() = %+v before arrival, want %+v", got, origin)
	}

	clock.fire(event.Duration)

	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1", arrivedCalls)
	}
	if got := mover.Position(); got != target {
		t.Fatalf("Position() = %+v after arrival, want %+v", got, target)
	}
	if mover.Moving() {
		t.Fatal("Moving() = true after arrival, want false")
	}
}

func TestCreatureMove_MoveToLocationBlockedRouteFiresArrivedOnNextTick(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	mover, err := NewCreatureMove(origin, 100, &recordingGeo{height: 0})
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 0})
	if err != nil {
		t.Fatalf("MoveToLocation() error = %v, want nil", err)
	}
	if want := (Event{Origin: origin, Destination: origin, Speed: 100}); event != want {
		t.Fatalf("MoveToLocation() event = %+v, want %+v", event, want)
	}
	if !mover.Moving() {
		t.Fatal("Moving() = false before zero-distance arrival, want true")
	}

	clock.fire(PositionUpdateInterval)

	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1", arrivedCalls)
	}
	if mover.Moving() {
		t.Fatal("Moving() = true after zero-distance arrival, want false")
	}
}

func TestCreatureMove_MoveToLocationSupersedesPendingArrival(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	first, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 0})
	if err != nil {
		t.Fatal(err)
	}
	second, err := mover.MoveToLocation(location.Location{X: 200, Y: 0, Z: 0})
	if err != nil {
		t.Fatal(err)
	}

	// The superseded first timer must not move the actor once the second
	// request has changed the destination.
	clock.fire(first.Duration)
	if arrivedCalls != 0 {
		t.Fatalf("arrived hook calls after stale timer = %d, want 0", arrivedCalls)
	}
	if got := mover.Position(); got != origin {
		t.Fatalf("Position() = %+v after stale timer, want %+v", got, origin)
	}

	clock.fire(second.Duration)
	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls after current timer = %d, want 1", arrivedCalls)
	}
	if got := mover.Position(); got != (location.Location{X: 200, Y: 0, Z: 0}) {
		t.Fatalf("Position() = %+v, want %+v", got, location.Location{X: 200, Y: 0, Z: 0})
	}
}

func TestCreatureMove_CancelMoveStopsArrival(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	event, err := mover.MoveToLocation(location.Location{X: 100, Y: 0, Z: 0})
	if err != nil {
		t.Fatal(err)
	}
	mover.CancelMove()
	clock.fire(event.Duration)

	if arrivedCalls != 0 {
		t.Fatalf("arrived hook calls after CancelMove = %d, want 0", arrivedCalls)
	}
	if mover.Moving() {
		t.Fatal("Moving() = true after CancelMove, want false")
	}
	if got := mover.Position(); got != origin {
		t.Fatalf("Position() = %+v after CancelMove, want %+v", got, origin)
	}
}

func TestCreatureMove_OffensiveFollowTickSchedulesArrival(t *testing.T) {
	origin := location.Location{X: 0, Y: 0, Z: 0}
	geo := &recordingGeo{canMove: true, height: 0}
	mover, err := NewCreatureMove(origin, 100, geo)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeMoveClock{}
	mover.afterFunc = clock.AfterFunc
	arrivedCalls := 0
	mover.SetArrivedHook(func() { arrivedCalls++ })

	mover.StartOffensiveFollow(9, 40)
	outside := TargetSnapshot{ObjectID: 9, Known: true, Position: location.Location{X: 200, Y: 0}, CollisionRadius: 10}
	event, moved, err := mover.FollowTick(outside, 9.9)
	if err != nil {
		t.Fatal(err)
	}
	if !moved {
		t.Fatal("FollowTick() moved = false, want true")
	}

	clock.fire(event.Duration)

	if arrivedCalls != 1 {
		t.Fatalf("arrived hook calls = %d, want 1", arrivedCalls)
	}
	if got := mover.Position(); got != (location.Location{X: 200, Y: 0, Z: 0}) {
		t.Fatalf("Position() = %+v, want %+v", got, location.Location{X: 200, Y: 0, Z: 0})
	}
}
