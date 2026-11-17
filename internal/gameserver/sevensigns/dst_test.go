package sevensigns

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNextPeriodChangeMatchesReferenceAcrossDaylightSaving replays
// testdata/dst.golden, "sevensigns <zone> <local time> => <from ms> <change ms>"
// from the reference calendar code (internal/gameserver/testdata/dst/run.sh):
// a major period ends at 18:00 wall clock on the next Monday, in the weeks
// around a clock change and either side of the Monday 18:00 boundary.
func TestNextPeriodChangeMatchesReferenceAcrossDaylightSaving(t *testing.T) {
	f, err := os.Open("testdata/dst.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var kind, zone, local string
		var from, want int64
		if _, err := fmt.Sscanf(strings.Replace(sc.Text(), "=>", "", 1), "%s %s %s %d %d", &kind, &zone, &local, &from, &want); err != nil {
			t.Fatalf("golden line %q: %v", sc.Text(), err)
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		now := time.UnixMilli(from).In(loc)
		for _, period := range []Period{Competition, SealValidation} {
			if got := nextPeriodChange(period, now); got.UnixMilli() != want {
				t.Errorf("nextPeriodChange(%v, %s %s) = %s, want %s", period, zone, local, got, time.UnixMilli(want).In(loc))
			}
		}
		rows++
	}
	if rows == 0 {
		t.Fatal("empty golden")
	}
}
