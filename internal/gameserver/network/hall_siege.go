package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// hallSiegeNotices tells the world what the hall sieges do: as the castle
// sieges do, and flagging the players left on a battlefield as it turns
// off.
type hallSiegeNotices struct{ siegeNotices }

// ClanHallSiegeNotifier returns the hall siege notifier telling the world
// through l.
func ClanHallSiegeNotifier(l *GameClientLink) clanhall.SiegeNotifier {
	return hallSiegeNotices{siegeNotices{l: l}}
}

// Announce sends m to every player in the world at once: a hall siege's
// notices come ahead of the siege states and teleports its step posts to
// the players' queues, and with the combat notices of its battlefield.
func (n hallSiegeNotices) Announce(m siege.Message) {
	n.l.toAllPlayers(func() wire.Frame { return frameSiegeMessage(m) })
}

// SetSiegeState gives each member in the world of clans the siege state
// s, then shows it its UserInfo and everyone who knows it its CharInfo and
// relations, on its own queue.
func (n hallSiegeNotices) SetSiegeState(clans []*clan.Clan, s siege.State) {
	l := n.l
	for _, cl := range clans {
		for _, member := range l.onlineClanMembers(cl, 0) {
			postLive(member, func() {
				member.SetSiegeState(int32(s))
				l.broadcastCharacterInfo(member)
			})
		}
	}
}

// FlagPvP puts every player standing in field on the normal pvp flag, on
// its own queue; a player then standing in a combat zone is not flagged,
// nor is one standing in hall hallID's grounds of another clan than
// ownerID, banished from them just before.
func (n hallSiegeNotices) FlagPvP(field *zone.Siege, hallID, ownerID int32) {
	l := n.l
	banished := map[zone.Actor]bool{}
	if l.zones != nil {
		for _, z := range zone.OfKind[*zone.ClanHall](l.zones) {
			if int32(z.ResidenceID) != hallID {
				continue
			}
			for _, a := range z.Occupants() {
				if p, ok := a.(zone.Player); ok && p.ClanID() != ownerID {
					banished[a] = true
				}
			}
			break
		}
	}
	for _, a := range field.Occupants() {
		za, ok := a.(*liveZoneActor)
		if !ok || banished[a] {
			continue
		}
		live := za.live
		postLive(live, func() {
			if za.ZoneFlags().Has(zone.FlagPvP) {
				return
			}
			l.startPvPFlag(live, false)
		})
	}
}

// hallSiege resolves siegable hall id's siege.
func (l *GameClientLink) hallSiege(id int32) (*clanhall.HallSiege, bool) {
	return l.halls.Siege(id)
}

// requestHallSiegeAttackerList shows the attacking clans of siegable hall
// id, and reports whether id names one. A hall no script runs the siege of
// sends nothing.
func (l *GameClientLink) requestHallSiegeAttackerList(live *livePlayer, id int32) bool {
	s, ok := l.hallSiege(id)
	if !ok {
		return false
	}
	if !s.Scripted() {
		// Silent on purpose: a hall no script runs the siege of has no
		// attacker list to send; the list request holds no pending client
		// action, the window staying empty as for an unknown id.
		return true
	}
	ids := s.Attackers()
	rows := make([]serverpackets.SiegeClan, 0, len(ids))
	for _, cl := range l.resolveSiegeClans(ids) {
		rows = append(rows, siegeClanRow(cl))
	}
	live.SendFrame(serverpackets.FrameSiegeAttackerList(id, rows))
	return true
}

// resolveSiegeClans resolves ids to the clans still existing, in order.
func (l *GameClientLink) resolveSiegeClans(ids []int32) []*clan.Clan {
	out := make([]*clan.Clan, 0, len(ids))
	for _, id := range ids {
		if cl, ok := l.clanService().Table().Get(id); ok {
			out = append(out, cl)
		}
	}
	return out
}

