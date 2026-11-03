package network

import (
	"errors"

	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door/doorhit"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// errSiegeCastRefused ends a cast a siege rule refused once its can-cast
// checks passed; the refusal is already told.
var errSiegeCastRefused = errors.New("siege cast refused")

// siegeCastAfterCanCast is the siege rule a player's cast of def runs once
// its can-cast checks passed, nil for a skill type no siege rule covers;
// target resolves the cast's final target. A refusal answers the reason
// alone.
func (l *GameClientLink) siegeCastAfterCanCast(live *livePlayer, def modelskill.Definition, target func() skilltarget.Actor) func() error {
	switch def.SkillType {
	case "RESURRECT":
		return func() error {
			if msg := l.siegeResurrectRefusal(live); msg != 0 {
				live.SendFrame(serverpackets.FrameSystemMessage(msg))
				return errSiegeCastRefused
			}
			return nil
		}
	case "STRIDER_SIEGE_ASSAULT":
		return func() error {
			if _, ok := l.siegeAssaultDoor(live, target(), def); !ok {
				return errSiegeCastRefused
			}
			return nil
		}
	}
	return nil
}

// siegeResurrectRefusal is the system message refusing live a resurrection
// cast while it stands on the battlefield of a siege in progress, 0 when
// the cast may go on: CANNOT_BE_RESURRECTED_DURING_SIEGE without a clan or
// for a clan on no side (a pending defender included),
// TOWER_DESTROYED_NO_RESURRECTION for the owner or a defender once no life
// control tower of the castle stands, and NO_RESURRECTION_WITHOUT_BASE_CAMP
// for an attacker whose clan holds no headquarters.
func (l *GameClientLink) siegeResurrectRefusal(live *livePlayer) int {
	x, y, z := live.Position()
	s, ok := l.sieges.ActiveAt(x, y, z)
	if !ok {
		return 0
	}
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return serverpackets.SystemMessageCannotBeResurrectedDuringSiege
	}
	switch s.Side(live.ClanID()) {
	case siege.SideOwner, siege.SideDefender:
		if s.AliveLifeTowers() == 0 {
			return serverpackets.SystemMessageTowerDestroyedNoResurrection
		}
	case siege.SideAttacker:
		if _, held := cl.Headquarters(); !held {
			return serverpackets.SystemMessageNoResurrectionWithoutBaseCamp
		}
	default:
		return serverpackets.SystemMessageCannotBeResurrectedDuringSiege
	}
	return 0
}

// siegeAssaultDoor runs the siege check of the STRIDER_SIEGE_ASSAULT skill
// def live casts on target, and returns the door it strikes. In this order,
// live is told S1_CANNOT_BE_USED when it does not ride a strider,
// INVALID_TARGET when target is not a door, and S1_CANNOT_BE_USED when its
// clan does not attack the siege in progress where it stands.
func (l *GameClientLink) siegeAssaultDoor(live *livePlayer, target skilltarget.Actor, def modelskill.Definition) (*door.Object, bool) {
	cannot := func() {
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(def.ID), int32(def.Level)))
	}
	if !live.Riding() {
		cannot()
		return nil, false
	}
	var d *door.Object
	switch t := target.(type) {
	case *door.Object:
		d = t
	case doorhit.Target:
		d = t.Object
	}
	if d == nil {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return nil, false
	}
	x, y, z := live.Position()
	if !l.sieges.AttackerAt(live.ClanID(), x, y, z) {
		cannot()
		return nil, false
	}
	return d, true
}

// siegeAssaultCheck is the STRIDER_SIEGE_ASSAULT handler's landing check:
// the cast-time check again, for a caster still in the world.
func (l *GameClientLink) siegeAssaultCheck(caster, target skillhandler.Actor, def modelskill.Definition) (*door.Object, bool) {
	live, ok := l.livePlayerByID(caster.ObjectID())
	if !ok {
		return nil, false
	}
	t, _ := target.(skilltarget.Actor)
	return l.siegeAssaultDoor(live, t, def)
}
