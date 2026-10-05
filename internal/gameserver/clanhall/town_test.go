package clanhall

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/rs/zerolog"
)

// Reference: ClanHallManager.getClanHallsByLocation
// (ClanHallManager.java:212-215) keeps the halls whose town name equals
// the location ignoring case.
func TestInTownMatchesTownIgnoringCase(t *testing.T) {
	t.Parallel()
	const owner = int32(0x10000031)
	var halls []*hallmodel.Hall
	for _, a := range []hallmodel.HallAttrs{
		{ID: 24, Alias: "hall_24", Name: "Third", Description: "Third", Town: "Gludio Castle"},
		{ID: 22, Alias: "hall_22", Name: "First", Description: "First", Town: "gludio castle"},
		{ID: 23, Alias: "hall_23", Name: "Second", Description: "Second", Town: "Gludio"},
	} {
		h, err := hallmodel.NewHall(a, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		halls = append(halls, h)
	}
	data, err := hallmodel.NewTable(halls)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1_700_000_000, 0)
	clans := clan.NewTable()
	clans.Restore(clan.Snapshot{Clans: []clan.Row{{ID: owner, Name: "Owner", Level: 4}}}, start, 1)
	store := &laneStore{rows: []HallRow{{ID: 24, OwnerID: owner, PaidUntil: start.UnixMilli() + weekMs, Paid: true}}, bidAt: map[int32]int32{}}
	hs := NewHalls(data, clans, nil, store, nil, zerolog.Nop())
	if err := hs.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}

	var got []string
	for _, v := range hs.InTown("GLUDIO CASTLE") {
		got = append(got, fmt.Sprintf("%d %s %d", v.ID, v.Name, v.OwnerID))
	}
	want := []string{"22 First 0", fmt.Sprintf("24 Third %d", owner)}
	if !slices.Equal(got, want) {
		t.Fatalf("InTown = %q, want %q", got, want)
	}
	if got := hs.InTown("Dion"); len(got) != 0 {
		t.Fatalf("InTown(Dion) = %+v, want none", got)
	}
	if got := (*Halls)(nil).InTown("Gludio"); got != nil {
		t.Fatalf("nil InTown = %+v, want nil", got)
	}
}
