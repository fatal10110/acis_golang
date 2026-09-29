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
	if l.startPickupLiveGroundItem(ctx, live, target, shift) {
		return
	}
	if cur := live.Target(); cur == nil || cur.ObjectID() != target.ObjectID() {
		l.selectLiveTarget(live, target)
		return
	}
	if selected && l.actOnSummon(live, target, ctrl, shift) {
		return
	}
	if selected && l.actOnPlayer(live, target, ctrl, shift) {
		return
	}
	if selected && l.interactLiveStaticObject(live, target) {
		return
	}
	if selected && l.sitLiveOnChair(live, target, true) {
		return
	}
	if selected {
		l.attackLiveTarget(live, target)
	}
}

func (l *GameClientLink) interactLiveStaticObject(live *livePlayer, target world.Tracked) bool {
	obj, ok := target.(*staticobject.Object)
	if !ok {
		return false
	}

	// Interacting replaces the follow intention.
	switch obj.Type() {
	case staticobject.MapType:
		live.endFollow()
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(serverpackets.FrameShowTownMap("town_map."+obj.Template.Texture, obj.Template.MapX, obj.Template.MapY))
	case staticobject.ArenaSignType:
		live.endFollow()
		html, ok := l.html.Get("signboard.htm")
		if !ok {
			html = "<html><body>My html is missing:<br>data/html/signboard.htm</body></html>"
		}
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(serverpackets.FrameNpcHtmlMessage(obj.ObjectID(), html, 0))
	default:
		return false
	}
	return true
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
// past — attack or pickup lock), and either way answers the click with
// ActionFailed so the client's pending action releases immediately instead
// of waiting on a response that never comes.
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

func (l *GameClientLink) finishDeferredPickup(live *livePlayer) {
	pickup := live.takeDeferredPickup()
	if pickup == nil || pickup.target == nil {
		return
	}
	target := l.resolveTarget(pickup.target.ObjectID())
	if target != pickup.target {
		return
	}
	ground, ok := target.(*grounditem.Item)
	if !ok {
		return
	}
	if blocked, deferrable := livePickupBlockedDeferrable(live); blocked {
		l.deferOrFailPickup(pickup.ctx, live, ground, pickup.shift, deferrable)
		return
	}
	l.walkOrForwardPickup(pickup.ctx, live, ground, pickup.shift)
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
	// summonInteractApproachRange mirrors PlayerAI.thinkInteract's
	// maybeMoveToPawn(target, 100, isShiftPressed) offset
	// (PlayerAI.java:437): already this close skips the walk and opens the
	// pet status window immediately.
	summonInteractApproachRange = 100
	// summonInteractRange mirrors Npc.INTERACTION_DISTANCE, the gate
	// canDoInteract re-checks once an approach walk arrives
	// (PlayerAI.java:538, Npc.java:89).
	summonInteractRange = 150
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
			l.attackLiveTarget(live, s)
		} else {
			l.showOwnedPetStatus(live, s, shift)
		}
		return true
	}
	if s.AttackableWithoutForceBy(live.Character) || (ctrl && s.AttackableBy(live.Character)) {
		l.attackLiveTarget(live, s)
		return true
	}
	l.followLiveTarget(live, s, ctrl, shift)
	return true
}

// showOwnedPetStatus is the owner's interact with its own summon: the
// status window, after an approach walk when out of range.
func (l *GameClientLink) showOwnedPetStatus(live *livePlayer, pet *summon.Actor, shift bool) {
	// Interacting with an owned summon releases the pending action the client
	// registered for the click before showing the status window; PetStatusShow
	// alone leaves that action outstanding and locks further input. The
	// interact replaces the follow intention.
	live.endFollow()
	live.SendFrame(serverpackets.FrameActionFailed())
	if summonInRange(live, pet, summonInteractApproachRange) {
		live.SendFrame(serverpackets.FramePetStatusShow(pet.SummonType()))
		return
	}
	if shift || live.move == nil {
		return
	}
	px, py, pz := pet.Position()
	live.clearParkedApproaches()
	live.setPetInteract(pet)
	accepted, err := live.move.MoveToLocation(location.Location{X: px, Y: py, Z: pz})
	if err != nil {
		l.log.Warn().Err(err).Msg("move: broadcast")
	}
	if !accepted {
		live.takePetInteract()
	}
}

