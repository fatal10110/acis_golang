package network

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/boat"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// BoatSinks returns the event-sink factory for boats spawned into state.
func BoatSinks(state *world.State) func(*boat.Boat) event.Sink {
	return func(b *boat.Boat) event.Sink { return &boatSink{world: state, boat: b} }
}

// boatSink maps one boat's events to packets: its movement to the players
// that know it, its schedule to every player around its docks.
type boatSink struct {
	world *world.State
	boat  *boat.Boat
	known world.KnownBuffer
}

// Emit sends ev's packets.
func (s *boatSink) Emit(ev event.Event) {
	id := s.boat.ObjectID()
	switch e := ev.(type) {
	case event.BoatDeparted:
		broadcastKnown(&s.known, s.world, s.boat, func() wire.Frame {
			return serverpackets.FrameMoveToLocation(id, e.Destination, e.From)
		})
		broadcastKnown(&s.known, s.world, s.boat, func() wire.Frame {
			return serverpackets.FrameVehicleDeparture(id, e.Speed, e.Rotation, e.Destination)
		})
	case event.BoatStarted:
		broadcastKnown(&s.known, s.world, s.boat, func() wire.Frame {
			return serverpackets.FrameVehicleStarted(id, e.Moving)
		})
	case event.BoatShown:
		broadcastKnown(&s.known, s.world, s.boat, func() wire.Frame {
			return vehicleInfoFrame(s.boat)
		})
	case event.BoatAnnounced:
		listeners := s.audience(e.Audience)
		for _, msg := range e.MessageIDs {
			sendToEach(listeners, func() wire.Frame { return serverpackets.FrameBoatSay(msg) })
		}
	case event.BoatSounded:
		sendToEach(s.audience(e.Audience), func() wire.Frame {
			return serverpackets.FramePlaySoundAt(serverpackets.Sound{
				File: e.Sound, BindToObject: true, ObjectID: id, Location: e.At,
			})
		})
	}
}

// audience returns the online players a.Centers' radius takes in.
func (s *boatSink) audience(a event.BoatAudience) []frameReceiver {
	var out []frameReceiver
	for _, p := range s.world.Players() {
		receiver, ok := p.(*livePlayer)
		if !ok {
			continue
		}
		x, y, _ := receiver.Position()
		for _, c := range a.Centers {
			dx, dy := float64(x-c.X), float64(y-c.Y)
			if math.Sqrt(dx*dx+dy*dy) < float64(a.Radius) {
				out = append(out, receiver)
				break
			}
		}
	}
	return out
}

func sendToEach(receivers []frameReceiver, build func() wire.Frame) {
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, r := range receivers {
			send(r)
		}
	})
}

// vehicleInfoFrame shows b where it stands now.
func vehicleInfoFrame(b *boat.Boat) wire.Frame {
	x, y, z := b.Position()
	return serverpackets.FrameVehicleInfo(b.ObjectID(), location.Location{X: x, Y: y, Z: z}, b.Heading())
}
