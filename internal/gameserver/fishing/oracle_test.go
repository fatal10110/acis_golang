package fishing

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// run is one "start:value" step of a golden's piecewise-constant sequence.
type run struct{ start, value int }

// valueAt returns the value of the run covering position at.
func valueAt(runs []run, at int) int {
	v := 0
	for _, r := range runs {
		if r.start > at {
			break
		}
		v = r.value
	}
	return v
}

func parseRuns(t *testing.T, fields []string) []run {
	t.Helper()
	var runs []run
	for _, f := range fields {
		s, v, ok := strings.Cut(f, ":")
		if !ok {
			t.Fatalf("malformed run %q", f)
		}
		runs = append(runs, run{atoi(t, s), atoi(t, v)})
	}
	return runs
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestFishingMatchesReferenceProbe replays testdata/fishing.golden, the output of
// testdata/oracle/run.sh: the reference's own lure grouping, look period,
// fish type table, fish level spread and pumping damage blocks run over
// every lure id from 6500 to 8599 and a grid of rolls and skills. A lure,
// group or roll the golden does not list must give the table's default:
// group 1, no look period, fish type 1.
func TestFishingMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/fishing.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	groups := map[int32]int{}
	delays := map[int32][]int{}
	types := map[[2]int][]run{}
	levels := map[int][]run{}
	type damageRow struct {
		power                    float32
		crystal                  int
		shot                     bool
		level, expertise, damage int
		penalty                  int
	}
	var damages []damageRow

	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		fl := strings.Fields(sc.Text())
		switch fl[0] {
		case "group":
			groups[int32(atoi(t, fl[1]))] = atoi(t, fl[2])
		case "delay":
			var d []int
			for _, s := range fl[2:] {
				d = append(d, atoi(t, s))
			}
			delays[int32(atoi(t, fl[1]))] = d
		case "type":
			types[[2]int{atoi(t, fl[1]), atoi(t, fl[2])}] = parseRuns(t, fl[3:])
		case "level":
			levels[atoi(t, fl[1])] = parseRuns(t, fl[2:])
		case "damage":
			p, err := strconv.ParseFloat(fl[1], 32)
			if err != nil {
				t.Fatal(err)
			}
			damages = append(damages, damageRow{float32(p), atoi(t, fl[2]), fl[3] == "1", atoi(t, fl[4]), atoi(t, fl[5]), atoi(t, fl[6]), atoi(t, fl[7])})
		default:
			t.Fatalf("malformed golden line %q", sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(groups) == 0 || len(delays) == 0 || len(types) == 0 || len(levels) == 0 || len(damages) == 0 {
		t.Fatal("golden is missing a section")
	}

	gutsGrid := []int{1, 3, 7, 99, 1000, 2000, 3000, 4000, 5000}
	for lure := int32(6500); lure < 8600; lure++ {
		want, ok := groups[lure]
		if !ok {
			want = 1
		}
		if got := LureGroup(lure); got != want {
			t.Errorf("LureGroup(%d) = %d, want %d", lure, got, want)
		}
		for i, guts := range gutsGrid {
			wantDelay := 0
			if d, ok := delays[lure]; ok {
				wantDelay = d[i]
			}
			if got := CheckDelay(lure, guts); got != time.Duration(wantDelay)*time.Millisecond {
				t.Errorf("CheckDelay(%d, %d) = %v, want %dms", lure, guts, got, wantDelay)
			}
		}
		for g := 0; g <= 2; g++ {
			runs, listed := types[[2]int{g, int(lure)}]
			for check := range 100 {
				want := 1
				if listed {
					want = valueAt(runs, check)
				}
				if got := fishType(g, lure, check); got != want {
					t.Errorf("fishType(group %d, lure %d, check %d) = %d, want %d", g, lure, check, got, want)
				}
			}
		}
	}

	for base, runs := range levels {
		for check := range 100 {
			roll := func(n int) int { return check }
			if got, want := fishLevel(base, roll), valueAt(runs, check); got != want {
				t.Errorf("fishLevel(%d) check %d = %d, want %d", base, check, got, want)
			}
		}
	}

	for _, d := range damages {
		gotDamage, gotPenalty := Damage(d.power, item.CrystalType(d.crystal), d.shot, d.level, d.expertise)
		if gotDamage != d.damage || gotPenalty != d.penalty {
			t.Errorf("Damage(%v, crystal %d, shot %v, level %d, expertise %d) = %d, %d; want %d, %d",
				d.power, d.crystal, d.shot, d.level, d.expertise, gotDamage, gotPenalty, d.damage, d.penalty)
		}
	}
}
