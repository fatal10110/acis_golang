package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// leadsSiegeClan reports whether live leads cl and cl has the level a clan
// needs to register on a siege, so live holds the siege skills.
func (l *GameClientLink) leadsSiegeClan(live *livePlayer, cl *clan.Clan) bool {
	return cl.IsLeader(live.ObjectID()) && cl.Level() >= l.sieges.MinClanLevel()
}

// giveSiegeSkills gives live, on its queue, the siege skills of a clan
// leader; nothing is stored and no skill list follows.
func (l *GameClientLink) giveSiegeSkills(live *livePlayer) {
	if l.skills == nil {
		return
	}
	if err := l.skills.GrantTransientSkills(live.Character, modelskill.SiegeSkills(live.IsNoble())); err != nil {
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("give siege skills")
	}
}

// takeSiegeSkills takes the siege skills of a clan leader from live, on its
// queue; a noble loses Build Advanced Headquarters with them. Nothing is
// stored and no skill list follows.
func (l *GameClientLink) takeSiegeSkills(live *livePlayer) {
	if l.skills == nil {
		return
	}
	for _, ref := range modelskill.SiegeSkills(live.IsNoble()) {
		l.removeLiveSkill(live, int(ref.ID), false)
	}
}
