package castlemanor

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// knownDivergence lists the golden rows whose configured wall-clock time falls
// in the hour a clock change skips or repeats (a manor time set to 02:30, say),
// where the reference calendar and time.Date resolve the time differently.
// Tracked in https://github.com/fatal10110/acis_golang/issues/5341
// (default manor times, 06:00 and 20:00, never land there). A row leaves the
// list when the fix makes it agree.
var knownDivergence = map[string]bool{
	"Europe/Paris 2026-03-28T10:00:00.123 modifiable 2:30,2:30+6":      true,
	"Europe/Paris 2026-03-28T19:00:00.000 modifiable 2:30,2:30+6":      true,
	"Europe/Paris 2026-03-29T12:00:00.250 modifiable 2:30,2:30+6":      true,
	"Europe/Paris 2026-10-24T10:00:00.123 modifiable 2:30,2:30+6":      true,
	"America/New_York 2026-03-08T12:00:00.250 modifiable 2:30,2:30+6":  true,
	"America/New_York 2026-03-08T12:00:00.250 maintenance 2:30,2:30+6": true,
	"America/New_York 2026-03-08T12:00:00.250 approved 2:30,2:30+6":    true,
	"America/New_York 2026-03-08T12:00:00.250 maintenance 3:0,1:55+10": true,
	"America/New_York 2026-11-01T12:00:00.000 approved 3:0,1:55+10":    true,
	"Australia/Sydney 2026-04-04T10:00:00.123 modifiable 2:30,2:30+6":  true,
	"Australia/Sydney 2026-10-03T10:00:00.123 modifiable 2:30,2:30+6":  true,
	"Australia/Sydney 2026-10-04T12:00:00.000 modifiable 2:30,2:30+6":  true,
}

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
		key := strings.Join([]string{zone, local, mode, clocks}, " ")
		switch diverges := knownDivergence[key]; {
		case got != want && !diverges:
			t.Errorf("%s manor in %s at %s (%s) changes at %s, want %s", mode, zone, local, clocks, time.UnixMilli(got).In(loc), time.UnixMilli(want).In(loc))
		case got == want && diverges:
			t.Errorf("%s no longer diverges from the reference; drop it from knownDivergence", key)
		}
		rows++
	}
	if rows == 0 {
		t.Fatal("empty golden")
	}
}
