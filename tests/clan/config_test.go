package clan

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestInvitationLapses lets a clan invitation run out on the clan clock:
// the invited player's late acceptance is ignored and leaves it clanless,
// and the leader may invite again at once.
func TestInvitationLapses(t *testing.T) {
	var now atomic.Int64
	now.Store(time.Now().UnixMilli())
	w := bootClanWorld(t, 10, 0, 0, gameservertest.WithClanClock(func() time.Time { return time.UnixMilli(now.Load()) }))
	w.found(t, "Knights")

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinPledge); !ok {
		t.Fatal("recruit was not asked to join")
	}
	now.Add(clan.InviteTimeout.Milliseconds())

	w.member.Send(encodeRequestAnswerJoinPledge(1))
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("acceptance after the invitation lapsed = %x, want nothing", opcodes(frames))
	}
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("leader's view of a lapsed acceptance = %x, want nothing", opcodes(frames))
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COALESCE(clanid,0) FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
		t.Fatalf("late acceptor's clanid = %d, want 0", got)
	}

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinPledge); !ok {
		t.Fatal("leader could not invite again after the lapse")
	}
}

// TestWithdrawPenaltyFollowsConfig stores the configured join penalty, in
// days, on a member that withdraws.
func TestWithdrawPenaltyFollowsConfig(t *testing.T) {
	const joinDays = 3
	w := bootClanWorld(t, 10, 0, 0, gameservertest.WithClanConfig(clan.Config{JoinDays: joinDays, CreateDays: 10}))
	w.found(t, "Knights")
	w.recruit(t)

	before := time.Now().UnixMilli()
	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	if ids := messages(t, drainFrames(t, w.member)); !slices.Contains(ids, serverpackets.SystemMessageYouHaveWithdrawnFromClan) {
		t.Fatalf("withdrawal messages = %v", ids)
	}
	after := time.Now().UnixMilli()
	w.srv.FlushPersistence(t)
	const window = joinDays * 24 * 3600 * 1000
	if got := queryInt(t, w, `SELECT clan_join_expiry_time FROM characters WHERE obj_Id = ?`, w.memberID); got < before+window || got > after+window {
		t.Fatalf("join penalty ends %d, want %d days after the withdrawal (%d..%d)", got, joinDays, before+window, after+window)
	}
}
