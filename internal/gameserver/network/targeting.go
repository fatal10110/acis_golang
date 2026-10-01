package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) broadcastAttack(attacker *livePlayer, snapshot event.Attack) {
	if attacker == nil {
		return
	}

	frame := serverpackets.FrameAttack(snapshot)
	encoded := append([]byte(nil), frame.Bytes()...)
	frame.Release()

	send := func(receiver frameReceiver) {
		receiver.BroadcastFrame(wire.BorrowedFrame(append([]byte(nil), encoded...)))
	}
	send(attacker)

	if l.world == nil {
		return
	}
	l.world.ForEachKnown(attacker, func(o world.Tracked) {
		receiver, ok := o.(frameReceiver)
		if !ok {
			return
		}
		send(receiver)
	})
}

// handleTargetAction answers a click on objectID. ctrl is set for a forced
// attack: an AttackRequest on the object already selected.
func (l *GameClientLink) handleTargetAction(ctx context.Context, live *livePlayer, objectID int32, selected, ctrl, shift bool) {
	target := l.resolveTarget(objectID)
	if target == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	_, ground := target.(*grounditem.Item)
	if ground {
		if inPostureTransition(live) {
			if live.DenyAIAction() {
				live.SendFrame(serverpackets.FrameActionFailed())
				return
			}
			live.deferAction(func() {
				if l.resolveTarget(objectID) != target {
					live.SendFrame(serverpackets.FrameActionFailed())
					return
				}
				l.startPickupLiveGroundItem(ctx, live, target, shift)
			})
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		live.takeDeferredAction()
		l.startPickupLiveGroundItem(ctx, live, target, shift)
		return
	}
	if cur := live.Target(); cur == nil || cur.ObjectID() != target.ObjectID() {
		l.selectLiveTarget(live, target)
		return
	}
	if !selected {
		return
	}
	// A static object's interact also waits out a swing or a cast; every
	// other selected-target click waits here only for a sit-down or
	// stand-up. A player that cannot act is refused the interact outright,
	// before anything is queued or run, so the click leaves a fear flee or
	// any other walk under way untouched.
	busy := inPostureTransition(live)
	if _, static := target.(*staticobject.Object); static {
		if live.DenyAIAction() {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		busy = itemAICastBusy(live)
	}
	if busy {
		if live.DenyAIAction() || (ctrl && liveOutOfControl(live)) || target.ObjectID() == live.ObjectID() {
			live.SendFrame(serverpackets.FrameActionFailed())
			return
		}
		if pet, ok := target.(*summon.Actor); ok && !ctrl && pet.ShownAsOwnedBy(live.ObjectID()) {
			live.deferInteract(pet, shift)
		} else if f, ok := target.(*npc.Folk); ok && !ctrl {
			live.deferInteract(f, shift)
		} else {
			run := l.queuedSelectedTargetAction(live, target, ctrl, shift)
			live.deferAction(func() {
				if l.resolveTarget(objectID) != target {
					live.SendFrame(serverpackets.FrameActionFailed())
					return
				}
				run()
			})
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	live.takeDeferredAction()
	l.actOnSelectedTarget(live, target, ctrl, shift)
}

func (l *GameClientLink) actOnSelectedTarget(live *livePlayer, target world.Tracked, ctrl, shift bool) {
	if l.actOnSummon(live, target, ctrl, shift) {
		return
	}
	if l.actOnPlayer(live, target, ctrl, shift) {
		return
	}
	if l.actOnFolk(live, target, ctrl, shift) {
		return
	}
	if obj, ok := target.(*staticobject.Object); ok {
		l.thinkStaticInteract(live, obj)
		return
	}
	l.attackLiveTarget(live, target, shift)
}

// queuedSelectedTargetAction captures the click's resolved intention before a
// posture transition ends. A later selection must not change what it runs.
func (l *GameClientLink) queuedSelectedTargetAction(live *livePlayer, target world.Tracked, ctrl, shift bool) func() {
	attack := func() { l.attackQueuedTarget(live, target, shift) }
	switch v := target.(type) {
	case *summon.Actor:
		// The caller queues a click without ctrl on an owned summon as its
		// interact intention, so only a ctrl-click reaches here.
		if v.ShownAsOwnedBy(live.ObjectID()) {
			return attack
		}
		if v.AttackableWithoutForceBy(live.Character) || (ctrl && v.AttackableBy(live.Character)) {
			return attack
		}
		return func() { l.startLiveFollow(live, v, shift) }
	case *livePlayer:
		if v.AttackableWithoutForceBy(live.Character) || (ctrl && v.AttackableBy(live.Character)) {
			return attack
		}
		if v.Operating() {
			return func() { l.tryToInteract(live, v, shift) }
		}
		return func() { l.startLiveFollow(live, v, shift) }
	case *staticobject.Object:
		return func() { l.thinkStaticInteract(live, v) }
	}
	return attack
}

// thinkStaticInteract runs a second click on a selected static object as an
// interact: the click is released with ActionFailed first, then a town map
// shows its map and an arena sign its signboard. A throne answers nothing
// more: a click never sits on or claims it, only the sit request does. A
// player that cannot act, sits, flies, runs a private store or trades
// interacts with nothing. Every interact ends idle, stopping a walk under
// way.
// ponytail: an object out of interact range is not walked to, and no
// MoveToPawn faces it (#2962).
func (l *GameClientLink) thinkStaticInteract(live *livePlayer, obj *staticobject.Object) {
	live.SendFrame(serverpackets.FrameActionFailed())
	if live.DenyAIAction() || live.Seated() || live.Flying() || !l.playerCanAttemptInteract(live) {
		live.tryToIdle(false)
		return
	}
	switch obj.Type() {
	case staticobject.MapType:
		live.SendFrame(serverpackets.FrameShowTownMap("town_map."+obj.Template.Texture, obj.Template.MapX, obj.Template.MapY))
	case staticobject.ArenaSignType:
		html, ok := l.html.Get("signboard.htm")
		if !ok {
			html = "<html><body>My html is missing:<br>data/html/signboard.htm</body></html>"
		}
		sendValidatedHTML(live, obj.ObjectID(), html, 0)
	}
	live.tryToIdle(false)
}

func (l *GameClientLink) startPickupLiveGroundItem(ctx context.Context, live *livePlayer, target world.Tracked, shift bool) bool {
	ground, ok := target.(*grounditem.Item)
	if !ok {
		return false
	}
	if blocked, deferrable := livePickupBlockedDeferrable(live); blocked {
		l.deferOrFailPickup(ctx, live, ground, shift, deferrable)
		return true
	}
	if live.combat != nil {
		live.combat.Stop()
	}
	return l.walkOrForwardPickup(ctx, live, ground, shift)
}

// deferOrFailPickup parks target for a later drain if deferrable (live's
// current blocker, as decided atomically alongside blocked by
// livePickupBlockedDeferrable, is one finishDeferredPickup will promote it
// past — attack, cast in flight or pickup lock), and either way answers the
// click with ActionFailed so the client's pending action releases
// immediately instead of waiting on a response that never comes.
func (l *GameClientLink) deferOrFailPickup(ctx context.Context, live *livePlayer, ground *grounditem.Item, shift, deferrable bool) {
	if deferrable {
		live.deferPickup(ctx, ground, shift)
	}
	live.SendFrame(serverpackets.FrameActionFailed())
}

// walkOrForwardPickup is the click-time decision shared by a fresh click
// (startPickupLiveGroundItem) and a drained deferred click
// (finishDeferredPickup): collect immediately if already in range, otherwise
// walk to it unless shift was held — a shift-click never walks, matching the
// reference's maybeMoveToLocation(..., isShiftPressed) (CreatureMove.java:
// 438-443, the walk is skipped when isShiftPressed).
func (l *GameClientLink) walkOrForwardPickup(ctx context.Context, live *livePlayer, ground *grounditem.Item, shift bool) bool {
	// The pickup is the current intention now, in range or not.
	live.dropHeldIntention()
	if groundPickupInRange(live, ground) {
		return l.pickupLiveGroundItem(ctx, live, ground)
	}
	if shift || live.move == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	x, y, z := ground.Position()
	live.clearParkedApproaches()
	live.setPickup(ctx, ground)
	live.SendFrame(serverpackets.FrameActionFailed())
	accepted, err := live.move.MoveToLocation(location.Location{X: x, Y: y, Z: z})
	if err != nil {
		l.log.Warn().Err(err).Msg("move: broadcast")
	}
	if accepted {
		return true
	}
	live.takePickup()
	return true
}

func (l *GameClientLink) finishLiveGroundPickup(live *livePlayer) {
	pickup := live.takePickup()
	if pickup == nil || pickup.target == nil {
		return
	}
	target := l.resolveTarget(pickup.target.ObjectID())
	if target != pickup.target {
		return
	}
	l.pickupLiveGroundItem(pickup.ctx, live, target)
}

// thinkLivePickup thinks the pickup a walk toward a ground item holds
// again: the client is released, then the player walks to the item afresh,
// or collects it once in range. A player that cannot act or is not standing,
// or an item gone meanwhile, ends the pickup idle, stopping the walk.
func (l *GameClientLink) thinkLivePickup(live *livePlayer) {
	pickup := live.takePickup()
	if pickup == nil {
		return
	}
	if live.DenyAIAction() || !live.Standing() {
		live.SendFrame(serverpackets.FrameActionFailed())
		live.tryToIdle(false)
		return
	}
	var ground *grounditem.Item
	if pickup.target != nil {
		if target := l.resolveTarget(pickup.target.ObjectID()); target == pickup.target {
			ground, _ = target.(*grounditem.Item)
		}
	}
	if ground == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		if live.move != nil {
			live.move.Stop()
		}
		return
	}
	l.walkOrForwardPickup(pickup.ctx, live, ground, false)
}

// finishDeferredPickup runs the pickup queued as the next intention, if any,
// and reports whether one was waiting. The pickup replaces the attack or
// cast intention whatever its outcome: the swing, shot or cast it waited
// behind is not followed by another. An item gone meanwhile only releases
// the click.
func (l *GameClientLink) finishDeferredPickup(live *livePlayer) bool {
	pickup := live.takeDeferredPickup()
	if pickup == nil {
		return false
	}
	if live.combat != nil {
		live.combat.Replace()
	}
	var ground *grounditem.Item
	if pickup.target != nil {
		if target := l.resolveTarget(pickup.target.ObjectID()); target == pickup.target {
			ground, _ = target.(*grounditem.Item)
		}
	}
	if ground == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return true
	}
	if blocked, deferrable := livePickupBlockedDeferrable(live); blocked {
		l.deferOrFailPickup(pickup.ctx, live, ground, pickup.shift, deferrable)
		return true
	}
	l.walkOrForwardPickup(pickup.ctx, live, ground, pickup.shift)
	return true
}

func (l *GameClientLink) resolveTarget(objectID int32) world.Tracked {
	if l.world == nil {
		return nil
	}
	if obj, ok := l.world.Object(objectID); ok {
		return obj
	}
	if p, ok := l.world.Player(objectID); ok {
		return p
	}
	return nil
}

const (
	// interactApproachOffset is how far short of an interact target the
	// approach walk stops. A click from within this offset plus both bodies'
	// collision radii skips the walk.
	interactApproachOffset = 100
	// interactionDistance is the interaction distance: the gate every
	// interact re-checks before the target is acted on, and the distance
	// the player is shown facing the target from.
	interactionDistance = 150
)

// actOnSummon answers a click on an already-selected summon and reports
// whether target was one. Its owner opens the summon's status window, or
// attacks it when forcing. Anyone else attacks it when the owner's karma or
// PvP flag allows it without forcing, or when forcing and it is attackable,
// and otherwise follows it.
func (l *GameClientLink) actOnSummon(live *livePlayer, target world.Tracked, ctrl, shift bool) bool {
	s, ok := target.(*summon.Actor)
	if !ok || live == nil {
		return false
	}
	if s.ShownAsOwnedBy(live.ObjectID()) {
		if ctrl {
			l.attackLiveTarget(live, s, shift)
		} else {
			l.tryToInteract(live, s, shift)
		}
		return true
	}
	if s.AttackableWithoutForceBy(live.Character) || (ctrl && s.AttackableBy(live.Character)) {
		l.attackLiveTarget(live, s, shift)
		return true
	}
	l.followLiveTarget(live, s, ctrl, shift)
	return true
}

// actOnFolk answers a click on an already-selected civilian NPC and
// reports whether target was one. A forced click attacks it, since another
// creature may always attack it by force; any other click talks to it.
func (l *GameClientLink) actOnFolk(live *livePlayer, target world.Tracked, ctrl, shift bool) bool {
	f, ok := target.(*npc.Folk)
	if !ok || live == nil {
		return false
	}
	if ctrl {
		// A civilian NPC is not a combatant yet, so the attack answers
		// ActionFailed (#2664).
		l.attackLiveTarget(live, f, shift)
		return true
	}
	l.tryToInteract(live, f, shift)
	return true
}

// interactTarget is what a player's interact intention walks to and acts
// on: the player's own summon, whose status window opens, or a civilian
// NPC, whose chat window opens.
type interactTarget interface {
	world.Tracked
	Position() (x, y, z int)
	CollisionRadius() float64
}

// interactIntention is an interact queued behind a swing or a cast.
type interactIntention struct {
	target interactTarget
	shift  bool
}

// tryToInteract is the player's interact with target, after an approach
// walk when out of range. A player that cannot take AI actions is only
// answered ActionFailed. One still swinging or casting queues the interact
// for that to end, answered ActionFailed too. Otherwise the interact
// replaces the current intention, an attack waiting out a bow's reuse or
// chasing its target included, and runs now.
func (l *GameClientLink) tryToInteract(live *livePlayer, target interactTarget, shift bool) {
	if live.DenyAIAction() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if (live.attack != nil && live.attack.AttackingNow()) || live.CastingNow() {
		live.deferInteract(target, shift)
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	replaceWithInteract(live)
	l.thinkInteract(live, target, shift)
}

// replaceWithInteract makes the interact the current intention: the attack
// intention is dropped, every parked approach or queued intention (a
// pickup, a cast, a follow, a use-item, another interact) goes with it, and
// every follow task, an attack chase or a friendly follow, is cancelled. A
// walk under way is left to the interact's think, which walks elsewhere or
// stops it.
func replaceWithInteract(live *livePlayer) {
	if live.combat != nil {
		live.combat.Replace()
	}
	live.clearParkedApproaches()
	if live.move != nil {
		live.move.CancelFollow()
	}
}

// endInteractIdle ends the interact idle: a walk still under way stops,
// broadcast as StopMove.
func endInteractIdle(live *livePlayer) {
	if live.move != nil {
		live.move.Stop()
	}
}

// interactTargetLost reports whether target can no longer be interacted
// with: it left the world, or, for a summon, is no longer live's own.
func (l *GameClientLink) interactTargetLost(live *livePlayer, target interactTarget) bool {
	if l.resolveTarget(target.ObjectID()) != world.Tracked(target) {
		return true
	}
	if pet, ok := target.(*summon.Actor); ok {
		return pet.OwnerID() != live.ObjectID()
	}
	return false
}

// finishDeferredInteract runs the interact queued as the next intention, if
// any, and reports whether one was waiting. It replaces the attack
// intention; a target lost meanwhile ends it with ActionFailed. During a
// sit-down or stand-up the interact stays queued for PostureSettled.
func (l *GameClientLink) finishDeferredInteract(live *livePlayer) bool {
	if live == nil || live.detached() {
		return false
	}
	if inPostureTransition(live) {
		return live.hasDeferredInteract()
	}
	queued := live.takeDeferredInteract()
	if queued == nil {
		return false
	}
	replaceWithInteract(live)
	if l.interactTargetLost(live, queued.target) {
		live.SendFrame(serverpackets.FrameActionFailed())
		endInteractIdle(live)
		return true
	}
	l.thinkInteract(live, queued.target, queued.shift)
	return true
}

// thinkInteract runs one think of the interact with target, on the click
// and again when an approach walk arrives. It always releases the pending
// client action first; PetStatusShow alone leaves that action outstanding
// and locks further input. A player that cannot act, sits, flies, runs a
// private store or trades gets nothing more. Out of approach range, a
// movable player walks toward the target unless shift is held. In range,
// the player still inside interaction distance faces the target and acts
// on it. Every outcome but the approach walk, or a player that cannot move
// holding the interact, ends it idle.
func (l *GameClientLink) thinkInteract(live *livePlayer, target interactTarget, shift bool) {
	live.SendFrame(serverpackets.FrameActionFailed())
	if live.DenyAIAction() || !live.Standing() || live.Flying() || !l.playerCanAttemptInteract(live) {
		endInteractIdle(live)
		return
	}
	if !interactInRange(live, target, int(interactApproachOffset+live.CollisionRadius()+target.CollisionRadius())) {
		if shift {
			endInteractIdle(live)
			return
		}
		if live.move == nil || live.MovementDisabled() {
			return
		}
		live.setInteract(target)
		if !live.move.MoveToPawn(target, interactApproachOffset) {
			live.takeInteract()
			return
		}
		live.Character.SetHeading(live.move.Position().HeadingTo(targetLocation(target)))
		return
	}
	if !l.playerCanDoInteract(live, target) {
		endInteractIdle(live)
		return
	}
	// A moving NPC target is answered StopMove instead; no interact target
	// moves yet.
	at := live.CurrentLocation()
	live.Character.SetHeading(at.HeadingTo(targetLocation(target)))
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameMoveToPawn(live.ObjectID(), target.ObjectID(), interactionDistance, at)
	})
	l.onInteract(live, target)
	endInteractIdle(live)
}

// onInteract acts on an interact target in reach: the player's own summon
// shows its status window, a civilian NPC talks, a player running a store
// shows its store window.
func (l *GameClientLink) onInteract(live *livePlayer, target interactTarget) {
	switch t := target.(type) {
	case *livePlayer:
		l.showPrivateStore(live, t)
	case *summon.Actor:
		live.SendFrame(serverpackets.FramePetStatusShow(t.SummonType()))
	case *npc.Folk:
		l.talkToFolk(live, t)
	}
}

func targetLocation(target interactTarget) location.Location {
	x, y, z := target.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func interactInRange(live *livePlayer, target interactTarget, radius int) bool {
	lx, ly, lz := live.Position()
	tx, ty, tz := target.Position()
	return location.In3DRadius(lx, ly, lz, tx, ty, tz, radius)
}

// finishInteract thinks the interact an approach walk started by
// thinkInteract holds again, once the walk arrives (the Arrived event from
// its move.Controller) or an equip toggle mid-walk replaced it: the player
// or target may have moved meanwhile, so every gate runs again, and a
// target out of approach range is approached anew. A target lost meanwhile
// ends the interact idle, stopping a walk under way.
func (l *GameClientLink) finishInteract(live *livePlayer) {
	target := live.takeInteract()
	if target == nil {
		return
	}
	if l.interactTargetLost(live, target) {
		live.SendFrame(serverpackets.FrameActionFailed())
		endInteractIdle(live)
		return
	}
	l.thinkInteract(live, target, false)
}

// onPlayerArrivedBlocked is the player blocked-arrival arm: INTERACT in
// range broadcasts StopMove and finishes the interact; CAST sends
// DIST_TOO_FAR_CASTING_STOPPED then the base same-cell MoveToLocation;
// every other intention uses that base correction.
func (l *GameClientLink) onPlayerArrivedBlocked(live *livePlayer) bool {
	if live == nil {
		return false
	}
	if live.move != nil {
		pos := live.move.Position()
		l.updateLivePlayerPosition(live, pos, live.CurrentHeading())
	}
	if target := live.takeInteract(); target != nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		if !l.playerCanDoInteract(live, target) {
			return false
		}
		live.BroadcastStop()
		// onInteract has no world-presence check: a summon's PetStatusShow
		// uses the snapshot summon even if it has already left the world.
		l.onInteract(live, target)
		return true
	}
	magic, itemCast := live.takeDeferredMagicSkill(), live.takeDeferredItemAICast()
	if magic != nil || itemCast != nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageDistTooFarCastingStopped))
	}
	return false
}

