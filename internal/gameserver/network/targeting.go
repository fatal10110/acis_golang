package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
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

func (l *GameClientLink) handleTargetAction(ctx context.Context, live *livePlayer, objectID int32, selected bool) {
	target := l.resolveTarget(objectID)
	if target == nil {
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	if live.target == nil || live.target.ObjectID() != target.ObjectID() {
		l.selectLiveTarget(live, target)
		return
	}
	if selected && l.showOwnedPetStatus(live, target) {
		return
	}
	if selected && l.sitLiveOnChair(live, target) {
		return
	}
	if selected && l.pickupLiveGroundItem(ctx, live, target) {
		return
	}
	if selected {
		l.attackLiveTarget(live, target)
	}
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
	if live.target != nil && live.target.ObjectID() == target.ObjectID() {
		return true
	}
	live.target = target
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
	old := live.target
	live.target = nil
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
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameTargetSelected(live.ObjectID(), target.ObjectID(), at))
	})
}

func (l *GameClientLink) broadcastTargetUnselected(live *livePlayer) {
	if l.world == nil {
		return
	}
	x, y, z := live.Position()
	at := location.Location{X: x, Y: y, Z: z}
	l.world.ForEachKnown(live, func(o world.Tracked) {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			return
		}
		receiver.SendFrame(serverpackets.FrameTargetUnselected(live.ObjectID(), at))
	})
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
	return attacker.Level - targetLevel(target)
}

func targetLevel(target world.Tracked) int {
	switch t := target.(type) {
	case *livePlayer:
		return t.Level
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
		return []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusMaxHP, Value: int(t.MaxHP)},
			{Type: serverpackets.StatusCurrentHP, Value: t.CurrentHP()},
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
