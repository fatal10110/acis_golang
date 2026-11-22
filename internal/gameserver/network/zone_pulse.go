package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/zonepulse"
)

// systemMessageTrapDeviceTripped is A_TRAP_DEVICE_HAS_BEEN_TRIPPED.
const systemMessageTrapDeviceTripped = 714

// zonePulsesOf builds the periodic tasks of the effect and damage zones of
// ix, run on a queue of their own. A link without a world runs none.
func (l *GameClientLink) zonePulsesOf(ix *zone.Index) *zonepulse.Pulses {
	if l.world == nil {
		return nil
	}
	if l.zonePulses == nil {
		l.zonePulses = l.queues.NewQueue("zone-pulses")
	}
	return zonepulse.New(ix, zonepulse.Config{
		Queue:       l.zonePulses,
		Objects:     l.world,
		Skills:      l.skills,
		Effects:     zonePulseEffects{},
		TrapTripped: l.trapTripped,
	})
}

// castleSiegeRunning reports, each time it is asked, whether the siege of
// castleID is in progress: a castle trap is live only then. A link without
// a world arms no trap.
func (l *GameClientLink) castleSiegeRunning(castleID int) bool {
	if l.world == nil || l.sieges == nil {
		return false
	}
	s, ok := l.sieges.Get(castleID)
	return ok && s.InProgress()
}

// trapTripped tells the defenders of castleID's siege that one of its traps
// started hurting (A_TRAP_DEVICE_HAS_BEEN_TRIPPED): the online members of
// its owner and defender clans.
func (l *GameClientLink) trapTripped(castleID int) {
	if l.sieges == nil {
		return
	}
	s, ok := l.sieges.Get(castleID)
	if !ok {
		return
	}
	defenders := s.Defenders()
	clans := make([]*clan.Clan, 0, len(defenders))
	for _, d := range defenders {
		clans = append(clans, d.Clan)
	}
	SiegeNotifier(l).TellClans(clans, siege.Message{ID: systemMessageTrapDeviceTripped})
}

// zonePulseEffects lands an effect zone's skills on an occupant, the
// occupant being both caster and target.
type zonePulseEffects struct{}

// Conditions evaluates def's conditions with target as caster and target,
// telling a player the failed condition's message.
func (zonePulseEffects) Conditions(target zonepulse.Target, def modelskill.Definition) bool {
	source, _ := target.(conditions.Source)
	clause, ok := conditions.EvaluateSkill(def, source, target)
	if !ok {
		if live, isPlayer := target.(*livePlayer); isPlayer {
			sendSkillConditionFailure(live, clause, def.ID)
		}
	}
	return ok
}

// Land lands def's effects on target, telling a player of each effect it
// resisted.
func (zonePulseEffects) Land(target zonepulse.Target, def modelskill.Definition) {
	effector, ok := target.(effect.Actor)
	if !ok {
		return
	}
	effected, ok := target.(skillhandler.Actor)
	if !ok {
		return
	}
	resisted := skillhandler.LandEffects(effector, effected, def)
	live, isPlayer := target.(*livePlayer)
	if !isPlayer {
		return
	}
	for range resisted {
		live.SendFrame(serverpackets.FrameSystemMessageStringSkillName(serverpackets.SystemMessageS1ResistedYourS2, live.CharacterName(), int32(def.ID), int32(def.Level)))
	}
}
