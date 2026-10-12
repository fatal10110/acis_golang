package network

import (
	"sync/atomic"
)

// reloadBoats reads the boat routes anew and replaces the fleet with them:
// every boat loses its schedule and the fare due, stops
// where it stands, puts its passengers ashore at their town restart point
// and leaves the world; then the docks are free and one boat per itinerary
// read puts out, whether AllowBoat turned boats on at boot or not. Routes
// that cannot be read leave the fleet as it was.
//
// Each passenger is put ashore on its own queue, the game master at once.
// The old boats leave the world, and the new ones enter it, once the last
// passenger is ashore: a passenger never sees its boat go before its own
// teleport, and the docks see the old boats go before the new ones come. With
// passengers aboard other than gm, gm is told the boats reloaded before
// that.
func (l *GameClientLink) reloadBoats(gm *livePlayer) error {
	if l.reloads.BoatRoutes == nil || l.boats == nil || l.world == nil {
		return errReloadNotPorted
	}
	itineraries, err := l.reloads.BoatRoutes()
	if err != nil {
		return err
	}
	retired, launch := l.boats.Retire()

	// pending counts the ousts still to run, plus this call's own share,
	// dropped last, so the swap waits for every oust posted.
	var pending atomic.Int32
	pending.Store(1)
	done := func() {
		if pending.Add(-1) != 0 {
			return
		}
		for _, b := range retired {
			b.Remove()
		}
		if err := l.boats.Launch(launch, itineraries); err != nil {
			l.log.Error().Err(err).Msg("admin: //reload boat could not launch the boats read")
		}
	}
	for _, b := range retired {
		boatID := b.ObjectID()
		for _, id := range b.Passengers() {
			p, ok := l.world.Player(id)
			if !ok {
				continue
			}
			live, ok := p.(*livePlayer)
			if !ok {
				continue
			}
			pending.Add(1)
			posted := onPlayer(gm, live, func() {
				defer done()
				l.oustFromRetiredBoat(live, boatID)
			})
			if !posted {
				// The player's queue is closed: it left the world, and its
				// boat with it.
				done()
			}
		}
	}
	done()
	return nil
}

// oustFromRetiredBoat puts live ashore at its town restart point if it
// still rides boatID; with no restart point to go to it
// only leaves the boat.
func (l *GameClientLink) oustFromRetiredBoat(live *livePlayer, boatID int32) {
	if live.BoatObjectID() != boatID {
		return
	}
	if dest, ok := l.restartDestination(live); ok {
		l.oustPassenger(live, dest)
		return
	}
	l.leaveBoat(live)
}
