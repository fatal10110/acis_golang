package party

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

const sayPartyMatchRoom int32 = 14

func encodeSay(typ int32, text string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(typ)
	return w.Bytes()
}

// TestPartyRoomOpenBrowseEnterLeave pins the room's life from the
// reference's RequestListPartyWaiting, RequestManagePartyRoom,
// RequestJoinPartyRoom and RequestWithdrawPartyRoom:
//
//   - Opening the window lists the rooms (none yet) and puts the player on
//     the waiting list.
//   - Opening a room shows its leader the terms (PartyMatchDetail), the
//     member list (mode 1, the leader's row marked 1), PARTY_ROOM_CREATED
//     and its own refreshed view; onlookers get its CharInfo.
//   - The list filters by location (-1 any, -2 the leader's world region,
//     otherwise the room's region number) and, in level mode 0, by the
//     room's level range; a player outside the range is refused entry.
//   - Entering (room 0: the first listed) shows the entrant the terms and
//     the members before it (mode 0), and every member its row (mode 0)
//     and S1_ENTERED_PARTY_ROOM.
//   - A room line reaches the room's members only.
//   - Withdrawing tells the others S1_LEFT_PARTY_ROOM and drops the row
//     (mode 2); the leaver's window closes, then PARTY_ROOM_EXITED.
func TestPartyRoomOpenBrowseEnterLeave(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 50}}, spawnRegions())
	leader, member, third := g.players[0], g.players[1], g.players[2]

	leader.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	if rooms := readRoomList(t, expectRoom(t, leader, "List")[0]); len(rooms) != 0 {
		t.Fatalf("rooms before any opened = %+v", rooms)
	}

	leader.c.Send(encodeManagePartyRoom(0, 12, 10, 40, 2, "Go"))
	frames := expectRoom(t, leader, "Detail", "Members1", sm(serverpackets.SystemMessagePartyRoomCreated), "UserInfo")
	if d := readDetail(t, frames[0]); d != (roomTerms{id: 1, maxMembers: 12, minLevel: 10, maxLevel: 40, loot: 2, location: spawnBBS, title: "Go"}) {
		t.Fatalf("PartyMatchDetail = %+v", d)
	}
	if mode, rows := readRoomMembers(t, frames[1]); mode != 1 || !slices.Equal(rows, []roomRow{{id: leader.id, name: "Leader", level: 20, location: spawnBBS, status: 1}}) {
		t.Fatalf("ExPartyRoomMember = mode %d %+v", mode, rows)
	}
	expectRoom(t, member, "CharInfo")
	expectRoom(t, third, "CharInfo")

	member.c.Send(encodeListPartyWaiting(anyLocation, 0))
	want := []listedRoom{{id: 1, title: "Go", location: spawnBBS, minLevel: 10, maxLevel: 40, members: 1, maxMember: 12, leader: "Leader"}}
	if rooms := readRoomList(t, expectRoom(t, member, "List")[0]); !slices.Equal(rooms, want) {
		t.Fatalf("Member's rooms = %+v, want %+v", rooms, want)
	}
	for _, tc := range []struct {
		location, levelMode int32
		listed              bool
	}{
		{anyLocation, 0, false}, // level 50 is past the room's range
		{anyLocation, allLevels, true},
		{spawnBBS, allLevels, true},
		{3, allLevels, false},
		{-2, allLevels, true}, // same world region as the leader
	} {
		third.c.Send(encodeListPartyWaiting(tc.location, tc.levelMode))
		if rooms := readRoomList(t, expectRoom(t, third, "List")[0]); (len(rooms) == 1) != tc.listed {
			t.Fatalf("location %d level mode %d listed %+v, want listed %v", tc.location, tc.levelMode, rooms, tc.listed)
		}
	}
	third.c.Send(encodeJoinPartyRoom(1, anyLocation, allLevels))
	expectRoom(t, third, sm(serverpackets.SystemMessageCantEnterPartyRoom))

	member.c.Send(encodeJoinPartyRoom(0, anyLocation, 0))
	frames = expectRoom(t, member, "Detail", "Members0", "UserInfo")
	if mode, rows := readRoomMembers(t, frames[1]); mode != 0 || len(rows) != 1 || rows[0].id != leader.id {
		t.Fatalf("entrant's ExPartyRoomMember = mode %d %+v, want the leader alone", mode, rows)
	}
	frames = expectRoom(t, leader, "Manage0:Member", sm(serverpackets.SystemMessageS1EnteredPartyRoom), "CharInfo")
	if mode, row := readManageRow(t, frames[0]); mode != 0 || row != (roomRow{id: member.id, name: "Member", level: 30, location: spawnBBS}) {
		t.Fatalf("ExManagePartyRoomMember = mode %d %+v", mode, row)
	}
	assertSystemMessageText(t, frames[1], serverpackets.SystemMessageS1EnteredPartyRoom, "Member")
	expectRoom(t, third, "CharInfo")

	member.c.Send(encodeSay(sayPartyMatchRoom, "hi"))
	expectRoom(t, leader, "Say")
	expectRoom(t, member, "Say")
	assertSilent(t, third.c, "player outside the room")
	third.c.Send(encodeSay(sayPartyMatchRoom, "hi"))
	for _, p := range g.players {
		assertSilent(t, p.c, "room line from a player in no room")
	}

	// Leader's waiting list: Third is waiting; Member left the list when
	// it entered, and the asker is never listed.
	leader.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestListPartyMatchingWaitingRoom, 1, 1, 80, 0))
	r := wire.NewReader(expectRoom(t, leader, "Waiting")[0][3:])
	if mode, n, name, class, level := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadInt32(), r.ReadInt32(); mode != 0 || n != 1 || name != "Third" || class != 0 || level != 50 || r.Remaining() != 0 {
		t.Fatalf("waiting list = mode %d, %d players, first %q class %d level %d", mode, n, name, class, level)
	}

	member.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestWithdrawPartyRoom, 1, 0))
	frames = expectRoom(t, leader, sm(serverpackets.SystemMessageS1LeftPartyRoom), "Manage2:Member", "CharInfo")
	assertSystemMessageText(t, frames[0], serverpackets.SystemMessageS1LeftPartyRoom, "Member")
	expectRoom(t, member, "Close", "UserInfo", sm(serverpackets.SystemMessagePartyRoomExited))
	expectRoom(t, third, "CharInfo")

	// Leaving the waiting list answers nothing and takes Third off it.
	third.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestExitPartyMatchingWaitingRoom))
	assertSilent(t, third.c, "leaving the waiting list")
	leader.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestListPartyMatchingWaitingRoom, 1, 1, 80, 0))
	if n := wire.NewReader(expectRoom(t, leader, "Waiting")[0][7:]).ReadInt32(); n != 0 {
		t.Fatalf("waiting list holds %d players, want none", n)
	}
}

