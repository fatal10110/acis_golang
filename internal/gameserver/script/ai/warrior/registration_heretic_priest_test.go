package warrior

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/rs/zerolog"
)

// hereticPriestPath is WarriorHereticPriestAggressive's scripts.xml path.
const hereticPriestPath = "script.ai.individual.Monster.WarriorBase.Warrior.WarriorHereticPriestAggressive"

// TestHereticPriestRegistrationMatchesReferenceManifest registers
// WarriorHereticPriestAggressive after the behavior chain it derives from,
// through the engine registry with its own seam gate and every id a
// hostile NPC. It is not refused, and the registry dump equals the
// reference manifest for it: its kind and bind lines, and every folded
// per-(npc, event) list restricted to it, each of its 3 ids on the 10
// events of its hooks and the Warrior chain's dying hook.
func TestHereticPriestRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+1)
	catalog := script.Catalog{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	list = append(list, script.Listing{Path: hereticPriestPath})
	catalog[hereticPriestPath] = HereticPriestAggressive
	ours := map[string]bool{hereticPriestPath: true}
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
	if wantFolds, got := 3*11, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	if want.kinds[hereticPriestPath] == "" {
		t.Fatalf("%s is not in the reference manifest", hereticPriestPath)
	}
	if got.kinds[hereticPriestPath] != want.kinds[hereticPriestPath] {
		t.Errorf("kind = %q, want %q", got.kinds[hereticPriestPath], want.kinds[hereticPriestPath])
	}
	if !slices.Equal(got.binds[hereticPriestPath], want.binds[hereticPriestPath]) {
		t.Errorf("binds:\n got %q\nwant %q", got.binds[hereticPriestPath], want.binds[hereticPriestPath])
	}
	if !slices.Equal(got.folds, want.folds) {
		t.Errorf("folded lists differ:\n got %q\nwant %q", got.folds, want.folds)
	}
	for _, id := range []int32{22142, 22146, 22175} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
