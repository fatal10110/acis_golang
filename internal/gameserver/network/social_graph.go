package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
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

func (g socialGraph) Party(objectID int32) (player.Party, bool) {
	if g.parties == nil {
		return nil, false
	}
	ref, ok := g.parties.PartyRef(objectID)
	if !ok {
		return nil, false
	}
	return socialParty{ref: ref}, true
}

// socialParty answers one party's leader and members as characters.
type socialParty struct {
	ref party.PartyRef[*livePlayer]
}

func (p socialParty) Leader() *player.Character { return p.ref.Leader().Character }

func (p socialParty) Members() []*player.Character {
	members := p.ref.Members()
	out := make([]*player.Character, len(members))
	for i, m := range members {
		out[i] = m.Character
	}
	return out
}

func (g socialGraph) SameChannel(a, b int32) bool {
	return g.parties != nil && g.parties.SameChannel(a, b)
}

func (g socialGraph) Channel(objectID int32) (*player.Character, [][]*player.Character, bool) {
	if g.parties == nil {
		return nil, nil, false
	}
	view, ok := g.parties.Channel(objectID)
	if !ok {
		return nil, nil, false
	}
	parties := make([][]*player.Character, len(view.Parties))
	members := view.Members
	for i, p := range view.Parties {
		parties[i] = make([]*player.Character, p.Count)
		for j, m := range members[:p.Count] {
			parties[i][j] = m.Character
		}
		members = members[p.Count:]
	}
	return view.Leader.Character, parties, true
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

func (g socialGraph) InAcademy(clanID, objectID int32) bool {
	cl, ok := g.clan(clanID)
	return ok && academyMember(cl, objectID)
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

func (g socialGraph) ClanLevel(clanID int32) int {
	if cl, ok := g.clan(clanID); ok {
		return cl.Level()
	}
	return 0
}

func (g socialGraph) Mentors(clanID, objectID int32) (sponsor, apprentice int32) {
	cl, ok := g.clan(clanID)
	if !ok {
		return 0, 0
	}
	m, ok := cl.Member(objectID)
	if !ok {
		return 0, 0
	}
	return m.Sponsor, m.Apprentice
}

func (g socialGraph) ClanHeadquarters(clanID int32) (x, y, z int, ok bool) {
	cl, found := g.clan(clanID)
	if !found {
		return 0, 0, 0, false
	}
	hq, held := cl.Headquarters()
	if !held {
		return 0, 0, 0, false
	}
	x, y, z = hq.Position()
	return x, y, z, true
}
