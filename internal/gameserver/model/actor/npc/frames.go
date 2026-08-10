package npc

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// ErrNoWorld is returned by a Broadcast* method when SetWorld has not been
// called yet — the actor has no known-observer list to broadcast to.
var ErrNoWorld = errors.New("npc: SetWorld not called")

// ErrNoFrameBuilder is returned by a Broadcast* method when SetFrameBuilder
// has not been called yet — a spawn site omitted wiring the network layer's
// packet builder onto this actor.
var ErrNoFrameBuilder = errors.New("npc: SetFrameBuilder not called")

// FrameBuilder translates a Hostile's or EffectPoint's broadcast-worthy
// state changes into wire frames. The network layer implements it (see
// serverpackets.NpcFrameBuilder) so this package never constructs packets
// or touches wire encoding itself — it only knows *when* to broadcast and
// *who* is listening.
type FrameBuilder interface {
	Info(npcinfo.Snapshot) wire.Frame
	Attack(snapshot attack.Snapshot) wire.Frame
	SkillUse(casterID int32, casterAt location.Location, targetID int32, targetAt location.Location, skillID, level int32, hitTime, reuseDelay int, success bool) wire.Frame
	SkillLaunched(objectID, skillID, level int32, targetIDs []int32) wire.Frame
	Die(objectID int32) wire.Frame
	Move(objectID int32, event move.Event) wire.Frame
	MoveToPawn(objectID, targetID int32, distance int, origin location.Location) wire.Frame
	Stop(objectID int32, at location.Location, heading int) wire.Frame
	Status(objectID int32, maxHP, curHP int) wire.Frame
}