// playerCanAttemptInteract reports whether live may interact at all: not
// while running a private store, trading, or holding an unexpired trade
// request it sent or received.
func (l *GameClientLink) playerCanAttemptInteract(live *livePlayer) bool {
	if live.Operating() {
		return false
	}
	return l.trades == nil || !l.trades.ProcessingTransaction(live.ObjectID())
}

// playerCanDoInteract is the interact attempt gate plus the 150 3D
// interaction distance. Blocked INTERACT uses this as the single
// StopMove+onInteract gate.
func (l *GameClientLink) playerCanDoInteract(live *livePlayer, target interactTarget) bool {
	if live == nil || target == nil || !l.playerCanAttemptInteract(live) {
		return false
	}
	return interactInRange(live, target, interactionDistance)
}

// requestChangeWaitType handles the sit/stand key (RequestChangeWaitType)
// and the action-bar sit/stand button (RequestActionUse action 0), which the
// reference routes through the same tryToSit(target)/tryToStand() AI calls.
// A sit request first tries the player's current target as a throne; an
// invalid or unclaimable target (wrong type, busy, out of range) still falls
// back to a plain sit, matching the reference's unconditional sitDown()
// ahead of its chair check. Any rejection releases the client with
// ActionFailed instead of silence.
func (l *GameClientLink) requestChangeWaitType(live *livePlayer, stand bool) {
	if live == nil {
		return
	}
	target := live.Target()
	if live.DenyAIAction() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if itemAICastBusy(live) {
		live.deferAction(func() { l.runChangeWaitType(live, stand, target) })
		return
	}
	live.takeDeferredAction()
	l.runChangeWaitType(live, stand, target)
}

