package move

import (
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// WalkOutcome is where a chain of position updates leaves a straight walk.
type WalkOutcome struct {
	// Stop is where the walker stands once the chain ends.
	Stop location.Location
	// Reached reports that the walk arrived on its destination.
	Reached bool
	// Blocked reports that the walk ended on a step the geodata refuses.
	Blocked bool
	// Points is the position after each update that ran: the chain stops
	// at the update that ends the walk, so any later step is not listed.
	Points []location.Location
}

// WalkSteps runs a player's straight walk from origin toward target over geo
// with the position updates steps stand for, each a duration of walking, and
// reports where it ends. The first walkStartUpdates updates advance at
// startSpeed and the rest at speed. It is the stepping the movement update
// itself runs (UpdatePosition on a mover given SetSpeeds), with no queue
// clock running, no path-finding and no owner to notify, so an offline caller can
// replay a recorded walk.
func WalkSteps(geo Geo, origin, target location.Location, speed, startSpeed float64, steps []time.Duration) (WalkOutcome, error) {
	if !validSpeed(speed) || !validSpeed(startSpeed) {
		return WalkOutcome{}, fmt.Errorf("move: walk speeds %v and %v must be finite and non-negative", speed, startSpeed)
	}
	// The arrival timer needs a queue; its clock never advances here, so the
	// timer never fires and each update stands for exactly its step.
	mover, err := NewCreatureMove(origin, speed, geo, WithQueue(sim.NewInline(time.Time{}).NewQueue("walk")))
	if err != nil {
		return WalkOutcome{}, err
	}
	mover.SetSpeeds(speed, startSpeed)
	if _, err := mover.MoveStraightToLocation(target); err != nil {
		return WalkOutcome{}, err
	}

	out := WalkOutcome{Points: make([]location.Location, 0, len(steps))}
	for _, step := range steps {
		_, moving := mover.UpdatePosition(step)
		out.Points = append(out.Points, mover.Position())
		if !moving {
			break
		}
	}
	out.Stop = mover.Position()
	if !mover.Moving() {
		out.Blocked = mover.walkBlocked()
		out.Reached = !out.Blocked
	}
	return out, nil
}

// walkBlocked reports that the stopped walk ended on a refused step.
func (m *CreatureMove) walkBlocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.routeBlocked
}
