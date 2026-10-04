package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// moveLivePlayer handles a client MoveBackwardToLocation request. target and
// packetOrigin are the packet's raw target/origin coordinates, before the
// floor-to-head Z conversion below.
func (l *GameClientLink) moveLivePlayer(live *livePlayer, target, packetOrigin location.Location) {
	// Reject while the player is out of control.
	if liveOutOfControl(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	// A zero move speed is rejected with both ActionFailed and
	// CANT_MOVE_TOO_ENCUMBERED, distinct from the
	// arrow-key (MoveMovement == 0) rejection handled by the caller.
	if liveMoveSpeed(live) == 0 {
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantMoveTooEncumbered))
		return
	}
	// Cancel an in-progress enchant once the out-of-control and zero-speed
	// gates pass, before
	// the floor-to-head Z conversion and 9900-distance cap — a later
	// rejected walk still closes the enchant window.
	l.cancelActiveEnchant(live)
	// Convert the floor-level target Z the client sent into head-level Z
	// before pathing.
	target.Z += int(live.CollisionHeight())
	// A player in a teleport mode jumps to the point instead, however far.
	if l.moveByTeleport(live, target) {
		return
	}
	// Reject any target farther than 9900 units from the packet's own
	// origin (not the
	// server-authoritative position used below to simulate the walk).
	if target.Distance3D(packetOrigin) > 9900 {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	// A passenger's walk is the client's own, on the boat's deck; no walk
	// is simulated until it steps ashore.
	if live.InBoat() {
		l.moveOnBoat(live, target, packetOrigin)
		return
	}
	// A walk across the entrance of a known boat's dock heads for the
	// entrance instead.
	if l.probeBoatEntrance(live, target) {
		return
	}
	live.SetCanBoard(false)
	l.tryLiveMoveTo(live, target, 0)
}

// tryLiveMoveTo makes walking to target the current intention, as a ground
// click does; boatID is the boat whose entrance the walk heads for, or 0.
func (l *GameClientLink) tryLiveMoveTo(live *livePlayer, target location.Location, boatID int32) {
	// A walk requested mid-swing, mid-cast or mid-transition waits for it as
	// the next intention, answered ActionFailed; the end of the swing, cast
	// or transition walks.
	if itemAICastBusy(live) {
		if live.DenyAIAction() {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		live.deferAction(func() { l.startLiveMove(live, target, boatID) })
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	live.takeDeferredAction()
	l.startLiveMove(live, target, boatID)
}

func (l *GameClientLink) startLiveMove(live *livePlayer, target location.Location, boatID int32) {
	if live.DenyAIAction() || live.MovementDisabled() {
		live.tryToIdle(false)
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	// The walk replaces the attack and follow intentions, so no chase
	// re-think steers back toward an old target. A walk already under way
	// is re-steered by the new MoveToLocation, never stopped first.
	live.replaceIntention()

	// The server-authoritative position, never the packet's claimed origin,
	// is what the walk simulates from — the client origin is nothing but a lag hint the
	// server must not adopt.
	accepted, err := live.move.MoveToLocation(target)
	if err != nil {
		l.log.Warn().Err(err).Msg("move: broadcast")
	}
	if !accepted {
		// A rejected walk leaves the player standing, heading untouched.
		live.move.Stop()
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	// Parked approach slots must not survive this new accepted walk.
	live.clearParkedApproaches()
	live.holdMoveTo(target, boatID)
}

// fleeLivePlayer runs live away from e.From as a server-driven move: run
// stance first, then the move request's gates. A player that could not take
// AI actions before the effect in progress landed, which includes one already
// afraid, is only answered ActionFailed; one that cannot move goes idle and is
// answered ActionFailed. Otherwise the walk starts toward the flee point,
// even when that point is the current cell.
//
// The request never arrives mid-attack or mid-cast: fear aborts both before
// its first flee, and every later flee is refused as AI-denied.
func (l *GameClientLink) fleeLivePlayer(live *livePlayer, e event.FleeRequested) {
	l.changeLiveMoveType(live, true)
	if e.AIDenied {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if live.MovementDisabled() || liveMoveSpeed(live) == 0 {
		live.tryToIdle(false)
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if live.combat != nil {
		live.combat.Stop()
	}
	origin := live.move.Position()
	target := origin.FleeFrom(e.From.X, e.From.Y, e.Distance)
	accepted, err := live.move.MoveToLocation(target)
	if err != nil {
		l.log.Warn().Err(err).Msg("flee: broadcast")
	}
	if !accepted {
		return
	}
	live.clearParkedApproaches()
}

func (l *GameClientLink) stopLivePlayer(live *livePlayer) {
	// CannotMoveAnymore is a stop report, not a position report. The walk
	// is simulated server-side, so the stop point is wherever that
	// simulation stands; the client-reported coordinates and heading are
	// discarded: the stop keeps the server-side position.
	live.move.Stop()
}

func (l *GameClientLink) validateLivePlayerPosition(live *livePlayer, reported location.Location, boatID int32) {
	// Validation is skipped entirely while teleporting or observing — no
	// correction packet, and the reported position is not taken: an
	// observer's client reports its camera, not the character.
	if live.Teleporting() || live.ObserverMode() {
		return
	}
	// Under the free camera the reported position is taken as is, with no
	// fall check and no correction.
	if live.teleportMode == teleportModeCamera {
		l.adoptCameraPosition(live, reported)
		return
	}
	damage, falling := live.CheckFall(reported.Z, !liveSwimming(live) && !live.Flying(), l.playerConfig.EnableFallingDamage, time.Now())
	if damage > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageFallDamage, int32(damage)))
	}
	if falling {
		return
	}
	// A passenger reports its deck position.
	if live.InBoat() {
		l.validatePassengerPosition(live, boatID, reported)
		return
	}
	// ValidatePosition only corrects excessive divergence: a client that
	// drifted beyond a second's worth of movement gets the server position
	// back, while a report within the threshold changes nothing. The walk
	// simulation owns the position, so a valid report is never adopted.
	// Ground movement measures the drift in 2D; a swimming or flying
	// player moves in 3D, so its height drift counts too.
	current := live.CurrentLocation()
	drift := current.Distance2D(reported)
	if liveSwimming(live) || live.Flying() {
		drift = current.Distance3D(reported)
	}
	if drift > liveMoveSpeed(live) && !live.BoatMovement() {
		live.SendFrame(serverpackets.FrameValidateLocation(live.ObjectID(), current, live.CurrentHeading()))
	}
}

func liveMoveSpeed(live *livePlayer) float64 {
	if live == nil || live.Template() == nil {
		return 0
	}
	return live.MoveSpeed()
}

// liveSwimming reports whether live stands inside a water zone.
func liveSwimming(live *livePlayer) bool {
	return live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagWater)
}

func (l *GameClientLink) changeLiveMoveType(live *livePlayer, run bool) {
	if !live.SetRunning(run) {
		return
	}
	if liveMoveSpeed(live) != 0 {
		swimming := liveSwimming(live)
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameChangeMoveType(live.ObjectID(), live.Running(), swimming)
		})
	}
	l.broadcastCharacterInfo(live)
}