// TestPartyRoomLeaderLeavingHandsOver pins a leader withdrawing from a room
// with company: the room passes to the next member first (each member is
// shown both rows again, mode 1, and PARTY_ROOM_LEADER_CHANGED), then the
// old leader leaves as any member does.
func TestPartyRoomLeaderLeavingHandsOver(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}}, spawnRegions())
	leader, member, third := g.players[0], g.players[1], g.players[2]
	id := g.openRoom(t, leader, "Go")
	g.enterRoom(t, member, id)
	g.enterRoom(t, third, id)

	leader.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestWithdrawPartyRoom, id, 0))
	handover := []string{"Manage1:Member", "Manage1:Leader", sm(serverpackets.SystemMessagePartyRoomLeaderChanged)}
	expectRoom(t, leader, append(handover, "Close", "UserInfo", sm(serverpackets.SystemMessagePartyRoomExited))...)
	for _, p := range []player{member, third} {
		frames := expectRoom(t, p, append(handover, sm(serverpackets.SystemMessageS1LeftPartyRoom), "Manage2:Leader", "CharInfo")...)
		if _, row := readManageRow(t, frames[0]); row.status != 1 {
			t.Fatalf("%s: new leader's row status %d, want 1", p.name, row.status)
		}
	}

	member.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	frames := expectRoom(t, member, "Detail", "Members2", "UserInfo")
	if _, rows := readRoomMembers(t, frames[1]); len(rows) != 2 || rows[0].name != "Member" || rows[0].status != 1 || rows[1].name != "Third" || rows[1].status != 0 {
		t.Fatalf("room members after the handover = %+v", rows)
	}
}

