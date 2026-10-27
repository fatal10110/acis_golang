// Package vitals answers the exact door and pet vital-sign values behavior
// tests assert: door damage stages, door regeneration ticks, revive HP, a
// pet's maximum HP and the experience a resurrection gives back. The answers
// are not worked by hand: they come from testdata/vitals.golden, the output of
// testdata/oracle/run.sh, which runs the reference code itself (formats in the
// README there). A question the golden does not list fails the test.
package vitals

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

//go:embed testdata/vitals.golden
var golden string

// Tick is a door's state after one regeneration tick.
type Tick struct {
	HP     float64
	Active bool // the regeneration task is still running
}

type run struct {
	from  float64
	stage int
}

type regenKey struct{ multiplier, start float64 }

type petExpKey struct {
	before, exp int64
	power       float64
}

type table struct {
	damage map[int][]run
	period map[string]time.Duration
	regen  map[regenKey][]Tick
	revive map[int]float64
	petHP  map[[2]int]int
	petExp map[petExpKey]int64
}

var (
	once   sync.Once
	loaded table
)

func load() *table {
	once.Do(func() {
		loaded = table{
			damage: map[int][]run{}, period: map[string]time.Duration{}, regen: map[regenKey][]Tick{},
			revive: map[int]float64{}, petHP: map[[2]int]int{}, petExp: map[petExpKey]int64{},
		}
		for _, line := range strings.Split(golden, "\n") {
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			if err := loaded.add(f); err != nil {
				panic(fmt.Sprintf("vitals.golden line %q: %v", line, err))
			}
		}
	})
	return &loaded
}

func (t *table) add(f []string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	num := func(s string) float64 { return must(strconv.ParseFloat(s, 64)) }
	whole := func(s string) int64 { return must(strconv.ParseInt(s, 10, 64)) }
	switch f[0] {
	case "damage":
		max := int(whole(f[1]))
		for _, r := range f[2:] {
			from, stage, _ := strings.Cut(r, ":")
			t.damage[max] = append(t.damage[max], run{num(from), int(whole(stage))})
		}
	case "period":
		t.period[f[1]] = time.Duration(whole(f[2])) * time.Millisecond
	case "regen":
		key := regenKey{num(f[1]), num(f[2])}
		for _, tick := range f[3:] {
			hp, active, _ := strings.Cut(tick, ":")
			t.regen[key] = append(t.regen[key], Tick{HP: num(hp), Active: active == "1"})
		}
	case "revive":
		t.revive[int(whole(f[1]))] = num(f[2])
	case "petmaxhp":
		t.petHP[[2]int{int(whole(f[1])), int(whole(f[2]))}] = int(whole(f[3]))
	case "petexp":
		t.petExp[petExpKey{whole(f[1]), whole(f[2]), num(f[3])}] = whole(f[4])
	default:
		return fmt.Errorf("unknown line kind %q", f[0])
	}
	return nil
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// DoorStage is the damage stage (0 full health, 6 broken) a door of maxHP
// reports at hp, which must lie on the golden's half-point grid.
func DoorStage(tb testing.TB, maxHP int, hp float64) int {
	tb.Helper()
	runs := load().damage[maxHP]
	if len(runs) == 0 {
		tb.Fatalf("vitals.golden has no damage line for max HP %d", maxHP)
	}
	if hp*2 != float64(int64(hp*2)) || maxHP > 1200 {
		tb.Fatalf("vitals.golden covers door HP in half points up to max HP 1200, not %v of %d", hp, maxHP)
	}
	stage := runs[0].stage
	for _, r := range runs {
		if r.from > hp {
			break
		}
		stage = r.stage
	}
	return stage
}

// DoorRegenPeriod is the time between a door's regeneration ticks.
func DoorRegenPeriod(tb testing.TB) time.Duration {
	tb.Helper()
	d, ok := load().period["door"]
	if !ok {
		tb.Fatal("vitals.golden has no door period line")
	}
	return d
}

// DoorRegenTicks is a door of 600 HP's state after each regeneration tick,
// from start HP until the task stops, under the players.properties
// HpRegenMultiplier multiplier (1 is the default).
func DoorRegenTicks(tb testing.TB, multiplier, start float64) []Tick {
	tb.Helper()
	ticks := load().regen[regenKey{multiplier, start}]
	if len(ticks) == 0 {
		tb.Fatalf("vitals.golden has no regeneration line from HP %v at multiplier %v", start, multiplier)
	}
	return ticks
}

// RevivedHP is the HP a creature of maxHP has right after a plain revive.
func RevivedHP(tb testing.TB, maxHP int) float64 {
	tb.Helper()
	hp, ok := load().revive[maxHP]
	if !ok {
		tb.Fatalf("vitals.golden has no revive line for max HP %d", maxHP)
	}
	return hp
}

// PetMaxHP is a pet's maximum HP from its base maximum and constitution.
func PetMaxHP(tb testing.TB, base, con int) int {
	tb.Helper()
	hp, ok := load().petHP[[2]int{base, con}]
	if !ok {
		tb.Fatalf("vitals.golden has no pet max HP line for base %d, CON %d", base, con)
	}
	return hp
}

// PetExpRestored is the experience a resurrection of power percent gives a
// pet that had before experience before dying and has exp now.
func PetExpRestored(tb testing.TB, before, exp int64, power float64) int64 {
	tb.Helper()
	n, ok := load().petExp[petExpKey{before, exp, power}]
	if !ok {
		tb.Fatalf("vitals.golden has no pet exp line for %d -> %d at power %v", before, exp, power)
	}
	return n
}
