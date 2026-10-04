package clan

import (
	"testing"
	"time"
)

// TestRestoreHallsSkipsUnknownHallsAndClans restores clanhall owners as
// the hall table and clan registry allow: a known hall goes to its
// existing owner, a hall the hall data lacks and an owner that no longer
// exists are skipped, and a clan named by two rows keeps the later one.
func TestRestoreHallsSkipsUnknownHallsAndClans(t *testing.T) {
	table := NewTable()
	table.Restore(Snapshot{Clans: []Row{{ID: 1, CastleID: 3}, {ID: 2}, {ID: 3}, {ID: 4}}}, time.Now(), 1)
	table.RestoreHalls([]HallOwner{
		{HallID: 21, ClanID: 1},
		{HallID: 99, ClanID: 2},
		{HallID: 22, ClanID: 404},
		{HallID: 23, ClanID: 3},
		{HallID: 24, ClanID: 3},
	}, func(id int32) bool { return id >= 21 && id <= 64 })
	for _, tt := range []struct {
		clan         int32
		castle, hall int32
	}{{1, 3, 21}, {2, 0, 0}, {3, 0, 24}, {4, 0, 0}} {
		cl, ok := table.Get(tt.clan)
		if !ok {
			t.Fatalf("clan %d not restored", tt.clan)
		}
		if cl.CastleID() != tt.castle || cl.HallID() != tt.hall {
			t.Fatalf("clan %d castle/hall = %d/%d, want %d/%d", tt.clan, cl.CastleID(), cl.HallID(), tt.castle, tt.hall)
		}
		if info := cl.Info(); info.CastleID != tt.castle || info.HallID != tt.hall {
			t.Fatalf("clan %d info castle/hall = %d/%d, want %d/%d", tt.clan, info.CastleID, info.HallID, tt.castle, tt.hall)
		}
	}
}
