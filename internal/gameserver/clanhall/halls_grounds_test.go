package clanhall

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// Reference: ClanHall.setOwner (ClanHall.java:237-292) refreshes the former
// owner's clan header, removes the functions, closes every gate
// (Residence.closeDoors), sets the new owner, refreshes its header, then
// banishes the players of other clans (banishForeigners) before storing
// the row; a null clan returns before any of it. ClanHall.free
// (ClanHall.java:195-233) closes the gates too, and banishes no one.

const (
	groundsHall   = int32(22)
	groundsFormer = int32(0x10000021)
	groundsWinner = int32(0x10000022)
)

// groundsLog records the clan header refreshes and the grounds calls in
// the order they were made.
type groundsLog struct{ calls []string }

func (g *groundsLog) HallChanged(cl *clan.Clan) {
	g.calls = append(g.calls, fmt.Sprintf("header %d hall %d", cl.ID(), cl.HallID()))
}

func (g *groundsLog) TellClan(cl *clan.Clan, n Notice) {
	g.calls = append(g.calls, fmt.Sprintf("tell %d kind %d", cl.ID(), n.Kind))
}

func (g *groundsLog) CloseDoors(gates []string) {
	g.calls = append(g.calls, "close "+strings.Join(gates, ";"))
}

func (g *groundsLog) BanishForeigners(hallID, clanID int32) {
	g.calls = append(g.calls, fmt.Sprintf("banish hall %d keep %d", hallID, clanID))
}

// refusingBank pays no lease.
type refusingBank struct{}

func (refusingBank) PayHallFee(int32, int) bool { return false }
func (refusingBank) ReturnAdena(int32, int)     {}

// groundsHalls restores a hall with two gates owned by groundsFormer, its
// lease unpaid and due at start, beside the clan groundsWinner.
func groundsHalls(t *testing.T, start time.Time) (*Halls, *clan.Table) {
	t.Helper()
	h, err := hallmodel.NewHall(hallmodel.HallAttrs{
		ID: int(groundsHall), Alias: "hall_22", Name: "Hall", Description: "Hall", Town: "Town",
		Lease: 500_000, Gates: []string{"gate_a", "gate_b"},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := hallmodel.NewTable([]*hallmodel.Hall{h})
	if err != nil {
		t.Fatal(err)
	}
	clans := clan.NewTable()
	clans.Restore(clan.Snapshot{Clans: []clan.Row{
		{ID: groundsFormer, Name: "Former", Level: 4}, {ID: groundsWinner, Name: "Winner", Level: 4},
	}}, start, 1)
	clans.RestoreHalls([]clan.HallOwner{{HallID: groundsHall, ClanID: groundsFormer}}, nil)
	store := &laneStore{
		rows:  []HallRow{{ID: groundsHall, OwnerID: groundsFormer, PaidUntil: start.UnixMilli() + weekMs, Paid: false}},
		bidAt: map[int32]int32{},
	}
	hs := NewHalls(data, clans, nil, store, nil, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	return hs, clans
}

// A hall changing hands closes its gates between the two clan header
// refreshes, then throws out every player not of the new owner.
func TestSetOwnerClosesGatesThenBanishes(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	hs, clans := groundsHalls(t, start)
	log := &groundsLog{}
	hs.Start(sim.NewInline(start).NewQueue("clanhalls"), refusingBank{}, log, log)
	winner, _ := clans.Get(groundsWinner)

	if !hs.SetOwner(groundsHall, winner) {
		t.Fatal("SetOwner refused a known hall")
	}
	want := []string{
		fmt.Sprintf("header %d hall 0", groundsFormer),
		"close gate_a;gate_b",
		fmt.Sprintf("header %d hall %d", groundsWinner, groundsHall),
		fmt.Sprintf("banish hall %d keep %d", groundsHall, groundsWinner),
	}
	if !slices.Equal(log.calls, want) {
		t.Fatalf("calls = %q, want %q", log.calls, want)
	}
}

// An award to no clan, a winner gone since it bid, leaves the gates and
// the grounds alone.
func TestSetOwnerToNoClanLeavesGrounds(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	hs, _ := groundsHalls(t, start)
	log := &groundsLog{}
	hs.Start(sim.NewInline(start).NewQueue("clanhalls"), refusingBank{}, log, log)

	hs.SetOwner(groundsHall, nil)
	if len(log.calls) != 0 {
		t.Fatalf("calls = %q, want none", log.calls)
	}
}

// A hall lost to an overdue lease closes its gates after the former
// owner's header refresh and throws no one out.
func TestFreeClosesGatesWithoutBanishing(t *testing.T) {
	t.Parallel()
	start := time.Unix(1_700_000_000, 0)
	hs, _ := groundsHalls(t, start)
	log := &groundsLog{}
	clock := sim.NewInline(start)
	hs.Start(clock.NewQueue("clanhalls"), refusingBank{}, log, log)

	clock.Advance(7 * 24 * time.Hour)
	want := []string{
		fmt.Sprintf("header %d hall 0", groundsFormer),
		"close gate_a;gate_b",
		fmt.Sprintf("tell %d kind %d", groundsFormer, NoticeFeeOverdue),
	}
	if !slices.Equal(log.calls, want) {
		t.Fatalf("calls = %q, want %q", log.calls, want)
	}
	if v, _ := hs.View(groundsHall); v.OwnerID != 0 {
		t.Fatalf("hall owner = %d, want free", v.OwnerID)
	}
}