func summonInRange(live *livePlayer, pet *summon.Actor, radius int) bool {
	lx, ly, lz := live.Position()
	px, py, pz := pet.Position()
	return location.In3DRange(lx, ly, lz, px, py, pz, radius)
}

// finishPetInteract fires once an approach walk started by showOwnedPetStatus
// arrives (the Arrived event from its move.Controller), mirroring
// thinkInteract's post-move canDoInteract recheck: the owner or pet may have
// moved again meanwhile, so the range and ownership gates run again before
// the status window opens.
func (l *GameClientLink) finishPetInteract(live *livePlayer) {
	l.applyOwnedPetInteract(live, live.takePetInteract())
}

// applyOwnedPetInteract is the onInteract half of an owned-pet INTERACT:
// world/owner checks plus the shared canDoInteract gate, then PetStatusShow.
func (l *GameClientLink) applyOwnedPetInteract(live *livePlayer, pet *summon.Actor) {
	if pet == nil {
		return
	}
	if l.resolveTarget(pet.ObjectID()) != world.Tracked(pet) {
		return
	}
	if pet.OwnerID() != live.ObjectID() {
		return
	}
	if !l.playerCanDoInteract(live, pet) {
		return
	}
	live.SendFrame(serverpackets.FramePetStatusShow(pet.SummonType()))
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
	if pet := live.takePetInteract(); pet != nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		if !l.playerCanDoInteract(live, pet) {
			return false
		}
		live.BroadcastStop()
		// onInteract has no world-presence check: PetStatusShow uses the
		// snapshot summon even if it has already left the world.
		live.SendFrame(serverpackets.FramePetStatusShow(pet.SummonType()))
		return true
	}
	if live.takeDeferredMagicSkill() != nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageDistTooFarCastingStopped))
	}
	return false
}

// playerCanDoInteract is operating / active trade / 150 3D range. Blocked
// INTERACT uses this as the single StopMove+PetStatusShow gate. Arrived
// INTERACT also requires the pet still in world before the same gate
// (thinkInteract's isTargetLost check).
func (l *GameClientLink) playerCanDoInteract(live *livePlayer, pet *summon.Actor) bool {
	if live == nil || pet == nil || live.Operating() {
		return false
	}
	if l.trades != nil && l.trades.HasActive(live.ObjectID()) {
		return false
	}
	return summonInRange(live, pet, summonInteractRange)
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
		if target := live.Target(); target != nil && l.sitLiveOnChair(live, target, false) {
			return
		}
	}
	if !l.changeLiveWaitType(live, stand) {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
}

// sitLiveOnChair claims target as a throne and sits live on it. viaClick
// distinguishes the two reference entry points that share this logic: a
// second Action click routes through StaticObject.onAction ->
// tryToInteract -> thinkInteract, whose first statement is an unconditional
// clientActionFailed() (PlayerAI.java:415) — so a successful click-driven
// sit still has to release the pending action here. The sit key and
// action-bar button route through tryToSit (PlayableAI.java:430), which only
// sends clientActionFailed on a denyAiAction rejection, never on success.
func (l *GameClientLink) sitLiveOnChair(live *livePlayer, target world.Tracked, viaClick bool) bool {
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
	if viaClick {
		live.SendFrame(serverpackets.FrameActionFailed())
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
	}
}

// attackLiveTarget starts (or continues) live's attack intention against
// target: closing distance first when target is out of weapon range, then
// swinging once in range, repeating on subsequent calls until target dies,
// is lost, or the attack is cancelled. It reports whether the attempt was
// accepted — false means the caller should report the action as failed.
func (l *GameClientLink) attackLiveTarget(live *livePlayer, target world.Tracked) bool {
	combatant, ok := target.(attackable.Combatant)
	if !ok {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	// Reference: AttackRequest.java:31 rejects via isOutOfControl()
	// (Creature.java:652-655) before dispatching to onAction.
	if liveOutOfControl(live) {
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
	if live.combat.RefuseTarget(combatant) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	live.clearParkedApproaches()
	if !live.combat.Start(combatant) {
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
func (l *GameClientLink) startSummonAttackStance(actor *summon.Actor) {
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
	resources := live.ResourceValues()
	live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentHP, Value: int(resources.CurrentHP)},
		{Type: serverpackets.StatusCurrentMP, Value: int(resources.CurrentMP)},
		{Type: serverpackets.StatusCurrentCP, Value: int(resources.CurrentCP)},
		{Type: serverpackets.StatusMaxCP, Value: int(resources.MaxCP)},
	}))
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
