package siege

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNextDateMatchesReference replays testdata/dst.golden, "siege <zone>
// <local time> <castle> => <now ms> <date ms>" from the reference calendar
// code with Monday-first weeks (internal/gameserver/testdata/dst/run.sh): a
// date already passed moves to the castle's Saturday or Sunday of the
// Monday-to-Sunday week holding now, two weeks on, at 18:00 wall clock,
// across clock changes, Sundays, Mondays and the turn of the year.
func TestNextDateMatchesReference(t *testing.T) {
	f, err := os.Open("testdata/dst.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var kind, zone, local string
		var castle int
		var now, want int64
		if _, err := fmt.Sscanf(strings.Replace(sc.Text(), "=>", "", 1), "%s %s %s %d %d %d", &kind, &zone, &local, &castle, &now, &want); err != nil {
			t.Fatalf("golden line %q: %v", sc.Text(), err)
		}
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatal(err)
		}
		if got := NextDate(castle, time.UnixMilli(0), time.UnixMilli(now).In(loc)); got.UnixMilli() != want {
			t.Errorf("NextDate(%d, %s %s) = %s, want %s", castle, zone, local, got, time.UnixMilli(want).In(loc))
		}
		rows++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if rows == 0 {
		t.Fatal("empty golden")
	}
}
