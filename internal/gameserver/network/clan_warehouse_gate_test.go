package network

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// TestWarehouseGateDropsClanWarehouseOfExpelledMember expels a member with
// its clan warehouse open, the expulsion's notice still queued behind a
// warehouse request on the member's own queue: the request finds the member
// off the roster though its character still carries the clan id. The gate
// drops the warehouse and the request without an answer, as the reference
// clears the active warehouse when it removes the member.
func TestWarehouseGateDropsClanWarehouseOfExpelledMember(t *testing.T) {
	const clanID, leaderID, memberID = 0x10000001, 0x10000002, 7
	table := clan.NewTable()
	table.Restore(clan.Snapshot{
		Clans: []clan.Row{{ID: clanID, Name: "Keepers", Level: 1, LeaderID: leaderID}},
		Members: []clan.MemberRow{
			{ClanID: clanID, Member: clan.Member{ObjectID: leaderID, Name: "Leader"}},
			{ClanID: clanID, Member: clan.Member{ObjectID: memberID, Name: "Member", PowerGrade: clan.MemberPowerGrade}},
		},
	}, time.Now(), 1)
	svc := clan.NewService(table, nil, nil, nil, clan.DefaultConfig(), nil, zerolog.Nop())
	l := &GameClientLink{log: zerolog.Nop(), clans: svc}

	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, memberID, capture)
	live.Character.Name = "Member"
	live.SetClanID(clanID)
	activate := func() {
		live.storage.active = activeStore{store: itemcontainer.NewClanWarehouse(clanID, testItemTemplates()), clanID: clanID}
	}

	// Another clan's warehouse is not the member's to use either.
	live.storage.active = activeStore{store: itemcontainer.NewClanWarehouse(clanID+1, testItemTemplates()), clanID: clanID + 1}
	if _, ok := l.warehouseRequestGate(live); ok || live.storage.active.store != nil {
		t.Fatalf("another clan's warehouse passed the gate (still active: %v)", live.storage.active.store != nil)
	}

	activate()
	leader := &player.Character{ID: leaderID, Name: "Leader"}
	leader.SetClanID(clanID)
	if _, _, res := svc.Oust(leader, "Member", func(int32) *player.Character { return live.Character }, time.Now()); res != clan.Ousted {
		t.Fatalf("oust = %v, want Ousted", res)
	}
	if live.ClanID() != clanID {
		t.Fatal("the expulsion cleared the member's clan id off its queue")
	}
	if _, ok := l.warehouseRequestGate(live); ok {
		t.Fatal("an expelled member's clan warehouse passed the gate")
	}
	if live.storage.active.store != nil {
		t.Fatal("the expelled member's clan warehouse is still active")
	}
	if frames := capture.Frames(); len(frames) != 0 {
		t.Fatalf("the dropped request answered %d frames, want none", len(frames))
	}
}
