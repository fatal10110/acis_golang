package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// socialGraph answers a character's party and clan standing from the
// link's party registry and clan table. Both guard their own state, so any
// goroutine may ask.
type socialGraph struct {
	parties *partyRegistry
	clans   *clan.Service
}

var _ player.SocialGraph = socialGraph{}

func (g socialGraph) InParty(objectID int32) bool {
	return g.parties != nil && g.parties.InParty(objectID)
}

func (g socialGraph) SameParty(a, b int32) bool {
	return g.parties != nil && g.parties.SameParty(a, b)
}

func (g socialGraph) PartyMembers(objectID int32) []*player.Character {
	if g.parties == nil {
		return nil
	}
	view, ok := g.parties.View(objectID)
	if !ok {
		return nil
	}
	out := make([]*player.Character, len(view.Members))
	for i, m := range view.Members {
		out[i] = m.Character
	}
	return out
}

func (g socialGraph) SameChannel(a, b int32) bool {
	return g.parties != nil && g.parties.SameChannel(a, b)
}

func (g socialGraph) clan(id int32) (*clan.Clan, bool) {
	if g.clans == nil {
		return nil, false
	}
	return g.clans.Table().Get(id)
}

func (g socialGraph) AllyID(clanID int32) int32 {
	if cl, ok := g.clan(clanID); ok {
		return cl.AllyID()
	}
	return 0
}

func (g socialGraph) ClanLeaderID(clanID int32) int32 {
	if cl, ok := g.clan(clanID); ok {
		return cl.LeaderID()
	}
	return 0
}

func (g socialGraph) AtWar(clanID, targetClanID int32) bool {
	cl, ok := g.clan(clanID)
	return ok && cl.AtWarWith(targetClanID)
}

func (g socialGraph) ClanCastleID(clanID int32) int32 {
	if cl, ok := g.clan(clanID); ok {
		return cl.CastleID()
	}
	return 0
}

func (g socialGraph) ClanHallID(clanID int32) int32 {
	if cl, ok := g.clan(clanID); ok {
		return cl.HallID()
	}
	return 0
}
