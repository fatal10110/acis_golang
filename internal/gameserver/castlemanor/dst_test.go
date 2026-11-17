package castlemanor

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNextChangeMatchesReferenceAcrossDaylightSaving replays testdata/dst.golden,
// "manor <zone> <local time> <mode> <approve h:m>,<refresh h:m>+<maintenance>
// => <now ms> <next change ms>" from the reference calendar code
// (internal/gameserver/testdata/dst/run.sh): the next mode change keeps its
// wall-clock time across a clock change, including a time the change skips.
func TestNextChangeMatchesReferenceAcrossDaylightSaving(t *testing.T) {
	f, err := os.Open("testdata/dst.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	modes := map[string]Status{"modifiable": StatusModifiable, "maintenance": StatusMaintenance, "approved": StatusApproved}
	rows := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var kind, zone, local, mode, clocks string
		var now, want int64
		if _, err := fmt.Sscanf(strings.Replace(sc.Text(), "=>", "", 1), "%s %s %s %s %s %d %d", &kind, &zone, &local, &mode, &clocks, &now, &want); err != nil {
			t.Fatalf("golden line %q: %v", sc.Text(), err)
		}
		var cfg Config
		if _, err := fmt.Sscanf(clocks, "%d:%d,%d:%d+%d", &cfg.ApproveHour, &cfg.ApproveMin, &cfg.RefreshHour, &cfg.RefreshMin, &cfg.MaintenanceMin); err != nil {
			t.Fatalf("golden clocks %q: %v", clocks, err)
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		got := nextChangeAt(cfg, modes[mode], time.UnixMilli(now).In(loc)).UnixMilli()
		if got != want {
			t.Errorf("%s manor in %s at %s (%s) changes at %s, want %s", mode, zone, local, clocks, time.UnixMilli(got).In(loc), time.UnixMilli(want).In(loc))
		}
		rows++
	}
	if rows == 0 {
		t.Fatal("empty golden")
	}
}
