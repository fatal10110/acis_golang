package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
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

func (l *GameClientLink) broadcastAttack(attacker *livePlayer, snapshot attack.Snapshot) {
	if attacker == nil {
		return
	}

	frame := serverpackets.FrameAttack(snapshot)
	encoded := append([]byte(nil), frame.Bytes()...)
	frame.Release()

	send := func(receiver interface{ BroadcastFrame(wire.Frame) bool }) {
		receiver.BroadcastFrame(wire.BorrowedFrame(append([]byte(nil), encoded...)))
	}
	send(attacker)

	if l.world == nil {
		return
	}
	l.world.ForEachKnown(attacker, func(o world.Tracked) {
		receiver, ok := o.(interface{ BroadcastFrame(wire.Frame) bool })
		if !ok {
			return
		}
		send(receiver)
	})
}

func (l *GameClientLink) handleTargetAction(ctx context.Context, live *livePlayer, objectID int32, selected, shift bool) {
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
	if selected && l.showOwnedPetStatus(live, target, shift) {
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

	switch obj.Type() {
	case staticobject.MapType:
		live.SendFrame(serverpackets.FrameActionFailed())
		live.SendFrame(serverpackets.FrameShowTownMap("town_map."+obj.Template.Texture, obj.Template.MapX, obj.Template.MapY))
	case staticobject.ArenaSignType:
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
	// This walk redirects live.move's single in-flight target away from any
	// pending pet-interact approach (showOwnedPetStatus) — drop it, or its
	// stale SetArrived callback would fire a range recheck against wherever
	// this walk actually lands instead of the pet it was originally aimed
	// at.
	live.takePetInteract()
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
	obj, ok := l.world.Object(objectID)
	if !ok {
		obj, ok = l.world.Player(objectID)
		if !ok {
			return nil
		}
	}
	target, ok := obj.(world.Tracked)
	if !ok {
		return nil
	}
	return target
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

func (l *GameClientLink) showOwnedPetStatus(live *livePlayer, target world.Tracked, shift bool) bool {
	pet, ok := target.(*summon.Actor)
	if !ok || live == nil || !pet.IsPet() || pet.OwnerID() != live.ObjectID() {
		return false
	}
	// Interacting with an owned summon releases the pending action the client
	// registered for the click before showing the status window; PetStatusShow
	// alone leaves that action outstanding and locks further input.
	live.SendFrame(serverpackets.FrameActionFailed())
	if summonInRange(live, pet, summonInteractApproachRange) {
		live.SendFrame(serverpackets.FramePetStatusShow(pet.SummonType()))
		return true
	}
	if shift || live.move == nil {
		return true
	}
	px, py, pz := pet.Position()
	// Symmetric to walkOrForwardPickup's cancellation above: this walk also
	// redirects live.move's single in-flight target, so any pending pickup
	// walk that target was still driving toward has to go too.
	live.takePickup()
	live.setPetInteract(pet)
	accepted, err := live.move.MoveToLocation(location.Location{X: px, Y: py, Z: pz})
	if err != nil {
		l.log.Warn().Err(err).Msg("move: broadcast")
	}
	if !accepted {
		live.takePetInteract()
	}
	return true
}

func summonInRange(live *livePlayer, pet *summon.Actor, radius int) bool {
	lx, ly, lz := live.Position()
	px, py, pz := pet.Position()
	return location.In3DRange(lx, ly, lz, px, py, pz, radius)
}

// finishPetInteract fires once an approach walk started by showOwnedPetStatus
// arrives (wired through move.Controller.SetArrived), mirroring
// thinkInteract's post-move canDoInteract recheck (PlayerAI.java:445): the
// owner or pet may have moved again meanwhile, so the range and ownership
// gates run again before the status window opens.
func (l *GameClientLink) finishPetInteract(live *livePlayer) {
	pet := live.takePetInteract()
	if pet == nil {
		return
	}
	if l.resolveTarget(pet.ObjectID()) != world.Tracked(pet) {
		return
	}
	if pet.OwnerID() != live.ObjectID() || !summonInRange(live, pet, summonInteractRange) {
		return
	}
	live.SendFrame(serverpackets.FramePetStatusShow(pet.SummonType()))
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
	// up and broadcasts the revive visual (PlayerAI.java:490-501).
	if stand && !live.Dead() && live.FakeDead() {
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
	chair, ok := target.(interface {
		staticobject.Chair
		StaticObjectID() int
	})
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

func (l *GameClientLink) selectLiveTarget(live *livePlayer, target world.Tracked) bool {
	if live == nil || target == nil {
		return false
	}
	if cur := live.Target(); cur != nil && cur.ObjectID() == target.ObjectID() {
		return true
	}
	live.SetTargetTracked(target)
	// Reference: Player.setTarget sends ValidateLocation for the new target
	// before MyTargetSelected, skipped only when the target is the selecting
	// player itself or aboard a boat (Player.java:2477-2479). Boats aren't a
	// ported feature, so every target here is treated as never in one.
	if target.ObjectID() != live.ObjectID() {
		// staticobject.Chair excludes the StaticObject branch, which sends no
		// ValidateLocation in the reference (Player.java:2465-2470); every
		// other target reaching this point is Creature-ish (Hostile,
		// player.Character, summon.Actor) and gets Position()/Heading() via
		// its embedded world.Presence, matching Player.setTarget's
		// ValidateLocation leg sitting strictly inside the `instanceof
		// Creature` branch (Player.java:2474-2475). AttackableBy alone would
		// under-match here: only Hostile and player.Character implement it,
		// silently excluding summon.Actor.
		if _, isStatic := target.(staticobject.Chair); !isStatic {
			if creatureLike, ok := target.(interface {
				Position() (int, int, int)
				Heading() int
			}); ok {
				x, y, z := creatureLike.Position()
				live.SendFrame(serverpackets.FrameValidateLocation(target.ObjectID(), location.Location{X: x, Y: y, Z: z}, creatureLike.Heading()))
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
	live.SetTargetTracked(nil)
	if live.combat != nil {
		live.combat.Stop()
	}
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
	// The reference's single intention slot drops PICK_UP on any subsequent
	// attack click regardless of which thinkAttack branch it takes — most
	// branches here also cancel or redirect the move itself (chase redirect,
	// immediate-swing move.Stop(), a rejection's stopLocked), but even the
	// bow-cooldown branch that leaves the move untouched still replaces the
	// intention. Clear it unconditionally, or a parked ground-pickup
	// approach fires stale against whatever arrival comes next (#1155).
	live.takePickup()
	if !live.combat.Start(combatant) {
		live.SendFrame(serverpackets.FrameActionFailed())
		return false
	}
	return true
}

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
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameAutoAttackStart(live.ObjectID())
	})
}

func (l *GameClientLink) stopLiveAutoAttack(live *livePlayer) {
	if live == nil || !live.SetInCombat(false) {
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

func (l *GameClientLink) broadcastTargetUnselected(live *livePlayer) {
	if l.world == nil {
		return
	}
	x, y, z := live.Position()
	at := location.Location{X: x, Y: y, Z: z}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameTargetUnselected(live.ObjectID(), at)
	}, func(send func(frameReceiver)) {
		l.world.ForEachKnown(live, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}

// broadcastLiveStatus sends live's current HP to its own session and every
// currently known observer, so a health bar reflects damage as it lands
// instead of only the moment the target dies or is reselected.
func (l *GameClientLink) broadcastLiveStatus(live *livePlayer) {
	if live == nil {
		return
	}
	attrs, ok := targetHPAttributes(live)
	if !ok {
		return
	}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameStatusUpdate(live.ObjectID(), attrs)
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

// broadcastLiveMPStatus sends live's current HP and MP to its own session
// and every currently known observer, matching PlayerStatus's
// broadcastStatusUpdate() override, which unconditionally includes CUR_MP
// alongside CUR_HP on every status packet (unlike the generic Creature/Npc
// broadcast, which is HP-only and threshold-gated). Used for MP-only
// changes — a mana-drain tick — where the generic HP broadcast alone would
// leave the client's MP bar stale.
func (l *GameClientLink) broadcastLiveMPStatus(live *livePlayer) {
	if live == nil {
		return
	}
	resources := live.ResourceValues()
	attrs := []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusCurrentHP, Value: int(resources.CurrentHP)},
		{Type: serverpackets.StatusCurrentMP, Value: int(resources.CurrentMP)},
	}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameStatusUpdate(live.ObjectID(), attrs)
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

// updateLiveAbnormalEffect sends live's own session its current active
// abnormal-effect icon list. Unlike broadcastLiveStatus, this packet only
// ever goes to the effected player's own client, matching the reference's
// AbnormalStatusUpdate.
func (l *GameClientLink) updateLiveAbnormalEffect(live *livePlayer) {
	if live == nil {
		return
	}
	entries := live.EffectList().IconEntries(time.Now())
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
	attackableTarget, ok := target.(interface {
		AttackableBy(skilltarget.Creature) bool
	})
	if !ok || !attackableTarget.AttackableBy(attacker) {
		return 0
	}
	return attacker.CharLevel - targetLevel(target)
}

func targetLevel(target world.Tracked) int {
	switch t := target.(type) {
	case *livePlayer:
		return t.CharLevel
	case *npc.Hostile:
		if t.Instance != nil && t.Instance.Template != nil {
			return t.Instance.Template.Level
		}
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