// TestPartyRoomOustAndDismiss pins RequestOustFromPartyRoom and
// RequestDismissPartyRoom. An ousted member leaves as on withdrawal, is
// put back on the waiting list and shown the rooms of location 1 at every
// level, then OUSTED_FROM_PARTY_ROOM. Disbanding closes every member's
// window with PARTY_ROOM_DISBANDED. Neither works for anyone but the
// leader (a deliberate refusal: the reference disbands any room by id).
func TestPartyRoomOustAndDismiss(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}}, spawnRegions())
	leader, member := g.players[0], g.players[1]
	id := g.openRoom(t, leader, "Go")
	g.enterRoom(t, member, id)

	member.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestOustFromPartyRoom, leader.id))
	assertSilent(t, member.c, "oust by a member")
	assertSilent(t, leader.c, "oust by a member")

	leader.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestOustFromPartyRoom, member.id))
	expectRoom(t, leader, sm(serverpackets.SystemMessageS1LeftPartyRoom), "Manage2:Member", "CharInfo")
	frames := expectRoom(t, member, "Close", "UserInfo", "List", sm(serverpackets.SystemMessageOustedFromPartyRoom))
	if rooms := readRoomList(t, frames[2]); len(rooms) != 0 {
		t.Fatalf("ousted player's list = %+v, want no room at location 1", rooms)
	}

	// Back on the waiting list: it may enter again at once.
	member.c.Send(encodeJoinPartyRoom(id, anyLocation, allLevels))
	expectRoom(t, member, "Detail", "Members0", "UserInfo")
	g.quiet(t)

	member.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestDismissPartyRoom, id, 0))
	assertSilent(t, member.c, "dismiss by a member")
	assertSilent(t, leader.c, "dismiss by a member")

	leader.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestDismissPartyRoom, id, 0))
	disbanded := sm(serverpackets.SystemMessagePartyRoomDisbanded)
	expectRoom(t, leader, "Close", disbanded, "UserInfo", "CharInfo")
	expectRoom(t, member, "CharInfo", "Close", disbanded, "UserInfo")

	// The room is gone: no one is in it to withdraw, and its id lists
	// nothing.
	member.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestWithdrawPartyRoom, id, 0))
	assertSilent(t, member.c, "withdraw from a disbanded room")
}

// TestPartyRoomInvitation pins RequestAskJoinPartyRoom and
// AnswerJoinPartyRoom: an unknown name is TARGET_IS_NOT_FOUND_IN_THE_GAME,
// a player holding a request is busy, the invited player gets
// ExAskJoinPartyRoom naming the inviter; a refusal tells the inviter
// PARTY_MATCHING_REQUEST_NO_RESPONSE, an acceptance enters the inviter's
// room as RequestJoinPartyRoom does, and an answer with nothing pending is
// TARGET_IS_NOT_FOUND_IN_THE_GAME.
func TestPartyRoomInvitation(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}}, spawnRegions())
	leader, member, third := g.players[0], g.players[1], g.players[2]
	id := g.openRoom(t, leader, "Go")

	leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestAskJoinPartyRoom, "Nobody"))
	expectRoom(t, leader, sm(serverpackets.SystemMessageTargetNotFound))

	leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestAskJoinPartyRoom, "third"))
	expectRoom(t, third, "Ask:Leader")
	member.c.Send(encodeExtendedName(clientpackets.OpcodeRequestAskJoinPartyRoom, "Third"))
	frames := expectRoom(t, member, sm(serverpackets.SystemMessageS1IsBusyTryLater))
	assertSystemMessageText(t, frames[0], serverpackets.SystemMessageS1IsBusyTryLater, "Third")
	third.c.Send(encodeExtendedInts(clientpackets.OpcodeAnswerJoinPartyRoom, 0))
	expectRoom(t, leader, sm(serverpackets.SystemMessagePartyMatchingRequestNoResponse))
	assertSilent(t, third.c, "refusing")

	third.c.Send(encodeExtendedInts(clientpackets.OpcodeAnswerJoinPartyRoom, 1))
	expectRoom(t, third, sm(serverpackets.SystemMessageTargetNotFound))

	member.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	g.quiet(t)
	leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestAskJoinPartyRoom, "Member"))
	expectRoom(t, member, "Ask:Leader")
	member.c.Send(encodeExtendedInts(clientpackets.OpcodeAnswerJoinPartyRoom, 1))
	expectRoom(t, member, "Detail", "Members0", "UserInfo")
	expectRoom(t, leader, "Manage0:Member", sm(serverpackets.SystemMessageS1EnteredPartyRoom), "CharInfo")

	leader.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	frames = expectRoom(t, leader, "Detail", "Members2", "UserInfo")
	if d := readDetail(t, frames[0]); d.id != id {
		t.Fatalf("room id %d, want %d", d.id, id)
	}
}