// broadcastFullStatus answers a stat func change that moved RUN_SPEED, and
// the end of a stop-all that stripped the player's effects: the weight and
// grade penalty bands are refreshed, then the player and its observers get
// its full view with the new stats. A player not in the world yet sends
// nothing; its entry burst carries the new values.
func (l *GameClientLink) broadcastFullStatus(live *livePlayer) {
	if !l.liveInWorld(live) {
		return
	}
	live.RefreshWeightPenalty()
	live.RefreshExpertisePenalty()
	l.broadcastCharacterInfo(live)
}

// sendModifiedStats answers any other stat func change: the weight and grade
// penalty bands are refreshed and the player gets its own UserInfo, then a
// changed P.Atk. or cast speed reaches the player and its observers as one
// StatusUpdate. A player not in the world yet sends nothing; its entry burst
// carries the new values.
func (l *GameClientLink) sendModifiedStats(live *livePlayer, attrs []event.StatusAttr) {
	if !l.liveInWorld(live) {
		return
	}
	live.RefreshWeightPenalty()
	live.RefreshExpertisePenalty()
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	if len(attrs) == 0 {
		return
	}
	status := make([]serverpackets.StatusAttribute, len(attrs))
	for i, attr := range attrs {
		typ := serverpackets.StatusAttackSpeed
		if attr.Kind == event.StatusMagicSpeed {
			typ = serverpackets.StatusCastSpeed
		}
		status[i] = serverpackets.StatusAttribute{Type: typ, Value: attr.Value}
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameStatusUpdate(live.ObjectID(), status)
	})
}

// liveInWorld reports whether live is the player the world currently holds
// under its object id and its login has spawned it, the gate for
// server-initiated view refreshes that the entry burst otherwise carries. A
// selected player is registered before it is spawned.
func (l *GameClientLink) liveInWorld(live *livePlayer) bool {
	if l.world == nil {
		return true
	}
	current, ok := l.world.Player(live.ObjectID())
	return ok && current == live && live.entered.Load()
}

// changeLiveWaitType sits live down or stands it up and broadcasts the new
// posture. A queued cast is dropped: the sit or stand request takes the
// next-intention slot it held.
func (l *GameClientLink) changeLiveWaitType(live *livePlayer, stand bool) bool {
	if live == nil || live.AlikeDead() || !live.ChangePosture(stand) {
		return false
	}
	dropPostureQueuedIntentions(live)
	l.broadcastLiveWaitType(live, stand)
	return true
}

