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

// individual3 are the classes of the third individual behavior batch
// (#188) this package ports, in scripts.xml order, with the Go constructor
// of each.
var individual3 = []struct {
	path string
	ctor func() script.Script
}{
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaGardener", FreyaGardener},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaPet", FreyaPet},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaSecretaryLeader", FreyaSecretaryLeader},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderBombSpawn", PartyLeaderBombSpawn},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCannibaliAggressiva", PartyLeaderCannibaliAggressiva},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastCurseAggressive", PartyLeaderCastCurseAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDD", PartyLeaderCastDD},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDAggressive", PartyLeaderCastDDAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDMagicStoneAggressive", PartyLeaderCastDDMagicStoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCouplePhysicalSpecial", PartyLeaderCouplePhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrol", PartyLeaderPatrol},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial", PartyLeaderPhysicalSpecial},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialAggressive", PartyLeaderPhysicalSpecialAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummon", PartyLeaderPhysicalSpecialSummon},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummonAggressive", PartyLeaderPhysicalSpecialSummonAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarriorAggressive", PartyLeaderWarriorAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastSplashAggressive", PartyLeaderCastSplashAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCasting3SkillsMagicalAggressive.AntharasPrivateBehemothDragon", AntharasPrivateBehemothDragon},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCasting3SkillsMagicalAggressive.PartyLeaderCasting3SkillsMagicalAggressive", PartyLeaderCasting3SkillsMagicalAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPabelgolemAggressive", PartyLeaderPabelgolemAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrolPhysicalSpecialStoneAggressive", PartyLeaderPatrolPhysicalSpecialStoneAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderSlowTypeAggressive", PartyLeaderSlowTypeAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.QuestPartyLeader", QuestPartyLeader},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.RoyalRushPartyLeader", RoyalRushPartyLeader},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptain", PartyLeaderCoupleCaptain},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainMaleAggressive", PartyLeaderCoupleCaptainMaleAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainNurseAggressive", PartyLeaderCoupleCaptainNurseAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderHeretic2Aggressive", PartyLeaderHeretic2Aggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderHereticAggressive", PartyLeaderHereticAggressive},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderPhysicalSpecial2", PartyLeaderPhysicalSpecial2},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PrimevalRoamer.PrimevalRoamer", PrimevalRoamer},
	{"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PrimevalRoamer.PrimevalRoamerWalker", PrimevalRoamerWalker},
}

// TestIndividual3RegistrationMatchesReferenceManifest registers the batch
// after the behavior chain it derives from, through the engine registry
// with its own seam gate and every id a hostile NPC (the datapack types all
// 91 ids Monster). None is refused, and the registry dump equals the
// reference manifest for the batch: each class's kind and bind lines (none
// for PartyLeaderCastDD and PartyLeaderCasting3SkillsMagicalAggressive,
// which have no NPC of their own), and every folded
// per-(npc, event) list restricted to the batch, each of the 91 ids on the
// 12 events of the PartyLeaderWarrior chain, and the four heretic leaders
// on their out-of-territory event too.
func TestIndividual3RegistrationMatchesReferenceManifest(t *testing.T) {
	list := make([]script.Listing, 0, len(chain)+len(individual3))
	catalog := script.Catalog{}
	ours := map[string]bool{}
	for _, c := range chain {
		list = append(list, script.Listing{Path: c.path})
		catalog[c.path] = c.ctor
	}
	for _, c := range individual3 {
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
	if wantFolds, got := (2+9+2+1+1+6+20+1+3+3+1+1+2+1+1+5+1+0+2+2+1+4+1+1+1+3+1+10+5)*12+3+1, len(want.folds); got != wantFolds {
		t.Fatalf("reference folds = %d entries, want %d", got, wantFolds)
	}

	for _, c := range individual3 {
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
	for _, id := range []int32{22109, 22118, 22155, 22159, 22167, 22188, 22200, 22227, 22205, 22213, 21517, 21512, 20963, 22028, 21345, 21371, 21432, 27041, 20947, 22096, 20983, 22135, 22029, 22032, 22038, 22039, 20376, 20520, 20522, 22100, 22104, 22102, 22113, 20773, 21081, 21058, 20966, 20973, 21078, 20986, 22060, 22074, 21343, 21369, 22092, 27185, 27186, 27187, 27188, 21434, 29069} {
		if !r.Behaves(id) {
			t.Errorf("Behaves(%d) = false, want a behavior bound", id)
		}
	}
}
