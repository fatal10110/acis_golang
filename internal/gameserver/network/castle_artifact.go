package network

import (
	"errors"

	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// engraveRange is how close, in 3D, a player must stand to the artifact it
// engraves.
const engraveRange = 200

// errTakeCastleRefused ends a TAKE_CASTLE cast its castle check refused;
// the refusal is already told.
var errTakeCastleRefused = errors.New("take castle refused")

// takeCastleSiege runs the castle check of the TAKE_CASTLE skill def live
// casts on target, and returns the siege of the castle the cast would take:
// the castle whose battlefield live stands on. In this order, live is told
// S1_CANNOT_BE_USED when it stands on no castle's battlefield,
// INVALID_TARGET when target is not one of that castle's holy artifacts,
// S1_CANNOT_BE_USED when the siege is not being fought,
// DIST_TOO_FAR_CASTING_STOPPED when it stands engraveRange or farther from
// target, and S1_CANNOT_BE_USED when it is outside the zone where artifacts
// are engraved or its clan does not attack. When announce is set, a check
// that passes tells the owner and the defenders the opponents started
// engraving.
func (l *GameClientLink) takeCastleSiege(live *livePlayer, target *npc.Folk, def modelskill.Definition, announce bool) (*siege.Siege, bool) {
	cannot := func() {
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(def.ID), int32(def.Level)))
	}
	x, y, z := live.Position()
	s, ok := l.sieges.At(x, y, z)
	switch {
	case !ok:
		cannot()
	case target == nil || !target.HolyThing() || !s.Castle().ArtifactNPC(target.NpcID()):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
	case !s.InProgress():
		cannot()
	case !inEngraveRange(live, target):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageDistTooFarCastingStopped))
	case live.zoneActor == nil || !live.zoneActor.ZoneFlags().Has(zone.FlagCastOnArtifact):
		cannot()
	case !s.CheckSides(live.ClanID(), siege.SideAttacker):
		cannot()
	default:
		if announce {
			s.Tell(siege.Message{ID: siege.MsgOpponentStartedEngraving}, siege.SideDefender)
		}
		return s, true
	}
	return nil, false
}

func inEngraveRange(live *livePlayer, artifact *npc.Folk) bool {
	lx, ly, lz := live.Position()
	ax, ay, az := artifact.Position()
	return location.In3DRadius(lx, ly, lz, ax, ay, az, engraveRange)
}

// takeCastleAfterCanCast is the castle check a TAKE_CASTLE cast on target
// runs once its can-cast checks passed, the opponents told it started; a
// refusal answers the reason alone.
func (l *GameClientLink) takeCastleAfterCanCast(live *livePlayer, def modelskill.Definition, target skilltarget.Actor) func() error {
	return func() error {
		artifact, _ := target.(*npc.Folk)
		if _, ok := l.takeCastleSiege(live, artifact, def, true); !ok {
			return errTakeCastleRefused
		}
		return nil
	}
}

// takeCastleHandler lands a TAKE_CASTLE skill. Only a clan leader's cast
// does anything: its castle check runs again, telling the leader a
// refusal, then the leader's clan engraves its ruler on the artifact, and
// the clan whose ruler every artifact of the castle then carries takes the
// castle (setCastleOwner).
type takeCastleHandler struct{ l *GameClientLink }

func (takeCastleHandler) Types() []string { return []string{"TAKE_CASTLE"} }

func (h takeCastleHandler) Use(c skillhandler.Cast) {
	if c.Caster == nil || c.Caster.Kind() != actor.KindPlayer || len(c.Targets) == 0 {
		return
	}
	live, ok := h.l.livePlayerByID(c.Caster.ObjectID())
	if !ok || !live.IsClanLeader() {
		return
	}
	artifact, _ := c.Targets[0].(*npc.Folk)
	s, ok := h.l.takeCastleSiege(live, artifact, c.Skill, false)
	if !ok {
		return
	}
	cl, member := h.l.clanService().ClanOf(live.Character)
	if !member {
		return
	}
	if s.Engrave(cl, artifact.ObjectID()) {
		h.l.setCastleOwner(live, s.Castle(), cl)
	}
}
