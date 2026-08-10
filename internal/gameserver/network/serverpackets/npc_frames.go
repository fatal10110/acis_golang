package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// NpcFrameBuilder builds the wire frames for npc.Hostile and
// npc.EffectPoint broadcasts and delivers them to receivers, keeping the
// model layer ignorant of packet construction, wire encoding, and frame
// delivery. Stateless: one value serves every live NPC.
type NpcFrameBuilder struct{}

// Attack builds and sends the attack packet for snapshot.
func (NpcFrameBuilder) Attack(receivers []world.Tracked, snapshot attack.Snapshot) {
	sendFrame(receivers, func() wire.Frame { return FrameAttack(snapshot) })
}

// SkillUse builds and sends the cast-start animation packet from caster to
// target.
func (NpcFrameBuilder) SkillUse(receivers []world.Tracked, casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int, success bool) {
	sendFrame(receivers, func() wire.Frame {
		return FrameMagicSkillUse(
			SkillCastObject{ObjectID: casterID, Location: casterAt},
			SkillCastObject{ObjectID: targetID, Location: targetAt},
			skillID, level, hitTime, reuseDelay, success,
		)
	})
}

// SkillLaunched builds and sends the cast-launch target packet.
func (NpcFrameBuilder) SkillLaunched(receivers []world.Tracked, objectID, skillID, level int32, targetIDs []int32) {
	sendFrame(receivers, func() wire.Frame { return FrameMagicSkillLaunched(objectID, skillID, level, targetIDs) })
}

// Die builds and sends the death packet.
func (NpcFrameBuilder) Die(receivers []world.Tracked, objectID int32) {
	sendFrame(receivers, func() wire.Frame { return FrameDie(objectID, DieOptions{}) })
}

// Move builds and sends a MoveToLocation packet for event.
func (NpcFrameBuilder) Move(receivers []world.Tracked, objectID int32, event move.Event) {
	sendFrame(receivers, func() wire.Frame { return FrameMove(objectID, event) })
}

// MoveToPawn builds and sends a rotation-only MoveToPawn notice toward
// target.
func (NpcFrameBuilder) MoveToPawn(receivers []world.Tracked, objectID, targetID int32, distance int, origin location.Location) {
	sendFrame(receivers, func() wire.Frame { return FrameMoveToPawn(objectID, targetID, distance, origin) })
}

// Stop builds and sends a stop-in-place notice.
func (NpcFrameBuilder) Stop(receivers []world.Tracked, objectID int32, at location.Location, heading int) {
	sendFrame(receivers, func() wire.Frame { return FrameStopMove(objectID, at, heading) })
}

// Status builds and sends a current/max HP status packet.
func (NpcFrameBuilder) Status(receivers []world.Tracked, objectID int32, maxHP, curHP int) {
	sendFrame(receivers, func() wire.Frame {
		return FrameStatusUpdate(objectID, []StatusAttribute{
			{Type: StatusMaxHP, Value: maxHP},
			{Type: StatusCurrentHP, Value: curHP},
		})
	})
}

// sendFrame lazily builds one frame the first time it finds a
// frame-capable receiver in receivers, then delivers an independently
// owned copy to each one — never handing the same frame to more than one
// recipient, since SendFrame consumes and may encrypt its frame in place.
func sendFrame(receivers []world.Tracked, build func() wire.Frame) {
	var frame wire.Frame
	built := false
	defer func() { frame.Release() }()
	for _, o := range receivers {
		receiver, ok := o.(interface{ SendFrame(wire.Frame) bool })
		if !ok {
			continue
		}
		if !built {
			frame = build()
			built = true
		}
		owned, copied := wire.CopyFrame(frame)
		if copied {
			receiver.SendFrame(owned)
		}
	}
}