// requestJoinHallSiege registers cl, live's clan, on siegable hall
// req.ID's siege, or drops its registration, and reports whether req.ID
// names one. A clan whose dissolution is pending may not register. A
// refusal is told; live then sees the siege window again, unless no script
// runs the hall's siege, when the registration stops there.
func (l *GameClientLink) requestJoinHallSiege(live *livePlayer, cl *clan.Clan, req clientpackets.RequestJoinSiege) bool {
	s, ok := l.hallSiege(req.ID)
	if !ok {
		return false
	}
	if req.IsJoining == 1 {
		if l.halls.Now() < cl.Info().DissolvingExpiry {
			live.SendFrame(serverpackets.FrameSystemMessage(siege.MsgDissolutionInProgress))
			return true
		}
		switch result, refusal := s.Register(cl); result {
		case clanhall.RegisterRefused:
			live.SendFrame(frameSiegeMessage(refusal))
		case clanhall.RegisterNoSiege:
			// Silent on purpose: a hall no script runs the siege of takes
			// no attacker and ends the request there with no reply, past
			// the checks that do not need its siege. The siege window's
			// registration holds no pending client action.
			return true
		}
	} else {
		s.Unregister(cl.ID())
	}
	live.SendFrame(l.frameHallSiegeInfo(live, s))
	return true
}

// frameHallSiegeInfo builds hall siege s's window as live sees it.
func (l *GameClientLink) frameHallSiegeInfo(live *livePlayer, s *clanhall.HallSiege) wire.Frame {
	var ownerID int32
	if v, ok := l.halls.View(s.ID()); ok {
		ownerID = v.OwnerID
	}
	v := serverpackets.SiegeInfoView{
		CastleID:  s.ID(),
		IsLord:    ownerID == live.ClanID() && live.IsClanLeader(),
		OwnerID:   ownerID,
		Now:       int32(l.halls.Now() / 1000),
		SiegeDate: int32(s.Date() / 1000),
	}
	if owner, ok := l.clanService().Table().Get(ownerID); ok && ownerID > 0 {
		info := owner.Info()
		v.Owner = true
		v.OwnerName, v.LeaderName, v.AllyID, v.AllyName = info.Name, info.LeaderName, info.AllyID, info.AllyName
	}
	return serverpackets.FrameSiegeInfo(v)
}

// showHallSiegeMessenger answers a talk to a siegable hall's messenger, and
// reports whether f is one: the leader of the clan owning the hall reads
// siege/01.htm, or siege/03.htm while its siege is under way; anyone else
// reads siege/02.htm while it is under way and otherwise sees the siege
// window. A page comes with ActionFailed.
func (l *GameClientLink) showHallSiegeMessenger(live *livePlayer, f *npc.Folk) bool {
	_, h := l.npcResidence(f.Instance)
	if h == nil {
		return false
	}
	s, ok := l.hallSiege(int32(h.ID))
	if !ok {
		return false
	}
	var ownerID int32
	if v, ok := l.halls.View(s.ID()); ok {
		ownerID = v.OwnerID
	}
	lord := live.IsClanLeader() && ownerID != 0 && ownerID == live.ClanID()
	underSiege := s.InProgress()
	var page string
	switch {
	case lord && underSiege:
		page = "03"
	case lord:
		page = "01"
	case underSiege:
		page = "02"
	default:
		live.SendFrame(l.frameHallSiegeInfo(live, s))
		return true
	}
	sendFilledHTML(live, f.ObjectID(), l.setPage("data/html/siege/"+page+".htm"), 0)
	live.SendFrame(serverpackets.FrameActionFailed())
	return true
}

// frameNpcSystemMessage builds the system message an NPC sends around it.
func frameNpcSystemMessage(e event.NpcSystemMessage) wire.Frame {
	return frameSiegeMessage(siege.Message{ID: e.ID, Text: e.Text, CastleID: e.CastleID, Number: e.Number, HasNumber: e.HasNumber})
}
