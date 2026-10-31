package castlemanor

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

// manorGolden reads testdata/manor.golden, the output of testdata/oracle/run.sh:
// the reference's own castle manor code, run on a clock the probe sets.
func manorGolden(t *testing.T, kind string) [][]string {
	t.Helper()
	f, err := os.Open("testdata/manor.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out [][]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, rest, _ := strings.Cut(sc.Text(), " "); k == kind {
			before, after, _ := strings.Cut(rest, " => ")
			out = append(out, []string{before, after})
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatalf("golden has no %q lines", kind)
	}
	return out
}

func statusNamed(t *testing.T, name string) Status {
	t.Helper()
	for _, s := range []Status{StatusModifiable, StatusMaintenance, StatusApproved} {
		if s.String() == name {
			return s
		}
	}
	t.Fatalf("unknown status %q", name)
	return 0
}

// TestStartModeMatchesReferenceProbe replays "mode <hour> <minute>:<mode>...",
// the mode a manor starts in by minute of each hour.
func TestStartModeMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	for _, l := range manorGolden(t, "mode") {
		f := strings.Fields(l[0])
		hour, runs := must(strconv.Atoi(f[0])), f[1:]
		for minute := range 60 {
			var want string
			for _, r := range runs {
				from, name, _ := strings.Cut(r, ":")
				if must(strconv.Atoi(from)) <= minute {
					want = name
				}
			}
			if got := statusAt(DefaultConfig(), at(hour, minute)); got.String() != want {
				t.Errorf("statusAt(%02d:%02d) = %s, want %s", hour, minute, got, want)
			}
		}
	}
}

// TestNextChangeMatchesReferenceProbe replays "next <mode> <clock> <ms>",
// the time from now until a manor in that mode changes it.
func TestNextChangeMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	for _, l := range manorGolden(t, "next") {
		f := strings.Fields(l[0])
		var hour, minute, second, ms int
		if _, err := fmt.Sscanf(f[1], "%d:%d:%d.%d", &hour, &minute, &second, &ms); err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 10, 5, hour, minute, second, ms*int(time.Millisecond), time.UTC)
		want := time.Duration(must(strconv.ParseInt(f[2], 10, 64))) * time.Millisecond
		if got := nextChangeAt(DefaultConfig(), statusNamed(t, f[0]), now).Sub(now); got != want {
			t.Errorf("%s manor at %s changes in %v, want %v", f[0], f[1], got, want)
		}
	}
}

// TestManorCostMatchesReferenceProbe replays "cost <seeds>|<crops> => <cost>":
// seeds as reference price x start amount ("none" for a seed the table does
// not hold), crops as price x start amount.
func TestManorCostMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	for _, l := range manorGolden(t, "cost") {
		seedSpec, cropSpec, _ := strings.Cut(l[0], "|")
		prices := map[int32]int32{}
		var seeds []manor.Seed
		var production []*Production
		var procure []*Procure
		if seedSpec != "" {
			for n, s := range strings.Split(seedSpec, ",") {
				price, start, _ := strings.Cut(s, "x")
				id := int32(7000 + n)
				if price != "none " {
					prices[id] = int32(must(strconv.ParseInt(price, 10, 32)))
					seeds = append(seeds, manor.Seed{CropID: int(id) + 1000, SeedID: int(id), MatureID: int(id) + 2000, CastleID: gludio})
				}
				amount := int32(must(strconv.ParseInt(start, 10, 32)))
				production = append(production, NewProduction(id, amount, 1, amount))
			}
		}
		if cropSpec != "" {
			for _, c := range strings.Split(cropSpec, ",") {
				price, start, _ := strings.Cut(c, "x")
				amount := int32(must(strconv.ParseInt(start, 10, 32)))
				procure = append(procure, NewProcure(codaCrop, amount, int32(must(strconv.ParseInt(price, 10, 32))), amount, 1))
			}
		}
		table := manor.NewTable([]manor.Manor{{ID: gludio, Name: "gludio", Seeds: seeds}})
		table.ApplyReferencePrices(func(id int32) (int32, bool) { p, ok := prices[id]; return p, ok })
		cfg := DefaultConfig()
		cfg.SavePeriod = 0
		h := startOver(t, cfg, table, at(12, 0), 0, &memStore{}, 0)
		h.m.SetNextSeedProduction(gludio, production)
		h.m.SetNextCropProcure(gludio, procure)
		if got, want := h.m.ManorCost(gludio, true), must(strconv.ParseInt(l[1], 10, 64)); got != want {
			t.Errorf("ManorCost of %q = %d, want %d", l[0], got, want)
		}
	}
}

