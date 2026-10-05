package network

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// trapSiegeStore is a siege_clans table holding rows and dropping writes.
type trapSiegeStore struct{ rows []siege.ClanRow }

func (s trapSiegeStore) LoadClans(context.Context) ([]siege.ClanRow, error) { return s.rows, nil }
func (trapSiegeStore) SaveClan(context.Context, int32, int32, siege.Side) error {
	return nil
}
func (trapSiegeStore) DeleteClan(context.Context, int32, int32) error { return nil }
func (trapSiegeStore) DeleteClans(context.Context, int32) error       { return nil }
func (trapSiegeStore) DeletePending(context.Context, int32) error     { return nil }

// TestTrapTrippedTellsTheDefendingClans pins the castle trap announcement,
// Siege.announce(A_TRAP_DEVICE_HAS_BEEN_TRIPPED, SiegeSide.DEFENDER): the
// online members of the castle owner's clan and of the approved defenders
// get message 714; an attacker and a defender still waiting for approval
// get nothing.
func TestTrapTrippedTellsTheDefendingClans(t *testing.T) {
	const (
		owner    int32 = 0x10000001
		defender int32 = 0x10000002
		attacker int32 = 0x10000003
		pending  int32 = 0x10000004
	)
	clans := clan.NewTable()
	clans.Restore(clan.Snapshot{
		Clans: []clan.Row{
			{ID: owner, Name: "Lords", Level: 5, CastleID: 1},
			{ID: defender, Name: "Kings", Level: 5},
			{ID: attacker, Name: "Rivals", Level: 5},
			{ID: pending, Name: "Waiting", Level: 5},
		},
		Members: []clan.MemberRow{
			{ClanID: owner, Member: clan.Member{ObjectID: 1, Name: "Lord"}},
			{ClanID: defender, Member: clan.Member{ObjectID: 2, Name: "King"}},
			{ClanID: attacker, Member: clan.Member{ObjectID: 3, Name: "Rival"}},
			{ClanID: pending, Member: clan.Member{ObjectID: 4, Name: "Waiter"}},
		},
	}, time.Now(), 1)

	gludio, err := castledata.NewCastle(castledata.CastleAttrs{ID: 1, Alias: "gludio_castle", Name: "Gludio Castle"}, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	table, err := castledata.NewTable([]*castledata.Castle{gludio})
	if err != nil {
		t.Fatal(err)
	}
	castles := castle.NewManager(table, clans, nil, nil, zerolog.Nop())
	castles.Restore([]castle.Row{{ID: 1}}, []castle.Owner{{ClanID: owner, CastleID: 1}})
	sieges := siege.New(siege.DefaultConfig(), castles, clans, nil, trapSiegeStore{rows: []siege.ClanRow{
		{CastleID: 1, ClanID: defender, Side: siege.SideDefender},
		{CastleID: 1, ClanID: attacker, Side: siege.SideAttacker},
		{CastleID: 1, ClanID: pending, Side: siege.SidePending},
	}}, nil, zerolog.Nop())
	if err := sieges.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}

	clock := sim.NewInline(time.Unix(0, 0))
	state := world.New()
	captures := map[int32]*testsupport.FrameCapture{}
	for _, m := range []struct {
		id     int32
		clanID int32
	}{{1, owner}, {2, defender}, {3, attacker}, {4, pending}} {
		capture := &testsupport.FrameCapture{}
		live := newTestLivePlayer(t, m.id, capture)
		live.Live.SetQueue(clock.NewQueue("player"))
		state.Spawn(live, int(m.id)*100, 0, 0, 0)
		state.AddPlayer(live)
		cl, _ := clans.Get(m.clanID)
		if !cl.SetOnline(m.id, clan.Member{}) {
			t.Fatalf("player %d is not a member of clan %#x", m.id, m.clanID)
		}
		captures[m.id] = capture
	}
	clock.Run()
	for _, c := range captures {
		testsupport.ResetCapture(c)
	}

	link := &GameClientLink{world: state, sieges: sieges, log: zerolog.Nop()}
	link.trapTripped(1)
	clock.Run()

	for id, want := range map[int32]bool{1: true, 2: true, 3: false, 4: false} {
		frames := captures[id].Frames()
		if !want {
			if len(frames) != 0 {
				t.Fatalf("player %d got %d frames, want none", id, len(frames))
			}
			continue
		}
		if len(frames) != 1 {
			t.Fatalf("player %d got %d frames, want the trap message", id, len(frames))
		}
		assertStaticSystemMessageFrame(t, frames[0], 714) // A_TRAP_DEVICE_HAS_BEEN_TRIPPED
	}
}