// TestPartyRoomFollowsParty pins the room's ties to the party:
//
//   - A party member that does not lead may not browse rooms
//     (CANT_VIEW_PARTY_ROOMS, ActionFailed).
//   - Opening a room takes the leader's party in with it; their rows are
//     marked 2.
//   - A player joining the party of a room's member is shown again (mode
//     1) on every member's list, entering the room if it was in none.
//   - A new party leader leads the room too.
//   - Leaving the party shows the leaver the room once more, then takes it
//     out of the room.
//   - A member of the leader's party can neither be ousted
//     (CANNOT_DISMISS_PARTY_MEMBER) nor withdraw; nor can the leader while
//     in a party.
func TestPartyRoomFollowsParty(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}, {"Fourth", 50}}, spawnRegions())
	leader, member, third, fourth := g.players[0], g.players[1], g.players[2], g.players[3]
	g.invite(t, 0, 1, 0)

	member.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	expectRoom(t, member, sm(serverpackets.SystemMessageCantViewPartyRooms), "ActionFailed")

	leader.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	leader.c.Send(encodeManagePartyRoom(0, 12, 1, 80, 0, "Go"))
	frames := expectRoom(t, leader, "List", "CharInfo", "Detail", "Members1", sm(serverpackets.SystemMessagePartyRoomCreated), "UserInfo")
	if _, rows := readRoomMembers(t, frames[3]); len(rows) != 2 || rows[0].id != leader.id || rows[0].status != 1 || rows[1].id != member.id || rows[1].status != 2 {
		t.Fatalf("room members = %+v, want the leader (1) and its party member (2)", rows)
	}
	expectRoom(t, member, "UserInfo", "CharInfo")
	g.quiet(t)

	// Third enters the room, then joins the party: already in the room, it
	// is shown again to every member, now marked as the leader's party.
	g.enterRoom(t, third, 1)
	g.invite(t, 0, 2, 0)
	// Fourth, in no room, is pulled in by joining the party.
	leader.c.Send(encodeJoinParty("Fourth", 0))
	g.quiet(t)
	fourth.c.Send(encodeAnswerJoinParty(1))
	frames = drainFrames(t, fourth.c)
	got := roomTokens(t, frames)
	if !slices.Contains(got, "Manage1:Fourth") {
		t.Fatalf("Fourth's frames %q show no room row", got)
	}
	g.quiet(t)

	leader.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	frames = expectRoom(t, leader, "Detail", "Members2", "UserInfo")
	_, rows := readRoomMembers(t, frames[1])
	if len(rows) != 4 || rows[2].name != "Third" || rows[2].status != 2 || rows[3].name != "Fourth" || rows[3].status != 2 {
		t.Fatalf("room members = %+v", rows)
	}
	g.quiet(t)

	// A member of the leader's party stays put.
	leader.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestOustFromPartyRoom, third.id))
	expectRoom(t, leader, sm(serverpackets.SystemMessageCannotDismissPartyMember))
	third.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestWithdrawPartyRoom, 1, 0))
	assertSilent(t, third.c, "withdraw by a party member of the leader")
	leader.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestWithdrawPartyRoom, 1, 0))
	assertSilent(t, leader.c, "withdraw by a leader in a party")

	// The party passes to Member, and so does the room.
	leader.c.Send(encodeExtendedName(clientpackets.OpcodeRequestChangePartyLeader, "Member"))
	frames = drainFrames(t, third.c)
	if got := roomTokens(t, frames); !slices.Equal(got[len(got)-3:], []string{"Manage1:Member", "Manage1:Leader", sm(serverpackets.SystemMessagePartyRoomLeaderChanged)}) {
		t.Fatalf("Third's frames on the leader change = %q", got)
	}
	g.quiet(t)

	// Fourth leaves the party: it is shown the room once more, then leaves
	// it.
	fourth.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	got = roomTokens(t, drainFrames(t, fourth.c))
	if i := slices.Index(got, "Detail"); i < 0 || !slices.Equal(got[i:], []string{"Detail", "Members0", "Close", "UserInfo"}) {
		t.Fatalf("Fourth's frames on leaving the party = %q", got)
	}
	got = roomTokens(t, drainFrames(t, third.c))
	if i := slices.Index(got, sm(serverpackets.SystemMessageS1LeftPartyRoom)); i < 0 || !slices.Equal(got[i:i+2], []string{sm(serverpackets.SystemMessageS1LeftPartyRoom), "Manage2:Fourth"}) {
		t.Fatalf("Third's frames on Fourth leaving = %q", got)
	}
}

