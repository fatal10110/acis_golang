package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
	"github.com/fatal10110/acis_golang/internal/gameserver/partymatch"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// roomRegistry is the registry of the party-matching rooms live players
// open, and of the players waiting for one.
type roomRegistry = partymatch.Registry[*livePlayer]

// dispatchPartyMatch decodes and runs one top-level party-matching packet.
// It reports false once the connection has been closed for a malformed
// packet.
func (l *GameClientLink) dispatchPartyMatch(client *Client, live *livePlayer, opcode byte, payload []byte) bool {
	switch opcode {
	case clientpackets.OpcodeRequestListPartyWaiting:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestListPartyWaiting, l.requestListPartyWaiting)
	case clientpackets.OpcodeRequestManagePartyRoom:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestManagePartyRoom, l.requestManagePartyRoom)
	case clientpackets.OpcodeRequestJoinPartyRoom:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestJoinPartyRoom, l.requestJoinPartyRoom)
	}
	return true
}

// dispatchPartyMatchExtended decodes and runs one extended party-matching
// packet; see dispatchPartyMatch.
func (l *GameClientLink) dispatchPartyMatchExtended(client *Client, live *livePlayer, second uint16, payload []byte) bool {
	switch second {
	case clientpackets.OpcodeRequestOustFromPartyRoom:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestOustFromPartyRoom, l.requestOustFromPartyRoom)
	case clientpackets.OpcodeRequestDismissPartyRoom:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestDismissPartyRoom, l.requestDismissPartyRoom)
	case clientpackets.OpcodeRequestWithdrawPartyRoom:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestWithdrawPartyRoom, l.requestWithdrawPartyRoom)
	case clientpackets.OpcodeRequestAskJoinPartyRoom:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestAskJoinPartyRoom, l.requestAskJoinPartyRoom)
	case clientpackets.OpcodeAnswerJoinPartyRoom:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeAnswerJoinPartyRoom, l.answerJoinPartyRoom)
	case clientpackets.OpcodeRequestListPartyMatchingWaitingRoom:
		return dispatchLive(l, client, live, payload, clientpackets.DecodeRequestListPartyMatchingWaitingRoom, l.requestListWaitingPlayers)
	case clientpackets.OpcodeRequestExitPartyMatchingWaitingRoom:
		// The request carries no body; leaving the waiting list answers
		// nothing, the client having closed its window already.
		if live != nil {
			onLive(live, func() { l.rooms.RemoveWaiting(live) })
		}
	}
	return true
}

// requestListPartyWaiting opens live's party-matching window. In a room,
// it shows that room again. Otherwise live joins the waiting list and is
// shown the rooms it asked for, unless it is a party member that does not
// lead its party.
func (l *GameClientLink) requestListPartyWaiting(live *livePlayer, req clientpackets.RequestListPartyWaiting) {
	if room, ok := l.rooms.RoomOf(live.ObjectID()); ok {
		live.SendFrame(serverpackets.FramePartyMatchDetail(roomTerms(room)))
		live.SendFrame(serverpackets.FrameExPartyRoomMember(int32(partymatch.ListRevised), l.roomMemberRows(room)))
		l.broadcastCharacterInfo(live)
		return
	}
	if view, ok := l.parties.View(live.ObjectID()); ok && view.Leader.ObjectID() != live.ObjectID() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantViewPartyRooms))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	l.rooms.AddWaiting(live)
	live.SendFrame(serverpackets.FramePartyMatchList(roomRows(l.rooms.Rooms(live, req.Location, req.LevelMode, l.nearTo(live)))))
}

// requestManagePartyRoom opens a room live leads, taking its party members
// in with it, or revises the room live leads. A player off the waiting
// list opens nothing, and a revision of a room live does not lead changes
// nothing; neither answers, the room window having sent the request and
// waiting on nothing.
func (l *GameClientLink) requestManagePartyRoom(live *livePlayer, req clientpackets.RequestManagePartyRoom) {
	settings := partymatch.Settings{
		MaxMembers: req.MaxMembers,
		MinLevel:   req.MinLevel,
		MaxLevel:   req.MaxLevel,
		Loot:       req.Loot,
		Title:      req.Title,
	}
	if req.RoomID > 0 {
		l.applyRoomNotices(l.rooms.Revise(live, req.RoomID, settings, l.roomLocation(live)))
		return
	}
	var members []*livePlayer
	if view, ok := l.parties.View(live.ObjectID()); ok {
		members = view.Members
	}
	l.applyRoomNotices(l.rooms.Open(live, settings, l.roomLocation(live), members))
}

// requestJoinPartyRoom enters live into the room it picked, or into the
// first one its window lists. No such room, or one live may not enter, is
// refused; a player off the waiting list enters nothing and hears nothing.
func (l *GameClientLink) requestJoinPartyRoom(live *livePlayer, req clientpackets.RequestJoinPartyRoom) {
	notices, status := l.rooms.Join(live, req.RoomID, req.Location, req.LevelMode, l.nearTo(live))
	if status == partymatch.JoinRefused {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantEnterPartyRoom))
		return
	}
	l.applyRoomNotices(notices)
}