func (l *GameClientLink) runChangeWaitType(live *livePlayer, stand bool, target world.Tracked) {
	if live.DenyAIAction() {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	// The reference's thinkStand rejects only on real death (denyAiAction),
	// not fake death, and instead stops the fake-death toggle: stopFakeDeath
	// removes the FAKE_DEATH effect, whose exit hook stands the player back
	// up and broadcasts the revive visual (PlayerAI.java:490-501). Once the
	// effect is gone the player is still getting up, and the request is
	// refused below as a stand while not seated.
	if stand && !live.Dead() && live.EffectList().IsAffected(effect.FlagFakeDeath) {
		live.EffectList().StopByType(effect.TypeFakeDeath)
		return
	}
	if !stand {
		if target != nil && l.sitLiveOnChair(live, target) {
			return
		}
	}
	if !l.changeLiveWaitType(live, stand) {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}

// sitLiveOnChair claims target as a throne and sits live on it, for a sit
// request: a successful sit answers no ActionFailed.
func (l *GameClientLink) sitLiveOnChair(live *livePlayer, target world.Tracked) bool {
	if live == nil {
		return false
	}
	chair, ok := target.(staticobject.Chair)
	if !ok || !staticobject.ClaimChair(live, chair, staticobject.ChairInteractionDistance) {
		return false
	}
	live.throne = chair
	if !l.changeLiveWaitType(live, false) {
		live.releaseChair()
		return false
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameChairSit(live.ObjectID(), chair.StaticObjectID())
	})
	return true
}

// positionedTarget is a target whose facing the client needs revalidated on
// selection.
type positionedTarget interface {
	Position() (int, int, int)
	Heading() int
}

func (l *GameClientLink) selectLiveTarget(live *livePlayer, target world.Tracked) bool {
	if live == nil || target == nil {
		return false
	}
	if cur := live.Target(); cur != nil && cur.ObjectID() == target.ObjectID() {
		return true
	}
	live.StoreTarget(target)
	// Reference: Player.setTarget sends ValidateLocation for the new target
	// before MyTargetSelected, skipped only when the target is the selecting
	// player itself or aboard a boat (Player.java:2477-2479). Boats aren't a
	// ported feature, so every target here is treated as never in one.
	if target.ObjectID() != live.ObjectID() {
		// Every creature target (players, NPCs including decorations,
		// summons, doors) gets a ValidateLocation; static objects and items
		// send none.
		if kind := target.Kind(); kind != actor.KindStatic && kind != actor.KindItem {
			if creature, ok := target.(positionedTarget); ok {
				x, y, z := creature.Position()
				live.SendFrame(serverpackets.FrameValidateLocation(target.ObjectID(), location.Location{X: x, Y: y, Z: z}, creature.Heading()))
			}
		}
	}
	live.SendFrame(serverpackets.FrameMyTargetSelected(target.ObjectID(), targetColor(live.Character, target)))
	if attrs, ok := targetHPAttributes(target); ok {
		live.SendFrame(serverpackets.FrameStatusUpdate(target.ObjectID(), attrs))
	}
	l.broadcastTargetSelected(live, target)
	if f, ok := target.(*npc.Folk); ok {
		live.currentFolk.Store(f)
	}
	return true
}

// requestTargetCancel handles a RequestTargetCancel packet, matching
// RequestTargetCancel.java:23-29's split between the unselect flag and an
// in-flight cast: unselect != 0 always clears the target; unselect == 0
// clears the target only when not casting, and while casting only fires
// the Esc cast-cancel (PlayerAI.java:160-165 onEvtCancel -> unconditional
// getCast().stop(), MagicSkillCanceled broadcast, no CASTING_INTERRUPTED,
// target left untouched) when still inside the interrupt window
// (canAbortCast() at RequestTargetCancel.java:26) — outside the window Esc
// is a no-op.
func (l *GameClientLink) requestTargetCancel(live *livePlayer, req clientpackets.RequestTargetCancel) {
	if req.Unselect == 0 && live.Character.CastingNow() {
		if live.Character.CanAbortCast() {
			live.Character.StopCast()
		}
		return
	}
	l.clearLiveTarget(live)
}

func (l *GameClientLink) clearLiveTarget(live *livePlayer) {
	if live == nil {
		return
	}
	old := live.Target()
	live.StoreTarget(nil)
	if live.combat != nil {
		live.combat.Stop()
	}
	l.announceTargetCleared(live, old)
}

// announceTargetCleared answers a cleared selection: ActionFailed to live,
// then TargetUnselected to live and its observers when old was selected.
func (l *GameClientLink) announceTargetCleared(live *livePlayer, old world.Tracked) {
	live.SendFrame(serverpackets.FrameActionFailed())
	if old != nil {
		l.broadcastTargetUnselected(live)
		live.currentFolk.Store(nil)
	}
}

// attackLiveTarget starts (or continues) live's attack intention against
// target: closing distance first when target is out of weapon range, then
// swinging once in range, repeating on subsequent calls until target dies,
// is lost, or the attack is cancelled. A shift-held attack never walks: on a
// target out of reach it goes idle. It reports whether the attempt was
// accepted — false means the caller should report the action as failed.
func (l *GameClientLink) attackLiveTarget(live *livePlayer, target world.Tracked, shift bool) bool {
	return l.attackLiveTargetWithGate(live, target, shift, true)
}

func (l *GameClientLink) attackQueuedTarget(live *livePlayer, target world.Tracked, shift bool) bool {
	return l.attackLiveTargetWithGate(live, target, shift, false)
}

func (l *GameClientLink) attackLiveTargetWithGate(live *livePlayer, target world.Tracked, shift, request bool) bool {
	combatant, ok := target.(attackable.Combatant)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	// Reference: AttackRequest.java:31 rejects via isOutOfControl()
	// (Creature.java:652-655) before dispatching to onAction.
	if request && liveOutOfControl(live) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	// The reference's single intention slot drops PICK_UP, INTERACT, and
	// CAST on any subsequent attack click regardless of which thinkAttack
	// branch it takes — most branches here also cancel or redirect the move
	// itself (chase redirect, the in-range move.Stop(), a rejection's
	// stopLocked), but even a click waited out behind a swing or cast, which
	// leaves the move untouched, still replaces the intention. Clear every
	// parked approach, or a geo close mid-chase still takes INTERACT/CAST. A
	// target the playable attack gate refuses replaces nothing, so it is
	// answered before the clear: a pickup or pet interact still in flight
	// completes.
	if request && live.combat.RefuseTarget(combatant) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	live.takeDeferredAction()
	live.clearParkedApproaches()
	var accepted bool
	if request {
		accepted = live.combat.Start(combatant, shift)
	} else {
		accepted = live.combat.StartIntention(combatant, shift)
	}
	if !accepted {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	return true
}

// liveOutOfControl reports whether live is out of control: stunned,
// immobile until attacked, sleeping, paralyzed, afraid, confused,
// teleporting or dead. Such a player may not send an attack request or walk.
func liveOutOfControl(live *livePlayer) bool {
	return live.Stunned() || live.ImmobileUntilAttacked() || live.Sleeping() || live.Paralyzed() ||
		live.Afraid() || live.Confused() || live.Teleporting() || live.Dead()
}

// startLiveAutoAttack enters or refreshes live's attack stance. Entering it
// shows the stance on live's summon first, then on live.
func (l *GameClientLink) startLiveAutoAttack(live *livePlayer) {
	if live == nil {
		return
	}
	if l.attackStance != nil {
		l.attackStance.Add(live)
	}
	if !live.SetInCombat(true) {
		return
	}
	if l.world != nil {
		if obj, ok := l.world.Summon(live.ObjectID()); ok {
			if actor, ok := obj.(*summon.Actor); ok {
				l.broadcastSummonFrame(actor, serverpackets.FrameAutoAttackStart(actor.ObjectID()))
			}
		}
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameAutoAttackStart(live.ObjectID())
	})
}

// startSummonAttackStance enters or refreshes the attack stance of actor's
// owner, which a summon's stance is. Entering it shows the stance on actor,
// then on its owner.
//
// A summon revived after its owner left the world is in a stance of its
// own: the departed session is gone, and its object id may already be the
// owner's next session's. Entering it shows the stance on actor, and its
// expiry ends it on actor (attackStanceEffects).
func (l *GameClientLink) startSummonAttackStance(actor *summon.Actor) {
	if actor.OwnerLeft() {
		if l.attackStance == nil {
			return
		}
		if !l.attackStance.InAttackStance(actor) {
			l.broadcastSummonFrame(actor, serverpackets.FrameAutoAttackStart(actor.ObjectID()))
		}
		l.attackStance.Add(actor)
		// A hit that passed its Knows check as the summon's decay
		// despawned it can get here after the despawn cleanup's Remove
		// (leaveCorpseBehind), and its entry would outlive the summon,
		// its expiry refused by the closed corpse queue on every tick.
		// The summon leaves the world before that Remove runs, so either
		// the cleanup or this check drops the entry.
		if l.world != nil {
			if obj, ok := l.world.Object(actor.ObjectID()); !ok || obj != world.Tracked(actor) {
				l.attackStance.Remove(actor)
			}
		}
		return
	}
	owner, ok := liveSummonOwner(actor)
	if !ok {
		return
	}
	if l.attackStance != nil {
		l.attackStance.Add(owner)
	}
	if !owner.SetInCombat(true) {
		return
	}
	l.broadcastSummonFrame(actor, serverpackets.FrameAutoAttackStart(actor.ObjectID()))
	l.broadcastLiveFrame(owner, func() wire.Frame {
		return serverpackets.FrameAutoAttackStart(owner.ObjectID())
	})
}

func (l *GameClientLink) stopLiveAutoAttack(live *livePlayer) {
	if live == nil || !live.SetInCombat(false) {
		return
	}
	if l.attackStance == nil || !l.attackStance.Remove(live) {
		return
	}
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameAutoAttackStop(live.ObjectID())
	})
}

