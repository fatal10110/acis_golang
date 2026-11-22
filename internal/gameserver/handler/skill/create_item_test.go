package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func createItemRegistry() *Registry {
	return NewRegistry(createItemHandler{ids: &fakeSignetIDs{}})
}

// TestCreateItemWithoutProductReportsPreparedForReuse pins the guard on a
// CREATE_ITEM skill that names no item or a zero count: nothing is
// created and the skill is reported ready for reuse, once, with its id and
// level.
func TestCreateItemWithoutProductReportsPreparedForReuse(t *testing.T) {
	for name, def := range map[string]modelskill.Definition{
		"no ids":     modelskill.Definition{ID: 2104, Level: 3, SkillType: "CREATE_ITEM"}.WithExtra(modelskill.Extra{CreateItemCount: 1000, CreateItemRandomCount: 1}),
		"zero count": modelskill.Definition{ID: 2104, Level: 3, SkillType: "CREATE_ITEM"}.WithExtra(modelskill.Extra{CreateItemIDs: []int32{1835}, CreateItemRandomCount: 1}),
	} {
		t.Run(name, func(t *testing.T) {
			caster := &extractableFakeCaster{free: 10}

			result, _ := createItemRegistry().UseResult(Cast{Caster: caster, Skill: def, Targets: []Actor{}})

			if len(caster.granted) != 0 {
				t.Fatalf("granted = %v, want nothing", caster.granted)
			}
			want := PreparedForReuseMessage{SkillID: 2104, Level: 3}
			if len(result.Messages) != 1 || result.Messages[0] != want {
				t.Fatalf("messages = %+v, want exactly [%+v]", result.Messages, want)
			}
		})
	}
}

// TestCreateItemWithoutRandomCountCreatesNothing pins a CREATE_ITEM skill
// whose random count is below 1: no extra count can be drawn, so nothing
// is created and nothing is reported.
func TestCreateItemWithoutRandomCountCreatesNothing(t *testing.T) {
	caster := &extractableFakeCaster{free: 10}

	result, _ := createItemRegistry().UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{ID: 2104, Level: 1, SkillType: "CREATE_ITEM"}.WithExtra(modelskill.Extra{CreateItemIDs: []int32{1835}, CreateItemCount: 1000}),
		Targets: []Actor{},
	})

	if len(caster.granted) != 0 {
		t.Fatalf("granted = %v, want nothing", caster.granted)
	}
	if len(result.Messages) != 0 {
		t.Fatalf("messages = %+v, want none", result.Messages)
	}
}
