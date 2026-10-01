package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from manor_test.go ----
type manorFakeTarget struct {
	neutralNPC
	world.Presence
	fakeActor
	dead  bool
	level int
	state *npc.SeedState
}

func (m *manorFakeTarget) Kind() actor.Kind          { return actor.KindNPC }
func (m *manorFakeTarget) Dead() bool                { return m.dead }
func (m *manorFakeTarget) Level() int                { return m.level }
func (m *manorFakeTarget) SeedState() *npc.SeedState { return m.state }

// sownState is a seed state already sown by sowerID, carrying a crop that
// matures into matureID.
func sownState(sowerID int32, matureID int) *npc.SeedState {
	state := &npc.SeedState{}
	state.Sow(sowerID, manor.Seed{MatureID: matureID})
	return state
}

type manorFakeItem struct {
	seed manor.Seed
	ok   bool
}

func (i manorFakeItem) Seed() (manor.Seed, bool) { return i.seed, i.ok }

func harvestRegistry() *Registry {
	return NewRegistry(harvestHandler{ids: &fakeSignetIDs{}})
}

func TestSowEventuallySucceedsAndMarksSeeded(t *testing.T) {
	// Seed/target/player levels all equal give a 90% sow success rate — not
	// a certainty, so the roll can't be forced deterministically. Retrying
	// drives the false-negative chance for this assertion to effectively
	// zero (0.1^300) without depending on a specific random outcome.
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	item := manorFakeItem{seed: manor.Seed{Level: 40, Alternative: false}, ok: true}

	for i := 0; i < 300; i++ {
		target := &manorFakeTarget{level: 40, state: &npc.SeedState{}}
		if !registry.Use(Cast{
			Caster:  caster,
			Item:    item,
			Skill:   modelskill.Definition{SkillType: "SOW"},
			Targets: []Actor{target},
		}) {
			t.Fatal("Use() returned false for SOW")
		}
		if target.state.Seeded() {
			if !target.state.AllowedToHarvest(7) {
				t.Fatal("sown state does not record the casting player as its sower")
			}
			return
		}
	}
	t.Fatal("SOW never succeeded in 300 attempts at a 90% success rate")
}

func TestSowAlreadySeededIsNoop(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: sownState(3, 0)}
	item := manorFakeItem{seed: manor.Seed{Level: 40}, ok: true}

	registry.Use(Cast{Caster: caster, Item: item, Skill: modelskill.Definition{SkillType: "SOW"}, Targets: []Actor{target}})
	if !target.state.AllowedToHarvest(3) {
		t.Fatal("already-seeded target should keep its original sower")
	}
}

func TestHarvestRewardsAllowedHarvester(t *testing.T) {
	registry := harvestRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: sownState(7, 5001)}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})

	if !target.state.Harvested() {
		t.Error("target should be marked harvested")
	}
	// Assert against the crop the seed state itself reports rather than a
	// literal, so this still fails if the handler stops threading the count
	// through and survives #240 changing what the count is.
	wantID, wantCount := target.state.HarvestedCrop()
	if wantID != 5001 {
		t.Fatalf("sown state crop id = %d, want 5001", wantID)
	}
	if caster.items[wantID] != wantCount {
		t.Fatalf("caster earned items = %v, want {%d: %d}", caster.items, wantID, wantCount)
	}
}

func TestHarvestDisallowedHarvesterGetsNothing(t *testing.T) {
	registry := harvestRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: sownState(3, 5001)}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})

	if target.state.Harvested() {
		t.Error("a disallowed harvester should not mark the target harvested")
	}
	if len(caster.items) != 0 {
		t.Fatalf("caster earned items = %v, want none", caster.items)
	}
}

func TestHarvestAlreadyHarvestedIsNoop(t *testing.T) {
	registry := harvestRegistry()
	caster := &manorFakeCaster{id: 7, level: 40}
	target := &manorFakeTarget{level: 40, state: sownState(7, 5001)}
	target.state.MarkHarvested()

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}})
	if len(caster.items) != 0 {
		t.Fatalf("caster earned items = %v, want none", caster.items)
	}
}
