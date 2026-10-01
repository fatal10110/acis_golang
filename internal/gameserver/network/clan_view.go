package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// clanFields is c's clan as UserInfo and CharInfo show it.
func (l *GameClientLink) clanFields(c *player.Character) serverpackets.ClanFields {
	if l == nil || l.clans == nil {
		return serverpackets.ClanFields{}
	}
	cl, ok := l.clans.ClanOf(c)
	if !ok {
		return serverpackets.ClanFields{}
	}
	info := cl.Info()
	m, _ := cl.Member(c.ID)
	return serverpackets.ClanFields{
		CrestID: info.CrestID, CrestLargeID: info.CrestLarge,
		AllyID: info.AllyID, AllyCrestID: info.AllyCrestID,
		Leader:     info.LeaderID == c.ID,
		Privileges: cl.MemberPrivileges(c.ID),
		PledgeType: int32(m.PledgeType),
	}
}

// liveMemberRow is a roster row read off live player c, the way a
// member-list update about a player is built.
func liveMemberRow(c *player.Character, m clan.Member) serverpackets.PledgeMemberListMember {
	return serverpackets.PledgeMemberListMember{
		Name: c.Name, Level: int32(c.Level()), ClassID: int32(c.ClassID()),
		Sex: int32(c.Sex), Race: int32(c.Race), OnlineObjectID: c.ObjectID(),
		PledgeType: int32(m.PledgeType), HasSponsor: m.Sponsor != 0 || m.Apprentice != 0,
	}
}

// memberRow is m's roster row: an online member's values are read off its
// live character, an offline member's are the stored ones. self, when not
// nil, is a member still entering the world, not yet found there.
func (l *GameClientLink) memberRow(m clan.Member, self ...*livePlayer) serverpackets.PledgeMemberListMember {
	if m.Online {
		for _, s := range self {
			if s != nil && s.ObjectID() == m.ObjectID {
				return liveMemberRow(s.Character, m)
			}
		}
		if live, ok := l.livePlayerByID(m.ObjectID); ok {
			return liveMemberRow(live.Character, m)
		}
	}
	return serverpackets.PledgeMemberListMember{
		Name: m.Name, Level: int32(m.Level), ClassID: int32(m.ClassID),
		Sex: int32(m.Sex), Race: int32(m.Race), PledgeType: int32(m.PledgeType),
		HasSponsor: m.Sponsor != 0 || m.Apprentice != 0,
	}
}

// memberUpdateRow is the row a member-list update built from a roster
// entry carries: an offline member shows sex and race 0.
func (l *GameClientLink) memberUpdateRow(m clan.Member) serverpackets.PledgeMemberListMember {
	row := l.memberRow(m)
	if row.OnlineObjectID == 0 {
		row.Sex, row.Race = 0, 0
	}
	return row
}

// framePledgeMemberList builds cl's main-clan roster with its header; see
// memberRow for self.
func (l *GameClientLink) framePledgeMemberList(cl *clan.Clan, self ...*livePlayer) wire.Frame {
	info := cl.Info()
	members := cl.Members()
	rows := make([]serverpackets.PledgeMemberListMember, 0, len(members))
	for _, m := range members {
		rows = append(rows, l.memberRow(m, self...))
	}
	return serverpackets.FramePledgeShowMemberListAll(serverpackets.PledgeMemberList{
		ClanID: info.ID, PledgeType: clan.SubunitMain, PledgeName: info.Name, LeaderName: info.LeaderName,
		CrestID: info.CrestID, Level: int32(info.Level), CastleID: info.CastleID, ClanHallID: info.HallID,
		Rank: int32(info.Rank), Reputation: int32(info.Reputation), Dissolving: info.DissolvingExpiry > 0,
		AllyID: info.AllyID, AllyName: info.AllyName, AllyCrestID: info.AllyCrestID, AtWar: info.AtWar,
		Members: rows,
	})
}

// framePledgeShowInfoUpdate builds cl's header refresh.
func framePledgeShowInfoUpdate(cl *clan.Clan) wire.Frame {
	info := cl.Info()
	return serverpackets.FramePledgeShowInfoUpdate(serverpackets.PledgeHeader{
		ClanID: info.ID, CrestID: info.CrestID, Level: int32(info.Level),
		CastleID: info.CastleID, ClanHallID: info.HallID, Rank: int32(info.Rank),
		Reputation: int32(info.Reputation), Dissolving: info.DissolvingExpiry > 0,
		AllyID: info.AllyID, AllyName: info.AllyName, AllyCrestID: info.AllyCrestID, AtWar: info.AtWar,
	})
}

// onlineClanMembers returns cl's members in the world, except exceptID.
func (l *GameClientLink) onlineClanMembers(cl *clan.Clan, exceptID int32) []*livePlayer {
	var out []*livePlayer
	for _, id := range cl.OnlineMemberIDs() {
		if id == exceptID {
			continue
		}
		if live, ok := l.livePlayerByID(id); ok {
			out = append(out, live)
		}
	}
	return out
}

// broadcastToClan sends each built packet, in order, to every member of cl
// in the world except exceptID (0 excepts nobody). Each packet is built
// once and every member gets its own copy.
func (l *GameClientLink) broadcastToClan(cl *clan.Clan, exceptID int32, builds ...func() wire.Frame) {
	recipients := l.onlineClanMembers(cl, exceptID)
	for _, build := range builds {
		broadcastFrame(build, func(send func(frameReceiver)) {
			for _, live := range recipients {
				send(live)
			}
		})
	}
}

// broadcastClanStatus refreshes every online member's clan window and
// status: the roster cleared and resent, then UserInfo.
func (l *GameClientLink) broadcastClanStatus(cl *clan.Clan) {
	for _, live := range l.onlineClanMembers(cl, 0) {
		live.SendFrame(serverpackets.FramePledgeShowMemberListDeleteAll())
		live.SendFrame(l.framePledgeMemberList(cl))
		live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	}
}

// clanService is the link's clan service; a link built without one runs
// with no clan at all.
func (l *GameClientLink) clanService() *clan.Service {
	if l.clans == nil {
		return clan.NewService(nil, nil, nil, nil, clan.DefaultConfig(), nil, l.log)
	}
	return l.clans
}