func (l *GameClientLink) broadcastTargetSelected(live *livePlayer, target world.Tracked) {
	if l.world == nil {
		return
	}
	x, y, z := live.Position()
	at := location.Location{X: x, Y: y, Z: z}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameTargetSelected(live.ObjectID(), target.ObjectID(), at)
	}, func(send func(frameReceiver)) {
		l.world.ForEachKnown(live, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}

// broadcastTargetUnselected sends TargetUnselected to live first, then to
// every known observer.
func (l *GameClientLink) broadcastTargetUnselected(live *livePlayer) {
	x, y, z := live.Position()
	at := location.Location{X: x, Y: y, Z: z}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameTargetUnselected(live.ObjectID(), at)
	}, func(send func(frameReceiver)) {
		send(live)
		if l.world == nil {
			return
		}
		l.world.ForEachKnown(live, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}

// sendLiveStatus sends live's own session its current HP, MP and CP. Other
// players see a player's health only through the one-off update sent when
// they select it.
func sendLiveStatus(live *livePlayer) {
	live.SendFrame(liveStatusFrame(live))
}

// liveStatusFrame is the StatusUpdate sendLiveStatus sends.
func liveStatusFrame(live *livePlayer) wire.Frame {
	resources := live.ResourceValues()
	return serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentHP, Value: int(resources.CurrentHP)},
		{Type: serverpackets.StatusCurrentMP, Value: int(resources.CurrentMP)},
		{Type: serverpackets.StatusCurrentCP, Value: int(resources.CurrentCP)},
		{Type: serverpackets.StatusMaxCP, Value: int(resources.MaxCP)},
	})
}

