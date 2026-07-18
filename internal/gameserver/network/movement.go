package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) moveLivePlayer(live *livePlayer, origin, target location.Location) {
	// A client-initiated walk overrides any attack-driven chase movement —
	// otherwise the server's own MaybeStartOffensiveFollow re-think would
	// fight the player's own steering back toward the old target.
	if live.combat != nil {
		live.combat.Stop()
	}

	heading := origin.HeadingTo(target)
	l.updateLivePlayerPosition(live, origin, heading)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMoveToLocation(live.ObjectID(), target, origin)
	})
}

func (l *GameClientLink) stopLivePlayer(live *livePlayer, at location.Location, heading int) {
	l.updateLivePlayerPosition(live, at, heading)
	l.broadcastLiveStopMove(live, at, heading)
}

func (l *GameClientLink) validateLivePlayerPosition(live *livePlayer, reported location.Location, heading int) {
	current := live.CurrentLocation()
	if current.Distance2D(reported) > liveMoveSpeed(live) {
		live.SendFrame(serverpackets.FrameValidateLocation(live.ObjectID(), current, live.CurrentHeading()))
		return
	}
	l.updateLivePlayerPosition(live, reported, heading)
}

func liveMoveSpeed(live *livePlayer) float64 {
	if live == nil || live.template == nil {
		return 0
	}
	if live.Running() {
		return live.RunSpeed()
	}
	return live.template.WalkSpeed
}

func (l *GameClientLink) changeLiveMoveType(live *livePlayer, run bool) {
	if !live.SetRunning(run) {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameChangeMoveType(live.ObjectID(), live.Running(), false)
	})
}

func (l *GameClientLink) changeLiveWaitType(live *livePlayer, stand bool) bool {
	if live == nil || live.AlikeDead() || !live.SetStanding(stand) {
		return false
	}
	x, y, z := live.Position()
	waitType := serverpackets.WaitSitting
	if stand {
		waitType = serverpackets.WaitStanding
		live.releaseChair()
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameChangeWaitType(live.ObjectID(), waitType, location.Location{X: x, Y: y, Z: z})
	})
	return true
}

func (l *GameClientLink) broadcastLiveSocialAction(live *livePlayer, actionID int32) {
	if actionID < 2 || actionID > 13 || live.AlikeDead() || !live.Standing() || live.InCombat() {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameSocialAction(live.ObjectID(), actionID)
	})
}

func (l *GameClientLink) broadcastLiveMoveEvent(live *livePlayer, event move.Event) {
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMove(live.ObjectID(), event)
	})
}

func (l *GameClientLink) broadcastLiveStopMove(live *livePlayer, at location.Location, heading int) {
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameStopMove(live.ObjectID(), at, heading)
	})
}

// broadcastLiveFrame sends frame() to live's own session and to every
// object it currently knows, building a fresh frame per recipient since
// each wire.Frame is released after its own send.
func (l *GameClientLink) broadcastLiveFrame(live *livePlayer, frame func() wire.Frame) {
	live.SendFrame(frame())
	if l.world == nil {
		return
	}
	known := live.appendKnown(l.world)
	defer live.releaseKnown()
	for _, o := range known {
		receiver, ok := o.(frameReceiver)
		if !ok {
			continue
		}
		receiver.SendFrame(frame())
	}
}

type frameReceiver interface {
	SendFrame(wire.Frame) bool
}

func (p *livePlayer) appendKnown(state *world.State) []world.Tracked {
	return p.known.Snapshot(state, p)
}

func (p *livePlayer) releaseKnown() {
	p.known.Release()
}

func (l *GameClientLink) updateLivePlayerPosition(live *livePlayer, position location.Location, heading int) {
	live.Character.SetLastKnownPosition(position, heading)
	live.Character.SetHeading(heading)
	if live.move != nil {
		// Reseed CreatureMove's own position tracking too, or the next
		// chase this controller starts computes its route/duration from a
		// stale seed (only this position changed; CreatureMove.origin
		// otherwise only advances on its own arrival).
		live.move.SetPosition(position)
	}
	if l.world == nil {
		return
	}
	if err := l.world.Move(live, position.X, position.Y, position.Z); err != nil {
		l.log.Debug().Err(err).Int32("object_id", live.ObjectID()).Msg("move player")
	}
}
