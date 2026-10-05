package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestContinuousBuffOnCursedHolderFromClanHallManager follows
// Continuous.useSkill's BUFF gate: a cursed-weapon holder receives no buff
// from another creature, except from a clan hall manager.
func TestContinuousBuffOnCursedHolderFromClanHallManager(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind string
		want int
	}{
		{"Folk", 0},
		{"ClanHallManagerNpc", 1},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			inst, err := npc.NewInstance(10, &npc.Template{ID: 35384, TemplateID: 35384, Type: tc.kind, Level: 70, HPMax: 2444})
			if err != nil {
				t.Fatal(err)
			}
			caster, err := npc.NewFolk(inst)
			if err != nil {
				t.Fatal(err)
			}
			target := &skillTarget{
				fakeActor: fakeActor{objectID: 2}, hp: 100, maxHP: 100, isPlayer: true, cursed: true,
				name: "Holder", effects: newTestList(nil), skillSuccessOK: true,
			}
			continuousHandler{}.UseResult(Cast{
				Caster:  caster,
				Skill:   modelskill.Definition{ID: 4342, Level: 1, SkillType: "BUFF", Effects: buffEffect()},
				Targets: []Actor{target},
			})
			if got := len(target.effects.All()); got != tc.want {
				t.Fatalf("buffs landed = %d, want %d", got, tc.want)
			}
		})
	}
}
