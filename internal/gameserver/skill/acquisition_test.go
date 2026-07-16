package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestValidAcquireRequest(t *testing.T) {
	if !ValidAcquireRequest(3, 1) {
		t.Fatal("ValidAcquireRequest(3, 1) = false")
	}
	if ValidAcquireRequest(0, 1) {
		t.Fatal("ValidAcquireRequest(0, 1) = true")
	}
	if ValidAcquireRequest(3, 0) {
		t.Fatal("ValidAcquireRequest(3, 0) = true")
	}
}

func TestLearnableGeneralRequiresNextLevelAndCost(t *testing.T) {
	tmpl := &player.Template{Skills: []player.SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 100},
		{SkillID: 4, Level: 1, MinLevel: 5, Cost: 0},
	}}

	grant, ok := LearnableGeneral(tmpl, 5, player.SkillLevels{}, 3, 1)
	if !ok {
		t.Fatal("LearnableGeneral returned ok=false")
	}
	if grant.SkillID != 3 || grant.Level != 1 {
		t.Fatalf("grant = %+v, want skill 3 level 1", grant)
	}
	if _, ok := LearnableGeneral(tmpl, 4, player.SkillLevels{}, 4, 1); ok {
		t.Fatal("zero-cost skill was learnable")
	}
	if _, ok := LearnableGeneral(tmpl, 5, player.SkillLevels{3: 1}, 3, 1); ok {
		t.Fatal("already-known skill level was learnable")
	}
}

func TestLearnableFishingRequiresLoadedDefinition(t *testing.T) {
	trees := &modelskill.Trees{Fishing: []modelskill.FishingSkill{
		{ID: 1368, Level: 1, MinLevel: 1, ItemID: 57, ItemCount: 1},
	}}
	loaded := func(skillID, level int) bool { return skillID == 1368 && level == 1 }

	node, ok := LearnableFishing(trees, 1, false, player.SkillLevels{}, loaded, 1368, 1)
	if !ok {
		t.Fatal("LearnableFishing returned ok=false")
	}
	if node.ID != 1368 || node.Level != 1 {
		t.Fatalf("node = %+v, want skill 1368 level 1", node)
	}
	if _, ok := LearnableFishing(trees, 1, false, player.SkillLevels{}, nil, 1368, 1); ok {
		t.Fatal("missing definition lookup was learnable")
	}
}

func TestNeedsStorageSync(t *testing.T) {
	for _, id := range []int32{1368, 1372} {
		if !NeedsStorageSync(id) {
			t.Fatalf("NeedsStorageSync(%d) = false", id)
		}
	}
	for _, id := range []int32{1367, 1373} {
		if NeedsStorageSync(id) {
			t.Fatalf("NeedsStorageSync(%d) = true", id)
		}
	}
}
