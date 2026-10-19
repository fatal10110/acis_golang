package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// TestShippedMotherTreeMessages: the shipped Mother Tree zones announce
// their crossings with the messages their data sets. The five elf-only
// zones (affectedRace 1) set enterMsgId 114 and leaveMsgId 115; the eleven
// zones open to every race set 1054 and 1055.
func TestShippedMotherTreeMessages(t *testing.T) {
	t.Parallel()
	index, err := LoadZones(datapackPath(t, filepath.Join("data", "xml", "zones")))
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}
	type announce struct{ race, enter, leave int }
	got := map[announce]int{}
	for _, z := range zone.OfKind[*zone.MotherTree](index) {
		got[announce{z.AffectedRace, z.EnterMessage, z.LeaveMessage}]++
	}
	want := map[announce]int{{1, 114, 115}: 5, {-1, 1054, 1055}: 11}
	if len(got) != len(want) {
		t.Fatalf("Mother Tree zones by (race, entry, exit) = %v, want %v", got, want)
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("Mother Tree zones by (race, entry, exit) = %v, want %v", got, want)
			break
		}
	}
}
