package fishchamp

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNextEndMatchesReferenceAcrossDaylightSaving replays testdata/dst.golden,
// "fishing <zone> <local time> => <from ms> <week end ms>" from the reference
// calendar code (internal/gameserver/testdata/dst/run.sh): the week end is
// 19:00 wall clock on the Tuesday of the week holding the day six days on, so
// a week spanning a clock change keeps 19:00.
func TestNextEndMatchesReferenceAcrossDaylightSaving(t *testing.T) {
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
		if got := NextEnd(from, loc); got != want {
			t.Errorf("NextEnd(%s %s) = %s, want %s", zone, local, time.UnixMilli(got).In(loc), time.UnixMilli(want).In(loc))
		}
		rows++
	}
	if rows == 0 {
		t.Fatal("empty golden")
	}
}
