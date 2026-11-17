package lottery

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNextDrawingMatchesReferenceAcrossDaylightSaving replays testdata/dst.golden,
// "lottery <zone> <local time> => <from ms> <drawing ms>" from the reference
// calendar code (internal/gameserver/testdata/dst/run.sh): the Saturday
// drawing keeps its 19:00 wall clock, and a drawing set from a Saturday is
// 168 elapsed hours on, so a week spanning a clock change lands an hour off.
func TestNextDrawingMatchesReferenceAcrossDaylightSaving(t *testing.T) {
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
		if got := nextDrawing(from, loc); got != want {
			t.Errorf("nextDrawing(%s %s) = %s, want %s", zone, local, time.UnixMilli(got).In(loc), time.UnixMilli(want).In(loc))
		}
		rows++
	}
	if rows == 0 {
		t.Fatal("empty golden")
	}
}
