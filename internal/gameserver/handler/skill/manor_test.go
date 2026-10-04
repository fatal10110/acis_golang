package skill

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Reference: Sow.java and Harvest.java. Only a player sows or harvests, only
// a Monster is a target, and each gate answers the caster with its own
// system message; a harvest takes the crop of the seed (its "id", not its
// matureId) times RateDropManor, and the sower's party may harvest too.

type manorFakeTarget struct {
	neutralNPC
	world.Presence
	fakeActor
	dead    bool
	level   int
	monster bool
	state   *npc.SeedState
}

func (m *manorFakeTarget) Kind() actor.Kind          { return actor.KindNPC }
func (m *manorFakeTarget) Dead() bool                { return m.dead }
func (m *manorFakeTarget) Level() int                { return m.level }
func (m *manorFakeTarget) SeedState() *npc.SeedState { return m.state }
func (m *manorFakeTarget) MonsterKind() bool         { return m.monster }

func freshMonster(level int) *manorFakeTarget {
	return &manorFakeTarget{level: level, monster: true, state: &npc.SeedState{}}
}

// manorFakePlayer is a player caster that earns items and knows which
// player ids share its party.
type manorFakePlayer struct {
	neutralPlayer
	world.Presence
	id    int32
	level int
	party []int32
	items map[int32]int
}

func (p *manorFakePlayer) ObjectID() int32 { return p.id }
func (p *manorFakePlayer) Level() int      { return p.level }
func (p *manorFakePlayer) Dead() bool      { return false }
func (p *manorFakePlayer) InPartyWith(id int32) bool {
	return slices.Contains(p.party, id)
}

func (p *manorFakePlayer) AddEarnedItem(itemID int32, count int, nextID func() (int32, error)) bool {
	if _, err := nextID(); err != nil {
		return false
	}
	if p.items == nil {
		p.items = make(map[int32]int)
	}
	p.items[itemID] += count
	return true
}

// darkCoda is seed 5016 (manors.xml: crop 5073, mature 5103, level 10).
var darkCoda = manor.Seed{CropID: 5073, SeedID: 5016, MatureID: 5103, Level: 10, CastleID: 1}

type manorFakeItem struct {
	seed manor.Seed
	ok   bool
}

func (i manorFakeItem) Seed() (manor.Seed, bool) { return i.seed, i.ok }

func sowCast(caster Creature, target Actor) Cast {
	return Cast{
		Caster:  caster,
		Item:    manorFakeItem{seed: darkCoda, ok: true},
		Skill:   modelskill.Definition{SkillType: "SOW"},
		Targets: []Actor{target},
	}
}

func harvestCast(caster Creature, target Actor) Cast {
	return Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "HARVEST"}, Targets: []Actor{target}}
}

func manorMessages(t *testing.T, r *Registry, cast Cast) []any {
	t.Helper()
	result, ok := r.UseResult(cast)
	if !ok {
		t.Fatalf("UseResult(%s) handled = false", cast.Skill.SkillType)
	}
	return result.Messages
}

func TestSowMarksSeededOrAnswersFailedRoll(t *testing.T) {
	// Seed, target and player levels all within reach give a 90% sow rate:
	// each attempt either sows and says so, or leaves the target unsown and
	// says so. Retrying reaches a sown target with near certainty.
	registry := NewDefaultRegistry()
	caster := &manorFakePlayer{id: 7, level: 10}
	for range 300 {
		target := freshMonster(10)
		got := manorMessages(t, registry, sowCast(caster, target))
		switch {
		case slices.Equal(got, []any{SeedSown}):
			if !target.state.Seeded() {
				t.Fatal("SeedSown reported on an unsown target")
			}
			if claim := target.state.ClaimHarvest(7, nil); claim != npc.HarvestClaimed {
				t.Fatalf("sower's own harvest claim = %v, want claimed", claim)
			}
			return
		case slices.Equal(got, []any{SeedNotSown}):
			if target.state.Seeded() {
				t.Fatal("SeedNotSown reported on a sown target")
			}
		default:
			t.Fatalf("sow messages = %v, want [SeedSown] or [SeedNotSown]", got)
		}
	}
	t.Fatal("SOW never succeeded in 300 attempts at a 90% success rate")
}

