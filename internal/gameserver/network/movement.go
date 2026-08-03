package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) moveLivePlayer(live *livePlayer, target location.Location) {
	// A client-initiated walk overrides any attack-driven chase movement —
	// otherwise the server's own MaybeStartOffensiveFollow re-think would
	// fight the player's own steering back toward the old target.
	if live.combat != nil {
		live.combat.Stop()
	}
	// A client-initiated walk cancels the AI's current intention, including
	// an in-flight cast (PlayerAI.onEvtCancel).
	live.Character.StopCast()

	// The server-authoritative position, never the packet's claimed origin,
	// is what the walk simulates from (matching the reference's
	// tryToMoveTo) — the client origin is nothing but a lag hint the
	// server must not adopt.
	origin := live.move.Position()
	if !live.move.MoveToLocation(target) {
		// A route that cannot make lateral progress (geo fully blocked) is
		// rejected outright; answer it so the click never goes silent. The
		// reference only ever rotates once a move is actually accepted
		// (CreatureMove.moveToLocation sets heading after resolving a
		// destination, never on an outright-rejected one), so a rejected
		// route must leave heading untouched too.
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	// Face the destination from the same server-authoritative origin the
	// walk itself started from.
	live.Character.SetHeading(origin.HeadingTo(target))
}

func (l *GameClientLink) stopLivePlayer(live *livePlayer) {
	// CannotMoveAnymore is a stop report, not a position report. The walk
	// is simulated server-side, so the stop point is wherever that
	// simulation stands; the client-reported coordinates and heading are
	// discarded exactly like the reference's getMove().stop() does.
	live.move.Stop()
}

func (l *GameClientLink) validateLivePlayerPosition(live *livePlayer, reported location.Location) {
	// ValidatePosition only corrects excessive divergence: a client that
	// drifted beyond a second's worth of movement gets the server position
	// back, while a report within the threshold changes nothing. The walk
	// simulation owns the position, so a valid report is never adopted.
	current := live.CurrentLocation()
	if current.Distance2D(reported) > liveMoveSpeed(live) {
		live.SendFrame(serverpackets.FrameValidateLocation(live.ObjectID(), current, live.CurrentHeading()))
	}
}

func liveMoveSpeed(live *livePlayer) float64 {
	if live == nil || live.template == nil {
		return 0
	}
	if live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagWater) {
		return live.SwimSpeed()
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
	swimming := live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagWater)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameChangeMoveType(live.ObjectID(), live.Running(), swimming)
	})
}

func (l *GameClientLink) changeLiveWaitType(live *livePlayer, stand bool) bool {
	if live == nil || live.AlikeDead() || !live.SetStanding(stand) {
		return false
	}
	if !stand {
		live.Character.StopCast()
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

// broadcastLiveSocialAction mirrors the reference behavior for an emote
// request: a rejected emote (out-of-range id, dead, sitting, or in combat)
// answers with nothing, on purpose. Emotes don't register a pending client
// action the way target/attack/item clicks do, so silence can't freeze input
// the way the silent-drop bug class behind #829 freezes it — and the
// reference handler itself stays silent on every rejection path except the
// fishing one (the fishing check is a separate, not-yet-wired gap and would
// carry its own message, not ActionFailed). Adding ActionFailed here would
// diverge from that behavior with no client-side benefit, so this is left
// intentionally silent instead of patched to match the ActionFailed pattern
// used by the action-locked handlers in #873.
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

// broadcastLiveDie sends the death packet live's own session and every
// observer, so the corpse-fall animation plays immediately instead of only
// on a later dead reconnect. Restart-selector options are left at their
// zero value: they depend on clan hall/castle/siege ownership and sweep
// eligibility that aren't wired yet.
func (l *GameClientLink) broadcastLiveDie(live *livePlayer) {
	l.abortFusionTargeting(live)
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameDie(live.ObjectID(), serverpackets.DieOptions{})
	})
}

// broadcastLiveRevive sends the revive packet to live's own session and
// every observer, so the corpse-fall animation clears immediately.
func (l *GameClientLink) broadcastLiveRevive(live *livePlayer) {
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameRevive(live.ObjectID())
	})
}

// broadcastLiveFrame sends one serialized frame to live's own session and to
// every object it currently knows. Each recipient gets an independent pooled
// copy because its session encrypts outgoing bytes in place.
func (l *GameClientLink) broadcastLiveFrame(live *livePlayer, frame func() wire.Frame) {
	broadcastFrame(frame, func(send func(frameReceiver)) {
		send(live)
		if l.world == nil {
			return
		}
		known := live.appendKnown(l.world)
		defer live.releaseKnown()
		for _, o := range known {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		}
	})
}

type frameReceiver interface {
	SendFrame(wire.Frame) bool
}

func broadcastFrame(build func() wire.Frame, recipients func(func(frameReceiver))) {
	var serialized wire.Frame
	built := false
	defer func() { serialized.Release() }()
	recipients(func(receiver frameReceiver) {
		if !built {
			serialized = build()
			built = true
		}
		frame, ok := serverpackets.CopyFrame(serialized)
		if ok {
			receiver.SendFrame(frame)
		}
	})
}

func (p *livePlayer) appendKnown(state *world.State) []world.Tracked {
	return p.known.Snapshot(state, p)
}

func (p *livePlayer) releaseKnown() {
	p.known.Release()
}

func (l *GameClientLink) updateLivePlayerPosition(live *livePlayer, position location.Location, heading int) {
	previous := live.CurrentLocation()
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
	l.revalidateZones(live, previous)
}
