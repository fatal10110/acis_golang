package clan

import (
	"encoding/binary"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The shared catalog's wyvern collar and the wyvern (npc 12621) it calls.
const (
	wyvernCollarID = int32(9601)
	wyvernNPCID    = int32(12621)
)

// wyvernDismount is what a rider sees getting off its wyvern: the feed
// gauge cleared, its skills without Wyvern Breath, the Ride dismount, its
// own UserInfo.
var wyvernDismount = []byte{
	serverpackets.OpcodeSetupGauge, serverpackets.OpcodeSkillList,
	serverpackets.OpcodeRide, serverpackets.OpcodeUserInfo,
}

// TestClanLeaderTransferDismountsFlyingFormerLeader: a former leader
// riding a wyvern when the transfer runs is first taken off it, then shows
// its new rank and takes off the Lord's Crown, ahead of the new leader's
// part and the status tail; it ends on foot.
func TestClanLeaderTransferDismountsFlyingFormerLeader(t *testing.T) {
	t.Parallel()
	summons, err := item.NewSummonItemTable([]item.SummonItem{{ItemID: wyvernCollarID, NPCID: wyvernNPCID, SummonType: 2}})
	if err != nil {
		t.Fatalf("summon items: %v", err)
	}
	var collar int32
	w, _ := bootLeaderTransferWith(t, func(srv *gameservertest.Server, leaderID int32) {
		collar = srv.GiveItem(t, leaderID, wyvernCollarID, 1)
	}, gameservertest.WithSummonItems(summons))
	// A rider cannot talk to the village master, so the founder nominates
	// the recruit first, then mounts.
	nominateRecruit(t, w)
	useItem := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	useItem.WriteInt32(collar)
	useItem.WriteInt32(0)
	w.leader.Send(useItem.Bytes())
	if _, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodeRide); !ok {
		t.Fatal("founder got no Ride using the wyvern collar")
	}
	if !founderFlying(t, w) {
		t.Fatal("founder is not flying after using the wyvern collar")
	}
	drainFrames(t, w.member)
	runTransfer(t, w)

	founder := checkStatusTail(t, "former leader", drainFrames(t, w.leader))
	want := slices.Concat(wyvernDismount, formerLeaderPart)
	checkOpcodes(t, "former leader", founder, want)
	if ride := founder[2]; len(ride) < 9 || binary.LittleEndian.Uint32(ride[5:9]) != 0 {
		t.Fatalf("former leader Ride = %x, want a dismount", ride)
	}
	checkCrownDisarmed(t, founder[len(wyvernDismount)+1])
	if founderFlying(t, w) {
		t.Error("former leader still flying after the transfer, want it off the wyvern")
	}
	if got := w.srv.PlayerPledgeClass(t, w.leaderID); got != 2 {
		t.Errorf("former leader's rank = %d, want 2 (a level 5 member)", got)
	}
}

// founderFlying reports whether the founder rides a flying mount.
func founderFlying(t *testing.T, w *clanWorld) bool {
	t.Helper()
	obj, ok := w.srv.State.Player(w.leaderID)
	if !ok {
		t.Fatal("founder missing from world state")
	}
	rider, ok := obj.(interface{ Flying() bool })
	if !ok {
		t.Fatalf("world player %T does not report flying", obj)
	}
	return rider.Flying()
}
