package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// NpcFrameBuilder builds the wire frames for npc.Hostile and
// npc.EffectPoint broadcasts, keeping the model layer ignorant of packet
// construction and wire encoding. Stateless: one value serves every live
// NPC.
type NpcFrameBuilder struct{}

// Info builds the current NPCInfo frame from snapshot.
func (NpcFrameBuilder) Info(snapshot npcinfo.Snapshot) wire.Frame {
	return FrameNPCInfo(snapshot)
}

// Attack builds the attack packet for snapshot.
func (NpcFrameBuilder) Attack(snapshot attack.Snapshot) wire.Frame {
	return FrameAttack(snapshot)
}

// FlyTo builds a forced-flight animation packet.
func (NpcFrameBuilder) FlyTo(objectID int32, dest, at location.Location, flight modelskill.Flight) wire.Frame {
	return FrameFlyToLocation(objectID, dest, at, flight)
}

// ValidateLocation builds a forced-location correction packet.
func (NpcFrameBuilder) ValidateLocation(objectID int32, at location.Location, heading int) wire.Frame {
	return FrameValidateLocation(objectID, at, heading)
}

// SkillUse builds the cast-start animation packet from caster to target.
func (NpcFrameBuilder) SkillUse(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int, success bool) wire.Frame {
	return FrameMagicSkillUse(
		SkillCastObject{ObjectID: casterID, Location: casterAt},
		SkillCastObject{ObjectID: targetID, Location: targetAt},
		skillID, level, hitTime, reuseDelay, success,
	)
}

// SkillLaunched builds the cast-launch target packet.
func (NpcFrameBuilder) SkillLaunched(objectID, skillID, level int32, targetIDs []int32) wire.Frame {
	return FrameMagicSkillLaunched(objectID, skillID, level, targetIDs)
}

// SkillCanceled builds the cast-cancel animation packet.
func (NpcFrameBuilder) SkillCanceled(objectID int32) wire.Frame {
	return FrameMagicSkillCanceled(objectID)
}

// Die builds the death packet.
func (NpcFrameBuilder) Die(objectID int32, sweep bool) wire.Frame {
	return FrameDie(objectID, DieOptions{Sweep: sweep})
}

// Move builds a MoveToLocation packet for event.
func (NpcFrameBuilder) Move(objectID int32, event move.Event) wire.Frame {
	return FrameMove(objectID, event)
}

// MoveToPawn builds a rotation-only MoveToPawn notice toward target.
func (NpcFrameBuilder) MoveToPawn(objectID, targetID int32, distance int, origin location.Location) wire.Frame {
	return FrameMoveToPawn(objectID, targetID, distance, origin)
}

// Stop builds a stop-in-place notice.
func (NpcFrameBuilder) Stop(objectID int32, at location.Location, heading int) wire.Frame {
	return FrameStopMove(objectID, at, heading)
}

// Status builds a current/max HP status packet.
func (NpcFrameBuilder) Status(objectID int32, maxHP, curHP int) wire.Frame {
	return FrameStatusUpdate(objectID, []StatusAttribute{
		{Type: StatusMaxHP, Value: maxHP},
		{Type: StatusCurrentHP, Value: curHP},
	})
}
