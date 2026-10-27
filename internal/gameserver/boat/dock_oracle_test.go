package boat

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// TestDocksMatchReferenceProbe replays testdata/dock.golden, the output of
// testdata/oracle/run.sh over the reference's own dock table and line
// geometry. "dock" lines are each dock's data; "board" lines a walk's
// entrance, exit and adjusted entrance boarding points from aboard (1) and
// from the shore (0); "convert" lines the deck-to-world and world-to-deck
// maps. The walks include the Rune ones the boat passenger tests rely on
// (their constants in tests/boat/passenger_helpers_test.go).
func TestDocksMatchReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/dock.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	docks, boards, converts := 0, 0, 0
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		head, tail, _ := strings.Cut(sc.Text(), " => ")
		fl := strings.Fields(head)
		atoi := func(s string) int {
			n, err := strconv.Atoi(s)
			if err != nil {
				t.Fatalf("golden line %d: %v", line, err)
			}
			return n
		}
		switch fl[0] {
		case "dock":
			d := dockSites[route.Dock(fl[1])]
			if d == nil {
				t.Fatalf("golden line %d: no dock %s", line, fl[1])
			}
			got := fmt.Sprintf("dock %s at %d %d %d oust %d %d %d entrance %d %d %d %d exit %d %d %d %d axis %d %d %d %d exclusive %t",
				fl[1], d.at.X, d.at.Y, d.at.Z, d.oust.X, d.oust.Y, d.oust.Z,
				d.entrance.x1, d.entrance.y1, d.entrance.x2, d.entrance.y2,
				d.exit.x1, d.exit.y1, d.exit.x2, d.exit.y2,
				d.axis.x1, d.axis.y1, d.axis.x2, d.axis.y2, d.exclusive)
			if got != head {
				t.Errorf("line %d:\n got %s\nwant %s", line, got, head)
			}
			docks++
		case "board":
			d := dockSites[route.Dock(fl[1])]
			inBoat := fl[2] == "1"
			origin, dest := Point{atoi(fl[3]), atoi(fl[4])}, Point{atoi(fl[5]), atoi(fl[6])}
			got := fmt.Sprintf("%s | %s | %s",
				crossing(d.BoardingPoint(origin, dest, inBoat)),
				crossing(d.ExitPoint(origin, dest, inBoat)),
				pt(d.AdjustedBoardingPoint(origin, dest, inBoat)))
			if got != tail {
				t.Errorf("line %d: %s:\n got %s\nwant %s", line, head, got, tail)
			}
			boards++
		case "convert":
			d := dockSites[route.Dock(fl[1])]
			got := fmt.Sprintf("%s | %s",
				pt(d.BoatToWorld(atoi(fl[2]), atoi(fl[3]))),
				pt(d.WorldToBoat(atoi(fl[4]), atoi(fl[5]))))
			if got != tail {
				t.Errorf("line %d: %s:\n got %s\nwant %s", line, head, got, tail)
			}
			converts++
		default:
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if docks != len(dockSites) || boards == 0 || converts == 0 {
		t.Fatalf("golden holds %d docks of %d, %d walks, %d conversions", docks, len(dockSites), boards, converts)
	}
}

// crossing renders a boarding point as the probe does: "x y", or "none" for
// a walk that does not cross the line.
func crossing(p Point, ok bool) string {
	if !ok {
		return "none"
	}
	return pt(p)
}

func pt(p Point) string { return fmt.Sprintf("%d %d", p.X, p.Y) }
