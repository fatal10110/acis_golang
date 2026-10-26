package monster

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/rs/zerolog"
)

// individual7 are the classes of the seventh individual behavior batch
// this package ports, in scripts.xml order, with the Go constructor of each.
var individual7 = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorHero", WarriorHero},
	{"script.ai.individual.Monster.LV3Monster.LV3Dagger", LV3Dagger},
	{"script.ai.individual.Monster.LV3Monster.LV3Dwarf", LV3Dwarf},
	{"script.ai.individual.Monster.LV3Monster.LV3Healer", LV3Healer},
	{"script.ai.individual.Monster.LV3Monster.LV3Knight", LV3Knight},
	{"script.ai.individual.Monster.LV3Monster.LV3Monster", LV3Monster},
	{"script.ai.individual.Monster.LV3Monster.LV3Orc", LV3Orc},
	{"script.ai.individual.Monster.LV3Monster.LV3PartyLeaderMonster", LV3PartyLeader},
	{"script.ai.individual.Monster.LV3Monster.LV3PartyPrivateMonster", LV3PartyPrivate},
	{"script.ai.individual.Monster.LV3Monster.LV3Ranger", LV3Ranger},
	{"script.ai.individual.Monster.LV3Monster.LV3SongDance", LV3SongDance},
	{"script.ai.individual.Monster.LV3Monster.LV3Summoner", LV3Summoner},
	{"script.ai.individual.Monster.LV3Monster.LV3Wizard", LV3Wizard},
	{"script.ai.individual.Monster.LV3Monster.LV3WizardPrivate", LV3WizardPrivate},
	{"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHold", WarriorDDMagicHold},
	{"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.ValakasPrivateLavasaurus", ValakasPrivateLavasaurus},
	{"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.WarriorDDMagicHoldAggressive", WarriorDDMagicHoldAggressive},
}

// individual7View is what a manifest-layout text records of the batch:
// per class its kind and bind lines, and the folded per-(npc, event) lists
// restricted to the batch, as "event id paths" triples.
type individual7View struct {
	kinds map[string]string
	binds map[string][]string
	folds []string
}

// individual7IDs reads a manifest id list: ids and a-b runs,
// comma-separated.
func individual7IDs(t *testing.T, s string) []int {
	t.Helper()
	var out []int
	for _, part := range strings.Split(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			t.Fatalf("bad id %q", part)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil {
				t.Fatalf("bad id %q", part)
			}
		}
		for id := a; id <= b; id++ {
			out = append(out, id)
		}
	}
	return out
}

// readIndividual7View reads a manifest-layout text (the reference manifest
// or a registry dump) as the batch sees it.
func readIndividual7View(t *testing.T, path string, ours map[string]bool, text *bufio.Scanner) individual7View {
	t.Helper()
	v := individual7View{kinds: map[string]string{}, binds: map[string][]string{}}
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
				for _, id := range individual7IDs(t, fields[4]) {
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

// TestIndividual7RegistrationMatchesReferenceManifest registers the batch
// after DefaultNpc and MonsterAI through the engine registry, with its own
// seam gate and every id a hostile NPC (the datapack types every bound id
// Monster). None is refused, and the registry dump equals the reference
// manifest for the batch: each class's kind and bind lines (LV3Monster and
// LV3PartyLeaderMonster bind none), and the folded per-(npc, event) lists
// restricted to the batch: 183 entries, the attacked and see-spell lists of
// the 35 LV3 ids among them, where a quest's hooks precede the behavior's.
func TestIndividual7RegistrationMatchesReferenceManifest(t *testing.T) {
	list := []script.Listing{{Path: "script.ai.individual.DefaultNpc"}, {Path: "script.ai.individual.Monster.MonsterAI"}}
	catalog := script.Catalog{"script.ai.individual.DefaultNpc": defaultnpc.New, "script.ai.individual.Monster.MonsterAI": New}
	ours := map[string]bool{}
	for _, c := range individual7 {
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
	got := readIndividual7View(t, "dump", ours, bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readIndividual7View(t, path, ours, sc)
	// ValakasPrivateLavasaurus adds 6 events on 29029.
	if got := len(want.folds); got != 183+6 {
		t.Fatalf("reference folds = %d entries, want %d", got, 183+6)
	}

	for _, c := range individual7 {
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
		t.Errorf("folded lists differ:\n got %q\nwant %q", got.folds, want.folds)
	}
	for _, id := range []int32{21260, 27299, 27259, 27266, 27258, 27294, 27261, 27253, 27289, 27269, 27313, 27250} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a batch class bound", id)
		}
	}
}
