package boat

import (
	"math"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// worldZMax caps a boat's height on every position update.
const worldZMax = 16410

// Boat is one scheduled boat in the world.
//
// Its position and heading live in the embedded world.Presence. Everything
// else is driven by its fleet's tick goroutine; the fields under mu are the
// ones other goroutines read (a player coming to know the boat), and the
// tick goroutine writes them under it.
type Boat struct {
	world.Presence

	objectID int32
	sink     event.Sink
	world    *world.State

	mu sync.Mutex
	// moving reports a leg under way toward destination.
	moving        bool
	destination   location.Location
	moveSpeed     int
	rotationSpeed int

	// Owned by the tick goroutine.
	xAccurate, yAccurate float64
	path                 []route.BoatLocation
	pathIndex            int
	engine               *engine
}

func newBoat(objectID int32, state *world.State) *Boat {
	return &Boat{objectID: objectID, world: state}
}

// ObjectID returns the boat's world object id.
func (b *Boat) ObjectID() int32 { return b.objectID }

// Kind reports KindBoat.
func (*Boat) Kind() actor.Kind { return actor.KindBoat }

// Attach installs sink as the receiver of this boat's events. Call it once,
// before the boat is spawned.
func (b *Boat) Attach(sink event.Sink) { b.sink = sink }

// Departure reports the leg the boat is under way on: where it heads and
// the move and rotation speeds it travels with. ok is false while the boat
// stands still.
func (b *Boat) Departure() (destination location.Location, moveSpeed, rotationSpeed int, ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.destination, b.moveSpeed, b.rotationSpeed, b.moving
}

func (b *Boat) emit(ev event.Event) {
	if b.sink != nil {
		b.sink.Emit(ev)
	}
}

func (b *Boat) location() location.Location {
	x, y, z := b.Position()
	return location.Location{X: x, Y: y, Z: z}
}

// executePath sails path from its first node on, then tells observers the
// boat set off.
func (b *Boat) executePath(path []route.BoatLocation) {
	b.pathIndex = 0
	b.path = path
	b.moveToNextSegment()
	b.emit(event.BoatStarted{Moving: true})
}

// moveToNextSegment announces the departure lines of the current node and
// heads for it.
func (b *Boat) moveToNextSegment() {
	node := b.path[b.pathIndex]
	b.engine.announce(node.DepartureMessages)
	b.moveTo(node)
}

// moveTo heads the boat for node at the node's speeds, then tells observers
// the move and the departure.
func (b *Boat) moveTo(node route.BoatLocation) {
	from := b.location()
	b.xAccurate, b.yAccurate = float64(from.X), float64(from.Y)
	b.SetHeading(from.HeadingTo(node.Location))

	b.mu.Lock()
	if node.Speed > 0 {
		b.moveSpeed = node.Speed
	}
	if node.Rotation > 0 {
		b.rotationSpeed = node.Rotation
	}
	b.moving = true
	b.destination = node.Location
	speed, rotation := b.moveSpeed, b.rotationSpeed
	b.mu.Unlock()

	b.emit(event.BoatDeparted{From: from, Destination: node.Location, Speed: speed, Rotation: rotation})
}

// updatePosition advances the leg under way by one position update, a tenth
// of a second at the boat's move speed, straight toward its destination in
// three dimensions. It reports whether the leg ended. Call it only while the
// boat is under way.
func (b *Boat) updatePosition() bool {
	b.mu.Lock()
	dest, speed := b.destination, b.moveSpeed
	b.mu.Unlock()
	if !b.Visible() {
		return true
	}

	x, y, z := b.Position()
	dx := float64(dest.X) - b.xAccurate
	dy := float64(dest.Y) - b.yAccurate
	dz := float64(dest.Z - z)
	left := math.Sqrt(dx*dx + dy*dy + dz*dz)
	// The step is a single-precision tenth of the speed.
	passed := float64(float32(speed) / 10)

	nx, ny, nz := dest.X, dest.Y, min(dest.Z, worldZMax)
	if passed < left {
		fraction := passed / left
		b.xAccurate += dx * fraction
		b.yAccurate += dy * fraction
		nx, ny = int(b.xAccurate), int(b.yAccurate)
		nz = min(z+int(dz*fraction+0.5), worldZMax)
	}
	if nx != x || ny != y || nz != z {
		// A boat never leaves the world bounds its route data keeps it in;
		// a failed move still updates its position.
		_ = b.world.Move(b, nx, ny, nz)
	}
	return passed >= left
}

// arrive ends the leg under way at its node: the node's arrival lines are
// announced, then the boat heads for the next node, waits off shore before
// the last one (the destination dock), or stops at the last one.
func (b *Boat) arrive() {
	b.mu.Lock()
	b.moving = false
	b.mu.Unlock()

	if b.pathIndex >= len(b.path) {
		return
	}
	b.engine.announce(b.path[b.pathIndex].ArrivalMessages)
	b.pathIndex++
	switch b.pathIndex {
	case len(b.path) - 1:
		b.engine.state = stateReadyToMoveToDock
	case len(b.path):
		b.engine.state = stateDocked
		b.stop()
	default:
		b.moveToNextSegment()
	}
}

// stop halts the boat and shows its observers it stopped where it stands.
func (b *Boat) stop() {
	b.mu.Lock()
	b.moving = false
	b.mu.Unlock()
	b.emit(event.BoatStarted{Moving: false})
	b.emit(event.BoatShown{})
}