// TestRolloverMatchesReferenceProbe replays "rollover <crops> <treasury>
// <roll> => warehouse <item:count,...> income <n> next <empty|lists> running
// <seeds> | <crops> mode <mode> delay <ms>": a manor rolling its period over
// at 20:00. The crops are start/left/price of what the running period buys,
// alternating Dark Coda and Red Coda.
func TestRolloverMatchesReferenceProbe(t *testing.T) {
	t.Parallel()
	for _, l := range manorGolden(t, "rollover") {
		f := strings.Fields(l[0])
		treasury, roll := int64(must(strconv.Atoi(f[1]))), must(strconv.Atoi(f[2]))
		store := &memStore{
			production: []ProductionRow{
				{CastleID: gludio, SeedID: redSeed, Amount: 1, StartAmount: 3, Price: 9},
				{CastleID: gludio, SeedID: codaSeed, Amount: 4, StartAmount: 10, Price: 5, NextPeriod: true},
			},
			procure: []ProcureRow{{CastleID: gludio, CropID: codaCrop, Amount: 70, StartAmount: 100, Price: 30, RewardType: 1, NextPeriod: true}},
		}
		for i, c := range strings.Split(f[0], "+") {
			v := strings.Split(c, "/")
			crop := codaCrop
			if i%2 == 1 {
				crop = redCrop
			}
			store.procure = append(store.procure, ProcureRow{
				CastleID: gludio, CropID: crop, StartAmount: int32(must(strconv.Atoi(v[0]))),
				Amount: int32(must(strconv.Atoi(v[1]))), Price: int32(must(strconv.Atoi(v[2]))), RewardType: 1,
			})
		}
		h := start(t, at(19, 0), treasury, store, roll)
		h.clock.Advance(time.Hour)

		var items []string
		for _, e := range h.effects.take() {
			var clanID, count, item int
			if _, err := fmt.Sscanf(e, "warehouse 0x%x +%d of %d", &clanID, &count, &item); err != nil {
				t.Fatalf("effect %q: %v", e, err)
			}
			items = append(items, fmt.Sprintf("%d:%d", item, count))
		}
		warehouse := "-"
		if len(items) > 0 {
			warehouse = strings.Join(items, ",")
		}
		gl, _ := h.castles.Get(gludio)
		next := "empty"
		if seeds, crops := h.m.SeedProduction(gludio, true), h.m.CropProcure(gludio, true); len(seeds)+len(crops) > 0 {
			next = ""
			for _, s := range seeds {
				next += fmt.Sprintf("%d:%d,", s.ID, s.Amount())
			}
			for _, c := range crops {
				next += fmt.Sprintf("%d:%d,", c.ID, c.Amount())
			}
		}
		var running string
		for _, s := range h.m.SeedProduction(gludio, false) {
			running += fmt.Sprintf(" %d:%d", s.ID, s.Amount())
		}
		running += " |"
		for _, c := range h.m.CropProcure(gludio, false) {
			running += fmt.Sprintf(" %d:%d", c.ID, c.Amount())
		}
		got := fmt.Sprintf("warehouse %s income %d next %s running%s mode %s delay %d", warehouse, gl.SeedIncome(), next, running, h.m.Status(), h.m.NextModeChange().Sub(h.clock.Now()).Milliseconds())
		if got != l[1] {
			t.Errorf("rollover of %s at treasury %d, roll %d:\n got %s\nwant %s", f[0], treasury, roll, got, l[1])
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