// requestOustFromPartyRoom takes a member out of the room live leads and
// back to the waiting list. A member of live's own party stays. Anyone but
// the room's leader, or a target in no room, ousts nobody and hears
// nothing.
func (l *GameClientLink) requestOustFromPartyRoom(live *livePlayer, req clientpackets.ObjectTarget) {
	target, ok := l.livePlayerByID(req.ObjectID)
	if !ok {
		return
	}
	notices, status := l.rooms.Oust(live, target, l.sameParty)
	switch status {
	case partymatch.OustPartyMember:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotDismissPartyMember))
	case partymatch.Ousted:
		l.applyRoomNotices(notices)
		target.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOustedFromPartyRoom))
	}
}

// requestDismissPartyRoom disbands the room live leads. A room live does
// not lead stays: only its leader's window offers the command, so the
// refusal answers nothing.
func (l *GameClientLink) requestDismissPartyRoom(live *livePlayer, req clientpackets.PartyRoomRef) {
	l.applyRoomNotices(l.rooms.Dismiss(live, req.RoomID))
}

// requestWithdrawPartyRoom takes live out of a room. A member of the
// leader's party stays, and an unknown room changes nothing; neither
// answers.
func (l *GameClientLink) requestWithdrawPartyRoom(live *livePlayer, req clientpackets.PartyRoomRef) {
	notices, status := l.rooms.Withdraw(live, req.RoomID, l.sameParty)
	if status != partymatch.Withdrew {
		return
	}
	l.applyRoomNotices(notices)
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePartyRoomExited))
}

// requestAskJoinPartyRoom invites the named player into live's room.
func (l *GameClientLink) requestAskJoinPartyRoom(live *livePlayer, req clientpackets.TargetName) {
	target, ok := l.livePlayerByName(req.Name)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	if l.tradeBook().Invite(tradebook.KindPartyRoom, live.ObjectID(), target.ObjectID(), false).Status != tradebook.RequestStarted {
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
		return
	}
	target.SendFrame(serverpackets.FrameExAskJoinPartyRoom(live.Name))
}

// answerJoinPartyRoom answers live's pending room invitation. With none
// pending, or its inviter gone, live is told its target is not found. A
// refusal tells the inviter; an acceptance enters live into the inviter's
// room as requestJoinPartyRoom does.
func (l *GameClientLink) answerJoinPartyRoom(live *livePlayer, req clientpackets.Response) {
	requesterID, ok := l.tradeBook().TakeInvite(tradebook.KindPartyRoom, live.ObjectID())
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	partner, ok := l.livePlayerByID(requesterID)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	if req.Response != 1 {
		partner.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessagePartyMatchingRequestNoResponse))
		return
	}
	notices, status := l.rooms.Answer(live, partner)
	if status == partymatch.JoinRefused {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantEnterPartyRoom))
		return
	}
	l.applyRoomNotices(notices)
}

// requestListWaitingPlayers lists the waiting players of the asked level
// range.
func (l *GameClientLink) requestListWaitingPlayers(live *livePlayer, req clientpackets.RequestListPartyMatchingWaitingRoom) {
	players := l.rooms.Waiting(live, req.MinLevel, req.MaxLevel)
	rows := make([]serverpackets.WaitingPlayer, len(players))
	for i, p := range players {
		rows[i] = serverpackets.WaitingPlayer{Name: p.Name, ClassID: int32(p.ClassID()), Level: int32(p.Level())}
	}
	live.SendFrame(serverpackets.FrameExListPartyMatchingWaitingRoom(req.Mode, rows))
}

// withdrawParty takes live out of its party at its own request; a member
// of a room then leaves the room too. A player in no party leaves nothing
// and hears nothing: the menu action waits on no answer.
func (l *GameClientLink) withdrawParty(live *livePlayer) {
	notices := l.parties.Leave(live, party.Left)
	if notices == nil {
		return
	}
	l.applyPartyNotices(notices)
	l.applyRoomNotices(l.rooms.LeaveWithParty(live))
}

// leavePartyMatch takes a player leaving the world off the waiting list
// and out of its room.
func (l *GameClientLink) leavePartyMatch(live *livePlayer) {
	if l.rooms == nil {
		return
	}
	l.applyRoomNotices(l.rooms.Leave(live))
}

// chatPartyMatchRoom is heard by every member of live's room, live
// included. Outside a room the line is dropped without an answer.
func (l *GameClientLink) chatPartyMatchRoom(_ *Client, live *livePlayer, line chat.Line) {
	room, ok := l.rooms.RoomOf(live.ObjectID())
	if !ok {
		return
	}
	l.broadcastToMembers(room.Members, frameCreatureSay(live, line))
}

