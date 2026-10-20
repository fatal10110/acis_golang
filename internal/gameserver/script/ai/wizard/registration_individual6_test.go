package wizard

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

// individual6 are the classes of the sixth individual behavior batch this
// package ports, in scripts.xml order, with the Go constructor of each.
var individual6 = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.Elcroki", Elcroki},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizard", PartyLeader},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardCastRangeCurseStoneAggressive", PartyLeaderCastRangeCurseStoneAggressive},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardCorpseNecroAggressive", PartyLeaderCorpseNecroAggressive},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2", PartyLeaderDD2},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2CurseAggressive", PartyLeaderDD2CurseAggressive},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardDD2Summon", PartyLeaderDD2Summon},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardHereticAggressive", PartyLeaderHereticAggressive},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardRangeCurseAggressive", PartyLeaderRangeCurseAggressive},
	{"script.ai.individual.Monster.WizardBase.PartyLeaderWizard.PartyLeaderWizardSaintAggressive", PartyLeaderSaintAggressive},
	{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizard", PartyPrivate},
	{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2", PartyPrivateDD2},
	{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2Curse", PartyPrivateDD2Curse},
}

// readIndividual6View reads a manifest-layout text as the batch sees it:
// per class of ours its kind and bind lines, and the folded per-(npc,
// event) lists restricted to ours, as "event id paths" triples.
func readIndividual6View(t *testing.T, path string, ours map[string]bool, text *bufio.Scanner) manifestView {
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

// TestIndividual6RegistrationMatchesReferenceManifest registers the batch
// after the behavior chain it derives from, through the engine registry
// with its own seam gate and every id a hostile NPC (the datapack types
// every id of the batch Monster). None is refused, and the registry dump
// equals the reference manifest for the batch: each class's kind and bind
// lines (PartyLeaderWizard, PartyLeaderWizardDD2 and PartyPrivateWizard
// bind none), and the folded per-(npc, event) lists restricted to the
// batch: Elcroki's id on
// its 12 events, the 11 events of each id of PartyLeaderWizardDD2's
// children but PartyLeaderWizardHereticAggressive (2 + 2 + 1 + 4 + 1 ids),
// and that class's 2 ids on 13, PartyLeaderWizardSaintAggressive's 2 ids on
// 12, and the 11 events of each id of PartyPrivateWizardDD2 and its child
// (2 + 1 ids; PartyPrivateWizard binds none).
func TestIndividual6RegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual6))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual6 {
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
	got := readIndividual6View(t, "dump", ours, bufio.NewScanner(strings.NewReader(dump.String())))

	path := filepath.Join("..", "..", "testdata", "oracle", "manifest.golden")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	want := readIndividual6View(t, path, ours, sc)
	if got := len(want.folds); got != 12+10*11+2*13+2*12+3*11 {
		t.Fatalf("reference folds = %d entries, want 205", got)
	}

	for _, c := range individual6 {
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
	for _, id := range []int32{22214, 21347, 21373, 21596, 21599, 21090, 22163, 22171, 22033, 22035, 22041, 22043, 21312, 21544, 21541, 21542, 21545, 20972} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want its behavior bound", id)
		}
	}
}
