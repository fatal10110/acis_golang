package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) broadcastAttack(attacker *livePlayer, snapshot attack.Snapshot) {
	if attacker == nil {
		return
	}

	frame := serverpackets.FrameAttack(snapshot)
	encoded := append([]byte(nil), frame.Bytes()...)
	frame.Release()

	send := func(receiver interface{ SendFrame(wire.Frame) bool }) {
		receiver.SendFrame(wire.BorrowedFrame(append([]byte(nil), encoded...)))
	}
	send(attacker)

	if l.world == nil {
		return
	}
	l.world.ForEachKnown(attacker, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
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
	if selected && l.showOwnedPetStatus(live, target) {
		return
	}
	if selected && l.sitLiveOnChair(live, target) {
		return
	}
	if selected {
		l.attackLiveTarget(live, target)
	}
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
	live.setPickup(ctx, ground)
	if live.move.MoveToLocation(location.Location{X: x, Y: y, Z: z}) {
		return true
	}
	live.takePickup()
	live.SendFrame(serverpackets.FrameActionFailed())
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetTooFar))
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

func (l *GameClientLink) showOwnedPetStatus(live *livePlayer, target world.Tracked) bool {
	pet, ok := target.(*summon.Actor)
	if !ok || live == nil || !pet.IsPet() || pet.OwnerID() != live.ObjectID() {
		return false
	}
	// Interacting with an owned summon releases the pending action the client
	// registered for the click before showing the status window; PetStatusShow
	// alone leaves that action outstanding and locks further input.
	live.SendFrame(serverpackets.FrameActionFailed())
	live.SendFrame(serverpackets.FramePetStatusShow(pet.SummonType()))
	return true
}

func (l *GameClientLink) sitLiveOnChair(live *livePlayer, target world.Tracked) bool {
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
	live.SendFrame(serverpackets.FrameMyTargetSelected(target.ObjectID(), targetColor(live.Character, target)))
	if attrs, ok := targetHPAttributes(target); ok {
		live.SendFrame(serverpackets.FrameStatusUpdate(target.ObjectID(), attrs))
	}
	l.broadcastTargetSelected(live, target)
	return true
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
		AttackableBy(attack.CreatureActor) bool
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
