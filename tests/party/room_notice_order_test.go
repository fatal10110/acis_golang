package party

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRoomNoticesFollowDecisionOrder pins #5489 on the pool executor:
// Joiner enters the room while Stayer, on its own queue, withdraws from
// it. Whichever change is decided first, every member sees the two in that
// order: a Joiner whose member list shows Stayer has Stayer's row deleted
// only after the list arrived, and a Stayer who saw Joiner's row added saw
// it before its room window closed. Delivered in the opposite order,
// either keeps a ghost row.
func TestRoomNoticesFollowDecisionOrder(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Leader", 20}, {"Stayer", 30}, {"Joiner", 40}}, spawnRegions(), gameservertest.WithRealPool())
	leader, stayer, joiner := g.players[0], g.players[1], g.players[2]
	leader.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	leader.c.Send(encodeManagePartyRoom(0, 12, 1, 80, 0, "Go"))
	g.quiet(t)
	g.enterRoom(t, stayer, 1)

	for round := range 25 {
		joiner.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
		g.quiet(t)

		joiner.c.Send(encodeJoinPartyRoom(1, anyLocation, allLevels))
		stayer.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestWithdrawPartyRoom, 1, 0))

		// Joiner: Stayer's row, if listed, is deleted after the list.
		frames := roomFrames(t, joiner)
		list := slices.IndexFunc(frames, isRoomEx(serverpackets.OpcodeExPartyRoomMember))
		if list < 0 {
			t.Fatalf("round %d: Joiner got no member list: %q", round, roomTokens(t, frames))
		}
		_, rows := readRoomMembers(t, frames[list])
		listsStayer := slices.ContainsFunc(rows, func(r roomRow) bool { return r.id == stayer.id })
		deleted := slices.IndexFunc(frames, isManage(t, 2, stayer.id))
		if deleted >= 0 && deleted < list {
			t.Fatalf("round %d: Joiner had Stayer's row deleted before its member list: %q", round, roomTokens(t, frames))
		}
		if listsStayer != (deleted >= 0) {
			t.Fatalf("round %d: Joiner's list shows Stayer %v, Stayer's row deleted %v: %q", round, listsStayer, deleted >= 0, roomTokens(t, frames))
		}

		// Stayer: Joiner's row, if added, is added before the room closed.
		frames = roomFrames(t, stayer)
		closed := slices.IndexFunc(frames, isRoomEx(serverpackets.OpcodeExClosePartyRoom))
		if closed < 0 {
			t.Fatalf("round %d: Stayer's room window never closed: %q", round, roomTokens(t, frames))
		}
		if added := slices.IndexFunc(frames, isManage(t, 0, joiner.id)); added > closed {
			t.Fatalf("round %d: Stayer saw Joiner added after its room closed: %q", round, roomTokens(t, frames))
		}

		// Back to Leader and Stayer in the room for the next round.
		g.quiet(t)
		joiner.c.Send(encodeExtendedInts(clientpackets.OpcodeRequestWithdrawPartyRoom, 1, 0))
		g.quiet(t)
		g.enterRoom(t, stayer, 1)
	}
}

// TestRoomNoticesFollowPartyNotices pins #5489 on the pool executor:
// Member leaves a party whose leader leads a room while Joiner, on its own
// queue, accepts an invitation into the same party. However the two party
// changes race, a room change made with a party change reaches every
// client after that party change's notices: Member's party row is deleted
// before it is told out of the room, Member's party window is cleared
// before its room window closes, and Joiner's party window comes before
// its room rows.
func TestRoomNoticesFollowPartyNotices(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Leader", 20}, {"Member", 30}, {"Third", 40}, {"Joiner", 50}}, spawnRegions(), gameservertest.WithRealPool())
	leader, member, third, joiner := g.players[0], g.players[1], g.players[2], g.players[3]
	g.invite(t, 0, 1, 0)
	g.invite(t, 0, 2, 0)
	leader.c.Send(encodeListPartyWaiting(anyLocation, allLevels))
	leader.c.Send(encodeManagePartyRoom(0, 12, 1, 80, 0, "Go"))
	g.quiet(t)

	for round := range 25 {
		leader.c.Send(encodeJoinParty(joiner.name, 0))
		assertSystemMessageText(t, skipPositions(leader.c), serverpackets.SystemMessageYouInvitedS1ToParty, joiner.name)
		assertFrameOpcode(t, joiner.c.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")

		joiner.c.Send(encodeAnswerJoinParty(1))
		member.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))

		for _, p := range []player{leader, third} {
			frames := roomFrames(t, p)
			partyRow := slices.IndexFunc(frames, func(f []byte) bool {
				if f[0] != serverpackets.OpcodePartySmallWindowDelete {
					return false
				}
				id, _ := readWindowDelete(t, f)
				return id == member.id
			})
			roomRow := slices.IndexFunc(frames, isManage(t, 2, member.id))
			if partyRow < 0 || roomRow < 0 || roomRow < partyRow {
				t.Fatalf("round %d: %s saw Member's party row deleted at %d, its room row at %d: %q", round, p.name, partyRow, roomRow, roomTokens(t, frames))
			}
		}

		frames := roomFrames(t, member)
		cleared := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodePartySmallWindowDeleteAll })
		closed := slices.IndexFunc(frames, isRoomEx(serverpackets.OpcodeExClosePartyRoom))
		if cleared < 0 || closed < 0 || closed < cleared {
			t.Fatalf("round %d: Member's party window cleared at %d, room closed at %d: %q", round, cleared, closed, roomTokens(t, frames))
		}

		frames = roomFrames(t, joiner)
		window := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodePartySmallWindowAll })
		room := slices.IndexFunc(frames, isManage(t, 1, joiner.id))
		if window < 0 || room < 0 || room < window {
			t.Fatalf("round %d: Joiner's party window at %d, its room row at %d: %q", round, window, room, roomTokens(t, frames))
		}

		// Back to Leader, Member and Third for the next round.
		g.quiet(t)
		joiner.c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
		g.quiet(t)
		g.invite(t, 0, 1, 0)
	}
}

// roomFrames drains p's frames, party positions left out.
func roomFrames(t *testing.T, p player) [][]byte {
	t.Helper()
	var kept [][]byte
	for _, f := range drainFrames(t, p.c) {
		if f[0] != serverpackets.OpcodePartyMemberPosition {
			kept = append(kept, f)
		}
	}
	return kept
}

// isRoomEx matches an extended frame of the sub-opcode sub.
func isRoomEx(sub uint16) func([]byte) bool {
	return func(f []byte) bool {
		return f[0] == serverpackets.OpcodeExtended && wire.NewReader(f[1:]).ReadUint16() == sub
	}
}

// isManage matches an ExManagePartyRoomMember of mode for the player id.
func isManage(t *testing.T, mode, id int32) func([]byte) bool {
	return func(f []byte) bool {
		if !isRoomEx(serverpackets.OpcodeExManagePartyRoomMember)(f) {
			return false
		}
		m, row := readManageRow(t, f)
		return m == mode && row.id == id
	}
}
