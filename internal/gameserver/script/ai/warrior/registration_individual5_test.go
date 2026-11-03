package warrior

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/rs/zerolog"
)

// individual5 are the classes of the fifth individual behavior batch this
// package ports (chests, PartyLeaderSplit and the berserkers), in
// scripts.xml order, with the Go constructor of each.
var individual5 = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Chests.TreasureChest", TreasureChest},
	{"script.ai.individual.Monster.WarriorBase.Chests.TreasureChestMimic", TreasureChestMimic},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderSplit", PartyLeaderSplit},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderSplitAggressive", PartyLeaderSplitAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserker", Berserker},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecial", BerserkerPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecialAggressive", BerserkerPhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorBerserkerPhysicalSpecialStoneAggressive", BerserkerPhysicalSpecialStoneAggressive},
}

// readBatchView reads a manifest-layout text as the batch sees it: per
// script of ours its kind and bind lines, and the folded per-(npc, event)
// lists restricted to ours, as "event id paths" triples.
func readBatchView(t *testing.T, path string, ours map[string]bool, text *bufio.Scanner) manifestView {
	t.Helper()
	v := manifestView{kinds: map[string]string{}, binds: map[string][]string{}}
	var cur string
	for text.Scan() {
		line := text.Text()
		fields := strings.Fields(line)
		switch {
		case strings.HasPrefix(line, "script "):
			cur = fields[1]
		case strings.HasPrefix(line, "  kind ") && ours[cur]:
			v.kinds[cur] = fields[1]
		case strings.HasPrefix(line, "  bind ") && ours[cur]:
			v.binds[cur] = append(v.binds[cur], strings.TrimSpace(line))
		case strings.HasPrefix(line, "fold npc "):
			var kept []string
			for _, p := range strings.Split(fields[3], ",") {
				if ours[p] {
					kept = append(kept, p)
				}
			}
			if len(kept) == 0 {
				continue
			}
			for _, ev := range strings.Split(fields[2], ",") {
				for _, id := range expandIDs(t, fields[4]) {
					v.folds = append(v.folds, ev+" "+strconv.Itoa(id)+" "+strings.Join(kept, ","))
				}
			}
		}
	}
	if err := text.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	slices.Sort(v.folds)
	return v
}

// TestIndividual5RegistrationMatchesReferenceManifest registers the batch
// after the behavior chain it derives from, through the engine registry
// with its own seam gate and every id a hostile NPC (the datapack types
// the 63 chests Chest, the rest Monster). None is refused, and the
// registry dump equals the reference manifest for the batch: each class's
// kind and bind lines, and every folded per-(npc, event) list restricted
// to the batch: the 34 + 29 chest ids on 8 events each, the 2 split ids on
// 10, and the 16 + 1 + 2 + 10 berserker ids on 11.
func TestIndividual5RegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual5))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual5 {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
		ours[c.path] = true
	}
	hostile := func(int32) (script.NPCKind, bool) { return script.KindHostile, true }
	r := script.Build(list, catalog, script.Config{KindOf: hostile, Log: zerolog.Nop()})
	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	got := readBatchView(t, "dump", ours, bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readBatchView(t, path, ours, sc)
	if wantFolds, got := (34+29)*8+2*10+(16+1+2+10)*11, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual5 {
		if want.kinds[c.path] == "" {
			t.Fatalf("%s is not in the reference manifest", c.path)
		}
		if got.kinds[c.path] != want.kinds[c.path] {
			t.Errorf("%s kind = %q, want %q", c.path, got.kinds[c.path], want.kinds[c.path])
		}
		if !slices.Equal(got.binds[c.path], want.binds[c.path]) {
			t.Errorf("%s binds:\n got %q\nwant %q", c.path, got.binds[c.path], want.binds[c.path])
		}
	}
	if !slices.Equal(got.folds, want.folds) {
		t.Errorf("folded lists differ: %d entries, reference %d", len(got.folds), len(want.folds))
	}
	for _, id := range []int32{18265, 21671, 22088, 22094, 20086, 21410, 21382, 21327} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
