package clan

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// enterExtra seeds name on account at level 10 with what raising a clan to
// level 3 costs, and brings it into the world beside the others.
func (w *clanWorld) enterExtra(t *testing.T, account, name string) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	id := w.srv.SeedCharacterFor(t, account, name, 10, toLevel3SP).ID
	w.srv.GiveItem(t, id, item.AdenaID, toLevel3Adena)
	w.srv.GiveItem(t, id, bloodMarkID, 1)
	c := w.srv.DialClient(t, account, 1)
	startInWorld(t, c)
	return c, id
}

// charInfoOf reports whether frames carry a CharInfo describing objectID,
// its fifth field after the position and heading.
func charInfoOf(frames [][]byte, objectID int32) bool {
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		for range 4 {
			r.ReadInt32()
		}
		if r.ReadInt32() == objectID {
			return true
		}
	}
	return false
}

// TestClanChangeRefreshesWarTags has Knights at war with Rivals, whose
// leader stands among Knights, and a third clan, Raiders, at war with
// Knights only from its side. A recruit joining Knights, then leaving it,
// then a second recruit expelled while offline, each refresh the Rivals
// leader around the Knights member who acted: it gets its UserInfo and its
// neighbours its CharInfo. The Raiders leader, of a clan Knights did not
// declare war on, is not refreshed.
func TestClanChangeRefreshesWarTags(t *testing.T) {
	var now atomic.Int64
	now.Store(time.Now().UnixMilli())
	w := bootWarWorld(t, gameservertest.WithClanClock(func() time.Time { return time.UnixMilli(now.Load()) }))
	w.found(t, "Knights")
	w.masterCommandBy(t, w.member, "create_clan Rivals")
	w.raiseToLevel3(t, w.leader)
	w.raiseToLevel3(t, w.member)

	raider, raiderID := w.enterExtra(t, "player3", "Raider")
	recruit, recruitID := w.enterExtra(t, "player4", "Recruit")
	squire, squireID := w.enterExtra(t, "player5", "Squire")
	all := []*testsupport.ScriptedClient{w.leader, w.member, raider, recruit, squire}
	drainAll := func() [][][]byte {
		out := make([][][]byte, len(all))
		for i, c := range all {
			out[i] = drainFrames(t, c)
		}
		return out
	}
	drainAll()
	if _, ok := firstOpcode(w.masterCommandBy(t, raider, "create_clan Raiders"), serverpackets.OpcodePledgeShowMemberListAll); !ok {
		t.Fatal("raider founded no clan")
	}
	w.raiseToLevel3(t, raider)

	w.leader.Send(encodeWarName(clientpackets.OpcodeRequestStartPledgeWar, "Rivals"))
	raider.Send(encodeWarName(clientpackets.OpcodeRequestStartPledgeWar, "Knights"))
	drainAll()

	wantRefresh := func(what string) {
		t.Helper()
		frames := drainAll()
		enemy, bystander := frames[1], frames[2]
		if _, ok := firstOpcode(enemy, serverpackets.OpcodeUserInfo); !ok {
			t.Fatalf("%s: enemy got %x, want its UserInfo refresh", what, opcodes(enemy))
		}
		if !charInfoOf(bystander, w.memberID) {
			t.Fatalf("%s: enemy's neighbour got %x, want the enemy's CharInfo", what, opcodes(bystander))
		}
		if _, ok := firstOpcode(bystander, serverpackets.OpcodeUserInfo); ok {
			t.Fatalf("%s: player of a clan attacking Knights got a UserInfo refresh", what)
		}
		if charInfoOf(enemy, raiderID) {
			t.Fatalf("%s: player of a clan attacking Knights refreshed to its neighbours", what)
		}
	}

	w.leader.Send(encodeRequestJoinPledge(recruitID, 0))
	drainFrames(t, recruit)
	recruit.Send(encodeRequestAnswerJoinPledge(1))
	wantRefresh("join")

	recruit.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawPledge).Bytes())
	wantRefresh("withdrawal")

	// The leader stays busy with its first invitation until it lapses.
	now.Add(clan.InviteTimeout.Milliseconds())
	w.leader.Send(encodeRequestJoinPledge(squireID, 0))
	drainFrames(t, squire)
	squire.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainAll()[4], serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("second recruit did not join")
	}
	all = all[:4]
	w.leaveWorld(t, squire)
	drainAll()
	w.leader.Send(encodeRequestOustPledgeMember("Squire"))
	wantRefresh("expulsion of an offline member")
}