// dropPostureQueuedIntentions drops what a sit or stand takes the place of:
// the queued cast, item cast, follow, item use and interaction, the held
// intention and a friendly follow. It runs on live's own queue.
func dropPostureQueuedIntentions(live *livePlayer) {
	live.takeDeferredMagicSkill()
	live.takeDeferredItemAICast()
	live.takeDeferredFollow()
	live.takeDeferredUseItem()
	live.takeDeferredInteract()
	live.dropHeldIntention()
	live.endFollow()
}

// broadcastLiveWaitType sends live's sitting or standing ChangeWaitType to
// live and every observer.
func (l *GameClientLink) broadcastLiveWaitType(live *livePlayer, stand bool) {
	x, y, z := live.Position()
	waitType := serverpackets.WaitSitting
	if stand {
		waitType = serverpackets.WaitStanding
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameChangeWaitType(live.ObjectID(), waitType, location.Location{X: x, Y: y, Z: z})
	})
}

// standAttackedLivePlayer answers a hit or offensive skill that reached a
// seated live player with the stand intention, even where the damage itself
// stands nobody up (an invulnerable or storing player). It runs where the
// hit or skill notifies its target, before any of the hit's damage, so the
// damage then finds the player no longer seated and stands nobody up again.
// A player whose AI is denied (storing, stunned, observing, ...) or who is
// mounted is answered with a single ActionFailed and goes idle; one seated
// while it plays dead gets up out of fake death, its Fake Death effect, if
// still on, ending with its own get-up first; anyone else stands up as a
// stand request does. Either way the intentions the player held or queued,
// a click held behind a posture transition among them, are dropped, on live's own queue, the only one that touches them; the
// idle answers nothing more even when the damage has meanwhile started a
// stand-up. A chair is released when a stand-up ends.
func (l *GameClientLink) standAttackedLivePlayer(live *livePlayer) {
	if live == nil || live.detached() || !live.Seated() {
		return
	}
	if live.Character.DenyAIAction() || live.Operating() || live.Mounted() {
		live.SendFrame(serverpackets.FrameActionFailed())
		postLive(live, func() {
			if !live.detached() {
				live.goIdle()
			}
		})
		return
	}
	if !live.StandFromFakeDeath() {
		if live.AlikeDead() || !live.ChangePosture(true) {
			return
		}
		l.broadcastLiveWaitType(live, true)
	}
	postLive(live, func() {
		if !live.detached() {
			live.takeDeferredAction()
			dropPostureQueuedIntentions(live)
		}
	})
}

// broadcastLiveSocialAction handles an emote request. A fishing player is
// told it cannot emote while fishing, whatever the emote. Any other rejected
// emote (out-of-range id, dead, sitting, or in combat) answers with nothing,
// on purpose. Emotes don't register a pending client action the way
// target/attack/item clicks do, so silence can't freeze input the way the
// silent-drop bug class behind #829 freezes it — and the specified handler
// stays silent on every rejection path except the fishing one. Adding
// ActionFailed here would diverge from that behavior with no client-side
// benefit, so this is left intentionally silent instead of patched to match
// the ActionFailed pattern used by the action-locked handlers in #873.
func (l *GameClientLink) broadcastLiveSocialAction(live *livePlayer, actionID int32) {
	if live.Fishing() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDoWhileFishing))
		return
	}
	if actionID < 2 || actionID > 13 || live.Operating() || live.AlikeDead() || !live.Standing() || live.InCombat() {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameSocialAction(live.ObjectID(), actionID)
	})
}

func (l *GameClientLink) broadcastLiveMoveEvent(live *livePlayer, ev event.Move) {
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMove(live.ObjectID(), ev)
	})
}

func (l *GameClientLink) broadcastLiveStopMove(live *livePlayer, at location.Location, heading int) {
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameStopMove(live.ObjectID(), at, heading)
	})
}

// broadcastLiveDie sends the death packet live's own session and every
// observer, so the corpse-fall animation plays immediately instead of only
// on a later dead reconnect.
func (l *GameClientLink) broadcastLiveDie(live *livePlayer) {
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameDie(live.ObjectID(), dieOptions(live))
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
		known := live.known.SnapshotCopy(l.world, live)
		defer known.Release()
		for _, o := range known.Tracked() {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		}
	})
}

type frameReceiver interface {
	BroadcastFrame(wire.Frame) bool
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
			receiver.BroadcastFrame(frame)
		}
	})
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
	l.revalidateZones(live, previous, revalidateForce)
}
