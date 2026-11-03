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

// individual1 are the classes of the first individual behavior batch
// (#186) this package ports, in scripts.xml order, with the Go constructor
// of each.
var individual1 = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.CorePrivatePremo", CorePrivatePremo},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RamphoPrimeval", RamphoPrimeval},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushStrongMan1", RoyalRushStrongMan1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushStrongMan2", RoyalRushStrongMan2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushWarriorPhysicalSpecial2", RoyalRushWarriorPhysicalSpecial2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressiveImmediate", AggressiveImmediate},
	{"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast2DebuffsAggressive", Cast2DebuffsAggressive},
}

// readIndividual1View reads a manifest-layout text as the batch sees it:
// per script of ours its kind and bind lines, and the folded per-(npc,
// event) lists restricted to ours, as "event id paths" triples.
func readIndividual1View(t *testing.T, path string, ours map[string]bool, text *bufio.Scanner) manifestView {
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

// TestIndividual1RegistrationMatchesReferenceManifest registers the batch
// after the behavior chain it derives from, through the engine registry
// with its own seam gate and every id a hostile NPC (the datapack types all
// 64 ids Monster). None is refused, and the registry dump equals the
// reference manifest for the batch: each class's kind and bind lines, and
// every folded per-(npc, event) list restricted to the batch, each of the
// 64 ids on the 10 events of the Warrior chain.
func TestIndividual1RegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual1))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual1 {
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
	got := readIndividual1View(t, "dump", ours, bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readIndividual1View(t, path, ours, sc)
	if wantFolds, got := (1+1+4+4+6+47+1)*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual1 {
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
	for _, id := range []int32{29013, 22199, 18158, 18164, 18159, 18165, 18135, 21419, 27178, 27194, 21379} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