// TestPartyRoomLeavesWithPlayer pins a player leaving the world: out of
// its room (the others told it left), and a lone leader's room is gone.
func TestPartyRoomLeavesWithPlayer(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}}, spawnRegions())
	leader, member, third := g.players[0], g.players[1], g.players[2]
	id := g.openRoom(t, leader, "Go")
	g.enterRoom(t, member, id)

	member.c.Send(encodeSingle(clientpackets.OpcodeLogout))
	g.srv.AdvanceUntil(t, "member leaves the world", func() bool {
		_, ok := g.srv.State.Player(member.id)
		return !ok
	})
	got := roomTokens(t, drainFrames(t, leader.c))
	if len(got) < 3 || !slices.Equal(got[:3], []string{sm(serverpackets.SystemMessageS1LeftPartyRoom), "Manage2:Member", "CharInfo"}) {
		t.Fatalf("leader's frames on the member leaving = %q", got)
	}
	drainFrames(t, third.c)

	leader.c.Send(encodeSingle(clientpackets.OpcodeLogout))
	g.srv.AdvanceUntil(t, "leader leaves the world", func() bool {
		_, ok := g.srv.State.Player(leader.id)
		return !ok
	})
	drainFrames(t, third.c)
	third.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	if rooms := readRoomList(t, expectRoom(t, third, "List")[0]); len(rooms) != 0 {
		t.Fatalf("rooms after their leader left = %+v", rooms)
	}
	third.c.Send(encodeJoinPartyRoom(id, anyLocation, allLevels))
	expectRoom(t, third, sm(serverpackets.SystemMessageCantEnterPartyRoom))
}

// TestPartyRoomRevise pins RequestManagePartyRoom on an existing room: its
// leader's new terms (and its current region) reach every member as
// PartyMatchDetail, the member list (mode 2) and PARTY_ROOM_REVISED. A
// member revising, or a player off the waiting list opening a room,
// changes nothing and hears nothing.
func TestPartyRoomRevise(t *testing.T) {
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}}, spawnRegions())
	leader, member, third := g.players[0], g.players[1], g.players[2]
	id := g.openRoom(t, leader, "Go")
	g.enterRoom(t, member, id)

	member.c.Send(encodeManagePartyRoom(id, 2, 1, 2, 0, "Mine"))
	assertSilent(t, member.c, "revision by a member")
	assertSilent(t, leader.c, "revision by a member")
	third.c.Send(encodeManagePartyRoom(0, 12, 1, 80, 0, "Off the list"))
	assertSilent(t, third.c, "room opened off the waiting list")

	leader.c.Send(encodeManagePartyRoom(id, 5, 15, 35, 1, "Now"))
	want := roomTerms{id: id, maxMembers: 5, minLevel: 15, maxLevel: 35, loot: 1, location: spawnBBS, title: "Now"}
	for _, p := range []player{leader, member} {
		frames := expectRoom(t, p, "Detail", "Members2", sm(serverpackets.SystemMessagePartyRoomRevised))
		if d := readDetail(t, frames[0]); d != want {
			t.Fatalf("%s: PartyMatchDetail = %+v, want %+v", p.name, d, want)
		}
	}
	third.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	rooms := readRoomList(t, expectRoom(t, third, "List")[0])
	if len(rooms) != 1 || rooms[0].title != "Now" || rooms[0].maxMember != 5 || rooms[0].members != 2 {
		t.Fatalf("listed rooms = %+v", rooms)
	}
}
