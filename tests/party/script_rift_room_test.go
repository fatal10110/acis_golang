package party

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The room cell of the script-facing API freeze (#3802), as the Dimensional
// Rift quest keeps its rooms: a room follows the party sent into it by
// identity through joins, leaves and a leader change, compares its members
// with the object ids taken when it was set, and reads the party as gone
// once it disbands.

// runRoom runs fn as the Director script for seat i's player.
func (g *group) runRoom(t *testing.T, i int, fn func(sc *script.Script, p *script.Player)) {
	t.Helper()
	if !g.srv.RunScript(t, g.players[i].id, "Director", fn) {
		t.Fatal("the script panicked")
	}
}

func objectIDs(players []*script.Player) []int32 {
	out := make([]int32, len(players))
	for i, p := range players {
		out[i] = p.ObjectID()
	}
	return out
}

// TestScriptRoomFollowsItsParty: a room set with Leader's party holds that
// party for its two members only, takes their ids, and stamps the game tick
// on each lock change. A third member joining changes the party; the same
// member leaving again restores it. A leader change keeps the party and
// moves its leader, which the player handle reads too. A room given another
// room's party holds it as well; clearing a room drops the party and
// unlocks it. Once the party disbands the room reads it as changed, with no
// leader and no members, while the taken ids stay.
func TestScriptRoomFollowsItsParty(t *testing.T) {
	t.Parallel()
	g := bootScriptChannel(t, false)
	leaderID, memberID, lonerID := g.players[scLeader].id, g.players[scMember].id, g.players[scLoner].id
	var room, copied *script.RoomCell

	g.runRoom(t, scLoner, func(sc *script.Script, p *script.Player) {
		room, copied = sc.NewRoom(3), sc.NewRoom(8)
		if room.Index() != 3 || room.Locked() || !room.PartyChanged() {
			t.Errorf("new room: index %d locked %v changed %v, want 3 false true", room.Index(), room.Locked(), room.PartyChanged())
		}
		if !room.HoldsPartyOf(p) {
			t.Error("an empty room does not match a player in no party")
		}
	})

	g.runRoom(t, scMember, func(sc *script.Script, p *script.Player) {
		before := sc.CurrentTick()
		if !room.TryLock() {
			t.Fatal("TryLock on an unlocked room refused")
		}
		if after := sc.CurrentTick(); room.Time() < before || room.Time() > after {
			t.Errorf("lock stamped tick %d, want within [%d, %d]", room.Time(), before, after)
		}
		room.SetParty(p)
		if got := room.MemberIDs(); !slices.Equal(got, []int32{leaderID, memberID}) {
			t.Errorf("MemberIDs() = %v, want [%d %d]", got, leaderID, memberID)
		}
		if room.PartyChanged() {
			t.Error("the party just set reads changed")
		}
		if leader := room.PartyLeader(); leader == nil || leader.ObjectID() != leaderID {
			t.Errorf("PartyLeader() = %v, want Leader", leader)
		}
		if got := objectIDs(room.PartyMembers()); !slices.Equal(got, []int32{leaderID, memberID}) {
			t.Errorf("PartyMembers() = %v, want [%d %d]", got, leaderID, memberID)
		}
	})
	for i, want := range map[int]bool{scLeader: true, scMember: true, scOther: false, scFourth: false, scLoner: false} {
		g.runRoom(t, i, func(_ *script.Script, p *script.Player) {
			if got := room.HoldsPartyOf(p); got != want {
				t.Errorf("%s: HoldsPartyOf = %v, want %v", g.players[i].name, got, want)
			}
		})
	}

	g.invite(t, scLeader, scLoner, 0)
	g.runRoom(t, scLeader, func(_ *script.Script, _ *script.Player) {
		if !room.PartyChanged() {
			t.Error("a party that gained a member reads unchanged")
		}
		if got := objectIDs(room.PartyMembers()); !slices.Equal(got, []int32{leaderID, memberID, lonerID}) {
			t.Errorf("PartyMembers() after the join = %v", got)
		}
	})
	g.runRoom(t, scLoner, func(_ *script.Script, p *script.Player) {
		if !room.HoldsPartyOf(p) {
			t.Error("the room does not hold the party Loner joined")
		}
	})
	g.players[scLoner].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	g.quiet(t)
	g.runRoom(t, scLeader, func(_ *script.Script, _ *script.Player) {
		if room.PartyChanged() {
			t.Error("the party back to the members taken reads changed")
		}
	})

	g.players[scLeader].c.Send(encodeExtendedName(clientpackets.OpcodeRequestChangePartyLeader, g.players[scMember].name))
	g.quiet(t)
	g.runRoom(t, scLeader, func(sc *script.Script, p *script.Player) {
		if room.PartyChanged() {
			t.Error("a leader change reads as a changed party")
		}
		if leader := room.PartyLeader(); leader == nil || leader.ObjectID() != memberID {
			t.Errorf("room PartyLeader() after the change = %v, want Member", leader)
		}
		if leader := p.PartyLeader(); leader == nil || leader.ObjectID() != memberID {
			t.Errorf("player PartyLeader() after the change = %v, want Member", leader)
		}
		if p.IsPartyLeader() {
			t.Error("Leader still leads after handing the party over")
		}
		copied.SetPartyFrom(room)
		if !copied.HoldsPartyOf(p) || copied.PartyChanged() {
			t.Error("a room given another room's party does not hold it unchanged")
		}
		before := sc.CurrentTick()
		copied.SetLock(true)
		copied.Clear()
		if copied.Locked() || copied.Time() < before || !copied.PartyChanged() || copied.HoldsPartyOf(p) {
			t.Errorf("cleared room: locked %v time %d changed %v", copied.Locked(), copied.Time(), copied.PartyChanged())
		}
	})

	g.players[scLeader].c.Send(encodeSingle(clientpackets.OpcodeRequestWithdrawParty))
	g.quiet(t)
	g.runRoom(t, scLeader, func(_ *script.Script, p *script.Player) {
		if !room.PartyChanged() {
			t.Error("a disbanded party reads unchanged")
		}
		if leader := room.PartyLeader(); leader != nil {
			t.Errorf("PartyLeader() of a disbanded party = %v, want nil", leader)
		}
		if got := room.PartyMembers(); len(got) != 0 {
			t.Errorf("PartyMembers() of a disbanded party = %v, want none", objectIDs(got))
		}
		if room.HoldsPartyOf(p) {
			t.Error("a room holding a disbanded party matches a player in no party")
		}
		if got := room.MemberIDs(); !slices.Equal(got, []int32{leaderID, memberID}) {
			t.Errorf("MemberIDs() after the disband = %v", got)
		}
	})
}