// applyRoomNotices sends the packets of a party-matching change, in order.
func (l *GameClientLink) applyRoomNotices(notices []partymatch.Notice) {
	for _, n := range notices {
		switch n := n.(type) {
		case partymatch.Detail[*livePlayer]:
			n.To.SendFrame(serverpackets.FramePartyMatchDetail(roomTerms(n.Room)))
		case partymatch.MemberList[*livePlayer]:
			n.To.SendFrame(serverpackets.FrameExPartyRoomMember(int32(n.Mode), l.roomMemberRows(n.Room)))
		case partymatch.MemberChange[*livePlayer]:
			row := l.roomMemberRow(n.Member, n.Leader)
			l.broadcastToMembers(n.To, func() wire.Frame {
				return serverpackets.FrameExManagePartyRoomMember(int32(n.Mode), row)
			})
		case partymatch.Msg[*livePlayer]:
			id := roomMessages[n.ID]
			l.broadcastToMembers(n.To, func() wire.Frame {
				if n.Name != "" {
					return serverpackets.FrameSystemMessageString(id, n.Name)
				}
				return serverpackets.FrameSystemMessage(id)
			})
		case partymatch.Close[*livePlayer]:
			n.To.SendFrame(serverpackets.FrameExClosePartyRoom())
		case partymatch.InfoRefresh[*livePlayer]:
			l.broadcastCharacterInfo(n.Member)
		case partymatch.RoomList[*livePlayer]:
			n.To.SendFrame(serverpackets.FramePartyMatchList(roomRows(n.Rooms)))
		}
	}
}

// sameParty reports whether a and b share a party.
func (l *GameClientLink) sameParty(a, b *livePlayer) bool {
	return l.parties.SameParty(a.ObjectID(), b.ObjectID())
}

// roomLocation is the region number rooms and their member rows show for
// live's position: its restart region's board number, or
// partymatch.NoLocation outside every region.
func (l *GameClientLink) roomLocation(live *livePlayer) int32 {
	if point, ok := l.restarts.PointAt(live.CurrentLocation()); ok {
		return int32(point.BBS)
	}
	return partymatch.NoLocation
}

// nearTo reports whether a room leader stands in live's world region.
func (l *GameClientLink) nearTo(live *livePlayer) func(*livePlayer) bool {
	at := live.CurrentLocation()
	key := world.RegionKey(at.X, at.Y)
	return func(leader *livePlayer) bool {
		pos := leader.CurrentLocation()
		return world.RegionKey(pos.X, pos.Y) == key
	}
}

// roomMemberRow is m's row on a member list of the room leader leads.
func (l *GameClientLink) roomMemberRow(m, leader *livePlayer) serverpackets.PartyRoomMember {
	var status int32
	switch {
	case m.ObjectID() == leader.ObjectID():
		status = 1
	case l.sameParty(m, leader):
		status = 2
	}
	return serverpackets.PartyRoomMember{
		ObjectID: m.ObjectID(),
		Name:     m.Name,
		ClassID:  int32(m.ClassID()),
		Level:    int32(m.Level()),
		Location: l.roomLocation(m),
		Status:   status,
	}
}

func (l *GameClientLink) roomMemberRows(room partymatch.Room[*livePlayer]) []serverpackets.PartyRoomMember {
	rows := make([]serverpackets.PartyRoomMember, len(room.Members))
	for i, m := range room.Members {
		rows[i] = l.roomMemberRow(m, room.Leader())
	}
	return rows
}

func roomTerms(room partymatch.Room[*livePlayer]) serverpackets.PartyRoomTerms {
	return serverpackets.PartyRoomTerms{
		ID:         room.ID,
		MaxMembers: room.Settings.MaxMembers,
		MinLevel:   room.Settings.MinLevel,
		MaxLevel:   room.Settings.MaxLevel,
		Loot:       room.Settings.Loot,
		Location:   room.Location,
		Title:      room.Settings.Title,
	}
}

func roomRows(rooms []partymatch.Room[*livePlayer]) []serverpackets.PartyRoom {
	rows := make([]serverpackets.PartyRoom, len(rooms))
	for i, r := range rooms {
		rows[i] = serverpackets.PartyRoom{
			ID:         r.ID,
			Title:      r.Settings.Title,
			Location:   r.Location,
			MinLevel:   r.Settings.MinLevel,
			MaxLevel:   r.Settings.MaxLevel,
			Members:    int32(len(r.Members)),
			MaxMembers: r.Settings.MaxMembers,
			LeaderName: r.Leader().Name,
		}
	}
	return rows
}

var roomMessages = map[partymatch.MessageID]int{
	partymatch.MsgRoomCreated:       serverpackets.SystemMessagePartyRoomCreated,
	partymatch.MsgRoomRevised:       serverpackets.SystemMessagePartyRoomRevised,
	partymatch.MsgRoomDisbanded:     serverpackets.SystemMessagePartyRoomDisbanded,
	partymatch.MsgRoomLeaderChanged: serverpackets.SystemMessagePartyRoomLeaderChanged,
	partymatch.MsgEnteredRoom:       serverpackets.SystemMessageS1EnteredPartyRoom,
	partymatch.MsgLeftRoom:          serverpackets.SystemMessageS1LeftPartyRoom,
}