// updateLiveAbnormalEffect sends live's own session its current active
// abnormal-effect icon list. Like sendLiveStatus, this packet only ever goes
// to the effected player's own client, matching the reference's
// AbnormalStatusUpdate.
func (l *GameClientLink) updateLiveAbnormalEffect(live *livePlayer) {
	if live == nil {
		return
	}
	entries := live.EffectList().IconEntries(live.Queue().Now())
	effects := make([]serverpackets.AbnormalStatusEffect, len(entries))
	for i, e := range entries {
		effects[i] = serverpackets.AbnormalStatusEffect{
			SkillID:        e.ID,
			Level:          int32(e.Level),
			DurationMillis: int(e.Duration),
			Toggle:         e.Toggle,
		}
	}
	live.SendFrame(serverpackets.FrameAbnormalStatusUpdate(effects))
}

func targetColor(attacker *player.Character, target world.Tracked) int {
	if attacker == nil {
		return 0
	}
	// A summon always shows its level difference; other skill actors only
	// while attackable, and other objects color neutral.
	if _, ok := target.(*summon.Actor); ok {
		return attacker.Level() - targetLevel(target)
	}
	attackableTarget, ok := target.(skilltarget.Actor)
	if !ok || !attackableTarget.AttackableBy(attacker) {
		return 0
	}
	return attacker.Level() - targetLevel(target)
}

func targetLevel(target world.Tracked) int {
	switch t := target.(type) {
	case *livePlayer:
		return t.Level()
	case *npc.Hostile:
		if t.Instance != nil && t.Instance.Template != nil {
			return t.Instance.Template.Level
		}
	case *npc.Folk:
		return t.Level()
	case *summon.Actor:
		return t.Level()
	}
	return 0
}

func targetHPAttributes(target world.Tracked) ([]serverpackets.StatusAttribute, bool) {
	switch t := target.(type) {
	case *livePlayer:
		resources := t.ResourceValues()
		return []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusMaxHP, Value: int(resources.MaxHP)},
			{Type: serverpackets.StatusCurrentHP, Value: int(resources.CurrentHP)},
		}, true
	case interface {
		MaxHP() int
		CurrentHP() int
	}:
		return []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusMaxHP, Value: t.MaxHP()},
			{Type: serverpackets.StatusCurrentHP, Value: t.CurrentHP()},
		}, true
	default:
		return nil, false
	}
}
