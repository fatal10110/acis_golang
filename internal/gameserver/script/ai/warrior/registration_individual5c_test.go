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

// individual5c are the classes of the fifth individual behavior batch's
// third PR (the heretic bombs, the Four Sepulchers room bosses and the
// tyrannosaurus), in scripts.xml order, with the Go constructor of each.
var individual5c = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb", RoyalRushHereticBomb},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb2", RoyalRushHereticBomb2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss1", RoyalRushRoomBoss1},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss2", RoyalRushRoomBoss2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss4", RoyalRushRoomBoss4},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss5", RoyalRushRoomBoss5},
	{"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBossBasic", RoyalRushRoomBossBasic},
	{"script.ai.individual.Monster.WarriorBase.Warrior.TyrannoPrimeval.TyrannoPrimeval", TyrannoPrimeval},
}

// TestIndividual5cRegistrationMatchesReferenceManifest registers the third
// PR's classes after the behavior chain they derive from, through the
// engine registry with its own seam gate and every id a hostile NPC. None
// is refused, and the registry dump equals the reference manifest for
// them: each class's kind and bind lines (RoyalRushRoomBossBasic has no
// ids and binds nothing), and every folded per-(npc, event) list
// restricted to them: the 2 + 1 heretic bomb ids on 11 events (out of
// territory added), the 8 + 4 ids of RoyalRushRoomBoss1 and 2 on 11 (clan
// died added), the 16 + 8 of RoyalRushRoomBoss4 and 5 on the Warrior
// chain's 10, and the 2 tyrannosaurus ids on 11 (abnormal status changed
// added).
func TestIndividual5cRegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual5c))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual5c {
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
	if wantFolds, got := (2+1+8+4+2)*11+(16+8)*10, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual5c {
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
	for _, id := range []int32{22145, 22195, 18121, 18120, 18141, 18122, 22215, 22216} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
