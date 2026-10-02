package clan

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// SurrenderPersonally has c give up, for itself, its clan's war on the
// clan named targetName: c wants peace from then on. A clanless c, or a
// name no clan bears, is ignored. WarSurrenderFailed refuses it when c's
// clan has not declared war on that clan or c already wants peace.
func (s *Service) SurrenderPersonally(c *player.Character, targetName string) (War, WarResult) {
	cl, ok := s.ClanOf(c)
	if !ok {
		return War{}, WarIgnored
	}
	target, ok := s.table.ByName(targetName)
	if !ok {
		return War{}, WarIgnored
	}
	if !cl.AtWarWith(target.id) || !c.RequestPeace() {
		return War{}, WarSurrenderFailed
	}
	return War{Clan: cl, Target: target}, WarDone
}

// EndSurrenderedWar ends war, reporting whether it did, once exactly every
// member of war.Clan but one wants peace; peace reports whether a member is
// online and, if so, whether it wants peace. Every member must be online to
// be counted: one offline member leaves the war on, as the count cannot be
// taken. The end stops both wars: war.Clan's on war.Target, as a stop is,
// with its penalty, and war.Target's own on war.Clan, if any, with none.
func (s *Service) EndSurrenderedWar(war War, peace func(objectID int32) (wants, online bool), now time.Time) bool {
	cl, target := war.Clan, war.Target
	members := cl.Members()
	wanting := 0
	for _, m := range members {
		wants, online := peace(m.ObjectID)
		if !online {
			return false
		}
		if wants {
			wanting++
		}
	}
	if wanting != len(members)-1 {
		return false
	}
	unlock := lockPair(cl, target)
	defer unlock()
	if !s.endWarLocked(cl, target, now) {
		return false
	}
	if _, ok := target.wars[cl.id]; ok {
		delete(target.wars, cl.id)
		delete(cl.attackers, target.id)
		clanID, targetID := target.id, cl.id
		s.write(clanID, "end clan war", func(ctx context.Context, st Store) error { return st.EndWar(ctx, clanID, targetID, 0) })
	}
	return true
}
