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
	{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardDD2Heal", PartyPrivateDD2Heal},
	{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticCoward", PartyPrivateHereticCoward},
	{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticDD2", PartyPrivateHereticDD2},
	{"script.ai.individual.Monster.WizardBase.PartyPrivateWizard.PartyPrivateWizardHereticDD2Curse", PartyPrivateHereticDD2Curse},
	{"script.ai.individual.Monster.WizardBase.Wizard.Agit02VampireArcherAgit", Agit02VampireArcherAgit},
	{"script.ai.individual.Monster.WizardBase.Wizard.DoormanOfDrain", DoormanOfDrain},
	{"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushHealer1", RoyalRushHealer1},
	{"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushHealer2", RoyalRushHealer2},
	{"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushWizardDDMagic2Hold", RoyalRushDDMagic2Hold},
	{"script.ai.individual.Monster.WizardBase.Wizard.RoyalRushWizardShortSelfRange", RoyalRushShortSelfRange},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.RoyalRushWizardClanBuff", RoyalRushClanBuff},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.RoyalRushWizardRangeDebuff", RoyalRushRangeDebuff},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2", DDMagic2},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffCurseStoneAggressive", CastClanBuffCurseStoneAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffRangeCurse", CastClanBuffRangeCurse},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCastClanBuffRangeDDCurseStoneAggressive", CastClanBuffRangeDDCurseStoneAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Aggressive", DDMagic2Aggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2DispellAggressive", DDMagic2DispellAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHallDDMagic2Aggressive", HallDDMagic2Aggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Curse", DDMagic2Curse},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2CurseAggressive", DDMagic2CurseAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardGrowth.WizardGrowthStep2", GrowthStep2},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardGrowth.WizardGrowthStep3Aggressive", GrowthStep3Aggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardGrowth.WizardGrowthStep4Aggressive", GrowthStep4Aggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagic2Curse", HereticDDMagic2Curse},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagic2CurseAggressive", HereticDDMagic2CurseAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseNecro.WizardCorpseNecro", CorpseNecro},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseNecro.WizardCorpseNecroAggressive", CorpseNecroAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseNecro.WizardCorpseNecroSummon", CorpseNecroSummon},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseNecro.WizardCorpseNecroSummonAggressive", CorpseNecroSummonAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireBasic", CorpseVampireBasic},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireBasicAggressive", CorpseVampireBasicAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireCurseAggressive", CorpseVampireCurseAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireRange", CorpseVampireRange},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireRangeAggressive", CorpseVampireRangeAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardCorpseVampireBasic.WizardCorpseVampireTeleport", CorpseVampireTeleport},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Heal", DDMagic2Heal},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2HealAggressive", DDMagic2HealAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardDDMagic2Solina", DDMagic2Solina},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHealer", Healer},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHealerAggressive", HealerAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagic2Aggressive", HereticDDMagic2Aggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardDDMagic2.WizardHereticDDMagicCurse", HereticDDMagicCurse},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcher.WizardFiendArcher", FiendArcher},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcher.WizardFiendArcherAggressive", FiendArcherAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcher.WizardFiendArcherSelfBuff", FiendArcherSelfBuff},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcher.WizardFiendArcherSelfBuffAggressive", FiendArcherSelfBuffAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardFiendArcherRunAway", FiendArcherRunAway},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardHereticPriestAggressive", HereticPriestAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic.WizardSaintBasic", SaintBasic},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic.WizardSaintBasicAggressive", SaintBasicAggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic.WizardSaintDDMagic2.WizardSaintDDMagic2", SaintDDMagic2},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic.WizardSaintDDMagic2.WizardSaintRangeDDMagic2Aggressive", SaintRangeDDMagic2Aggressive},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic.WizardSaintSelfRangeDDMagic.WizardSaintSelfRangeDDMagic", SaintSelfRangeDDMagic},
	{"script.ai.individual.Monster.WizardBase.Wizard.WizardSaintBasic.WizardSaintSelfRangeDDMagic.WizardSaintSelfRangeDDMagicAggressive", SaintSelfRangeDDMagicAggressive},
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
// (2 + 1 ids; PartyPrivateWizard binds none), the 9 events of each id of
// WizardDDMagic2 and its two royal rush children (35 + 6 + 3 ids), the 10
// of WizardHereticPriestAggressive's 2 ids, and the 46 dying entries the
// new classes share with other scripts. The 10 events of each id of
// WizardDDMagic2Curse, its aggressive child and the two growth steps (13 +
// 14 + 12 + 6 ids) and the 11 of each id of WizardHereticDDMagic2Curse and
// its child (2 + 3 ids), the 10 of each id of WizardDDMagic2Heal, its two
// children and WizardCorpseNecro (10 + 2 + 1 + 4 ids) and the 11 of each id
// of WizardHealer, its child, WizardHereticDDMagic2Aggressive and
// WizardHereticDDMagicCurse (2 + 1 + 2 + 1 ids), and the 10 of each id of
// WizardCorpseNecro's three children and WizardCorpseVampireBasic's
// children (3 + 4 + 2 + 3 + 1 + 1 + 1 ids; WizardCorpseVampireBasicAggressive
// binds none), and the 10 of each id of WizardCorpseVampireTeleport,
// WizardGrowthStep2, WizardFiendArcherAggressive,
// WizardFiendArcherSelfBuffAggressive, WizardSaintBasic and its child (2 +
// 12 + 6 + 1 + 5 + 1 ids; WizardFiendArcher and WizardFiendArcherSelfBuff
// bind none), the 10 of each id of WizardSaintDDMagic2 and its child (2 + 2
// ids) and the 11 of each id of WizardSaintSelfRangeDDMagic and its child
// (1 + 2 ids) close the count.
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
	if got := len(want.folds); got != 12+10*11+2*13+2*12+6*11+3*13+4*12+11+2*11+3*10+2*10+4*10+4*10+2*11+6*9+3*9+35*9+46+2*10+49*10+45*10+5*11+17*10+6*11+15*10+2*10+12*10+6*10+10+5*10+10+4*10+1*11+2*11 {
		t.Fatalf("reference folds = %d entries, want 2696", got)
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
	for _, id := range []int32{22214, 21347, 21373, 21596, 21599, 21090, 22163, 22171, 22033, 22035, 22041, 22043, 21312, 21544, 21541, 21542, 21545, 20972, 21543, 21546, 21823, 22158, 22162, 22174, 22166, 22165, 22168, 22173, 35634, 18343, 22136, 22143, 22151, 20092, 18147, 18138, 21404, 18139, 18228, 21581, 21563, 21559, 21576, 21585, 21591, 21586, 21588, 21453, 21497, 20819, 21388, 21521, 21526, 21537, 21532, 21650, 21535} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want its behavior bound", id)
		}
	}
}
