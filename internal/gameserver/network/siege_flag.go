package network

import (
	"errors"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// buildHeadquartersSkillID is Build Headquarters, the skill a refusal to
// put up any siege headquarters names.
const buildHeadquartersSkillID = 247

// errSiegeFlagRefused ends a SIEGE_FLAG cast its headquarters check
// refused; the refusal is already told.
var errSiegeFlagRefused = errors.New("siege flag refused")

// headquartersCheck runs the siege headquarters check for live putting one
// up where it stands.
func (l *GameClientLink) headquartersCheck(live *livePlayer) siege.HeadquartersRefusal {
	x, y, z := live.Position()
	site := siege.HeadquartersSite{
		X: x, Y: y, Z: z,
		ClanID:               live.ClanID(),
		Leader:               live.IsClanLeader(),
		OnHeadquartersGround: live.zoneActor != nil && live.zoneActor.ZoneFlags().Has(zone.FlagHQ),
		NearHeadquarters:     func() bool { return l.headquartersNear(live) },
	}
	if cl, ok := l.clanService().ClanOf(live.Character); ok {
		_, site.HasHeadquarters = cl.Headquarters()
	}
	return l.sieges.CheckHeadquarters(site)
}

// headquartersNear reports a siege headquarters within
// siege.HeadquartersRange of live.
func (l *GameClientLink) headquartersNear(live *livePlayer) bool {
	if l.world == nil {
		return false
	}
	near := false
	l.world.ForEachKnownInRadius(live, siege.HeadquartersRange, func(o world.Tracked) {
		if f, ok := o.(*npc.Folk); ok && f.SiegeFlag() {
			near = true
		}
	})
	return near
}

// headquartersRefusalFrame is the message telling why a headquarters may
// not be put up.
func headquartersRefusalFrame(r siege.HeadquartersRefusal) wire.Frame {
	switch r {
	case siege.HeadquartersNotLeader:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyClanLeaderCanIssueCommands)
	case siege.HeadquartersAlreadyUp:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotAnotherHeadquarters)
	case siege.HeadquartersNotHere:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotSetUpBaseHere)
	case siege.HeadquartersTooClose:
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageHeadquartersTooClose)
	default:
		return serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, buildHeadquartersSkillID, 1)
	}
}

// siegeFlagAfterCanCast is the headquarters check a SIEGE_FLAG cast runs
// once its can-cast checks passed; a refusal answers the reason alone.
func (l *GameClientLink) siegeFlagAfterCanCast(live *livePlayer) func() error {
	return func() error {
		if r := l.headquartersCheck(live); r != siege.HeadquartersAllowed {
			live.SendFrame(headquartersRefusalFrame(r))
			return errSiegeFlagRefused
		}
		return nil
	}
}

// siegeFlagHandler lands a SIEGE_FLAG skill: once the headquarters check
// passes again, told to no one when it does not, a siege headquarters of
// the caster's clan stands where the caster does, facing its way, with
// twice the HP for an advanced one.
type siegeFlagHandler struct{ l *GameClientLink }

func (siegeFlagHandler) Types() []string { return []string{"SIEGE_FLAG"} }

func (h siegeFlagHandler) Use(c skillhandler.Cast) {
	if c.Caster == nil || c.Caster.Kind() != actor.KindPlayer {
		return
	}
	l := h.l
	live, ok := l.livePlayerByID(c.Caster.ObjectID())
	if !ok || l.headquartersCheck(live) != siege.HeadquartersAllowed {
		return
	}
	cl, ok := l.clanService().ClanOf(live.Character)
	if !ok {
		return
	}
	npcs := l.npcSpawns.Load()
	if npcs == nil {
		l.log.Warn().Int32("clan_id", cl.ID()).Msg("siege flag: no npc population to place it in")
		return
	}
	x, y, z := live.Position()
	flag, err := npcs.SpawnFixedFolk(npc.SiegeFlagTemplate(cl.Name(), c.Skill.Advanced), x, y, z, live.Heading())
	if err != nil {
		l.log.Error().Err(err).Int32("clan_id", cl.ID()).Msg("siege flag: cannot place it")
		return
	}
	if !flag.StandFor(cl) {
		// Another headquarters of the clan went up meanwhile.
		flag.DeleteNow()
	}
}

// warnHeadquartersAttacked tells the members of the clan the siege
// headquarters f stands for that their base is under attack.
func (l *GameClientLink) warnHeadquartersAttacked(f *npc.Folk) {
	cl, ok := f.HeadquartersClan()
	if !ok {
		return
	}
	l.broadcastToClanQueued(cl, nil, func() wire.Frame {
		return serverpackets.FrameSystemMessage(serverpackets.SystemMessageBaseUnderAttack)
	})
}