func TestSowAlreadySownTargetKeepsSower(t *testing.T) {
	registry := NewDefaultRegistry()
	target := freshMonster(10)
	target.state.Sow(3, darkCoda)

	got := manorMessages(t, registry, sowCast(&manorFakePlayer{id: 7, level: 10}, target))
	if !slices.Equal(got, []any{SeedAlreadySown}) {
		t.Fatalf("messages = %v, want [SeedAlreadySown]", got)
	}
	if claim := target.state.ClaimHarvest(3, nil); claim != npc.HarvestClaimed {
		t.Fatalf("original sower's claim = %v, want claimed", claim)
	}
}

func TestSowSilentGates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		caster Creature
		target *manorFakeTarget
		item   any
	}{
		{"non-player caster", &manorFakeCaster{id: 7, level: 10}, freshMonster(10), manorFakeItem{seed: darkCoda, ok: true}},
		{"non-monster target", &manorFakePlayer{id: 7, level: 10}, &manorFakeTarget{level: 10, state: &npc.SeedState{}}, manorFakeItem{seed: darkCoda, ok: true}},
		{"dead target", &manorFakePlayer{id: 7, level: 10}, &manorFakeTarget{level: 10, monster: true, dead: true, state: &npc.SeedState{}}, manorFakeItem{seed: darkCoda, ok: true}},
		{"no seed row", &manorFakePlayer{id: 7, level: 10}, freshMonster(10), manorFakeItem{}},
		{"no item", &manorFakePlayer{id: 7, level: 10}, freshMonster(10), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cast := sowCast(tc.caster, tc.target)
			cast.Item = tc.item
			result, _ := NewDefaultRegistry().UseResult(cast)
			if len(result.Messages) != 0 || tc.target.state.Seeded() {
				t.Fatalf("messages = %v, seeded = %v; want silent and unsown", result.Messages, tc.target.state.Seeded())
			}
		})
	}
}

func TestHarvestGates(t *testing.T) {
	sownBy := func(sowerID int32) *manorFakeTarget {
		target := freshMonster(10)
		target.state.Sow(sowerID, darkCoda)
		return target
	}
	for _, tc := range []struct {
		name      string
		target    *manorFakeTarget
		party     []int32
		want      []any
		wantCrops int
	}{
		{"non-monster", &manorFakeTarget{level: 10, state: &npc.SeedState{}}, nil, []any{HarvestTargetNotSown}, 0},
		{"never sown", freshMonster(10), nil, []any{HarvestTargetNotSown}, 0},
		{"already harvested", func() *manorFakeTarget {
			target := sownBy(7)
			target.state.ClaimHarvest(7, nil)
			return target
		}(), nil, []any{HarvestFailed}, 0},
		{"stranger's crop", sownBy(3), nil, []any{HarvestNotAuthorized}, 0},
		{"own crop", sownBy(7), nil, []any{CropHarvested{CropID: 5073, Count: 3}}, 3},
		{"party member's crop", sownBy(3), []int32{3}, []any{CropHarvested{CropID: 5073, Count: 3}}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// cropRate 3 stands in for RateDropManor; the levels are within
			// five, so the harvest roll always succeeds.
			registry := NewRegistry(harvestHandler{ids: &fakeSignetIDs{}, cropRate: 3})
			caster := &manorFakePlayer{id: 7, level: 10, party: tc.party}
			got := manorMessages(t, registry, harvestCast(caster, tc.target))
			if !slices.Equal(got, tc.want) {
				t.Fatalf("messages = %v, want %v", got, tc.want)
			}
			if caster.items[5073] != tc.wantCrops || len(caster.items) > 1 {
				t.Fatalf("earned = %v, want %d of crop 5073", caster.items, tc.wantCrops)
			}
		})
	}
}

func TestHarvestIgnoresNonPlayerCaster(t *testing.T) {
	target := freshMonster(10)
	target.state.Sow(7, darkCoda)
	registry := NewRegistry(harvestHandler{ids: &fakeSignetIDs{}, cropRate: 1})
	result, _ := registry.UseResult(harvestCast(&manorFakeCaster{id: 7, level: 10}, target))
	if len(result.Messages) != 0 {
		t.Fatalf("messages = %v, want none", result.Messages)
	}
	if claim := target.state.ClaimHarvest(7, nil); claim != npc.HarvestClaimed {
		t.Fatalf("crop after a non-player harvest: claim = %v, want still claimable", claim)
	}
}
