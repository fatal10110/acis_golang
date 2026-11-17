package clan

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestExpelDuringSelectionEntersClanless expels a titled member while its
// selection is under way, after its clan state was read and before it is in
// the world: while the selection restores it, and once CharSelected is
// answered, ahead of EnterWorld. Its clan lists it offline, so the
// expulsion stores the leave on its row only. The character entering the
// world takes that leave too, as a fresh load of its row would show it:
// clanless, untitled, and under the join penalty, so another clan's
// invitation is refused for the penalty rather than for a clan it no longer
// has.
func TestExpelDuringSelectionEntersClanless(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		duringRestore bool
	}{
		{"while restored", true},
		{"before EnterWorld", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			expelDuringSelection(t, tc.duringRestore)
		})
	}
}

func expelDuringSelection(t *testing.T, duringRestore bool) {
	var armed atomic.Bool
	holding, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	letGo := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(letGo) // a failure below must not leave the selection held
	// Founder leads Knights and Rival leads Rivals; Loner is clanless. The
	// real pool lets the leader's expulsion run while the selection's
	// handler is held.
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithRealPool(),
		seedRival(t, loner),
		seedAllianceClans(t),
		gameservertest.WithSelectionHold(func(int32) {
			if armed.CompareAndSwap(true, false) {
				close(holding)
				<-release
			}
		}),
	)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t), memberID: rivalLeaderID}
	w.member = srv.DialClient(t, "player2", 1)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	lone := w.bringIn(t, loner)
	w.leader.Send(encodeRequestJoinPledge(lonerID, 0))
	drainFrames(t, lone)
	lone.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, lone), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("Loner did not join")
	}
	w.leader.Send(encodeRequestGiveNickName("Loner", "Lord"))
	drainFrames(t, w.leader)
	drainFrames(t, lone)
	lone.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	readUntilFrame(t, lone, "the restart's CharSelectInfo", func(f []byte) bool { return f[0] == serverpackets.OpcodeCharSelectInfo })
	w.srv.FlushPersistence(t)
	drainFrames(t, w.leader)

	before := time.Now().UnixMilli()
	expel := func() {
		w.leader.Send(encodeRequestOustPledgeMember("Loner"))
		readUntilFrame(t, w.leader, "the leader's SUCCEEDED_IN_EXPELLING_CLAN_MEMBER", func(f []byte) bool {
			return slices.Equal(messages(t, [][]byte{f}), []int{serverpackets.SystemMessageSucceededInExpellingClanMember})
		})
	}
	armed.Store(duringRestore)
	lone.Send(encodeRequestGameStart(0))
	if duringRestore {
		<-holding
		expel()
		letGo()
	}
	if reply := lone.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	if reply := lone.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", reply[0])
	}
	if !duringRestore {
		expel()
	}
	lone.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	burst := drainFrames(t, lone)
	info, ok := firstOpcode(burst, serverpackets.OpcodeUserInfo)
	if !ok {
		t.Fatal("login burst carries no UserInfo")
	}
	if title := userInfoTitle(t, info); title != "" {
		t.Fatalf("login UserInfo title = %q, want none", title)
	}
	if _, ok := firstOpcode(burst, serverpackets.OpcodePledgeShowMemberListAll); ok {
		t.Fatal("login burst lists the clan that expelled Loner")
	}
	if cl, _ := w.srv.Clans.Table().Get(knightsClanID); cl.IsMember(lonerID) {
		t.Fatal("Knights still lists Loner")
	}

	// Rivals' leader invites Loner: refused for the penalty, so Loner is
	// clanless and under it.
	drainFrames(t, w.member)
	w.member.Send(encodeRequestJoinPledge(lonerID, 0))
	frames := drainFrames(t, w.member)
	if len(frames) != 1 {
		t.Fatalf("invitation answer = %x", opcodes(frames))
	}
	if id, params := sysMsg(t, frames[0]); id != serverpackets.SystemMessageS1MustWaitBeforeJoiningAnotherClan || !slices.Equal(params, []string{"Loner"}) {
		t.Fatalf("invitation message = %d %v, want S1_MUST_WAIT_BEFORE_JOINING_ANOTHER_CLAN Loner", id, params)
	}

	lone.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	readUntilFrame(t, lone, "the second restart's CharSelectInfo", func(f []byte) bool { return f[0] == serverpackets.OpcodeCharSelectInfo })
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COUNT(*) FROM characters WHERE obj_Id = ? AND COALESCE(clanid, 0) = 0 AND title = ''`, lonerID); got != 1 {
		t.Fatal("expelled Loner's row is not clanless and untitled")
	}
	if got := queryInt(t, w, `SELECT clan_join_expiry_time FROM characters WHERE obj_Id = ?`, lonerID); got < before+24*3600*1000 {
		t.Fatalf("expelled Loner's join penalty ends %d, want a day after %d", got, before)
	}
}
