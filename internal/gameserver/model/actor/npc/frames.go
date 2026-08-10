package npc

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ErrNoWorld is returned by a Broadcast* method when SetWorld has not been
// called yet — the actor has no known-observer list to broadcast to.
var ErrNoWorld = errors.New("npc: SetWorld not called")

// ErrNoFrameBuilder is returned by a Broadcast* method when SetFrameBuilder
// has not been called yet — a spawn site omitted wiring the network layer's
// packet builder onto this actor.
var ErrNoFrameBuilder = errors.New("npc: SetFrameBuilder not called")

// FrameBuilder translates a Hostile's or EffectPoint's broadcast-worthy
// state changes into wire frames and delivers them to receivers. The
// network layer implements it (see serverpackets.NpcFrameBuilder) so this
// package never constructs packets or touches wire encoding itself — it
// only knows *when* to broadcast and *who* is listening.
type FrameBuilder interface {
	Attack(receivers []world.Tracked, snapshot attack.Snapshot)
	SkillUse(receivers []world.Tracked, casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int, success bool)
	SkillLaunched(receivers []world.Tracked, objectID, skillID, level int32, targetIDs []int32)
	Die(receivers []world.Tracked, objectID int32)
	Move(receivers []world.Tracked, objectID int32, event move.Event)
	MoveToPawn(receivers []world.Tracked, objectID, targetID int32, distance int, origin location.Location)
	Stop(receivers []world.Tracked, objectID int32, at location.Location, heading int)
	Status(receivers []world.Tracked, objectID int32, maxHP, curHP int)
}
