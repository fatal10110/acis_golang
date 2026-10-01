package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// TestConditionActorRidingFollowsMountType pins Player.isRiding()
// (_mountType == 1) as the riding state a skill's <player riding="..."/>
// condition reads: a strider rider is riding, a wyvern rider is flying and
// not riding, and a dismount clears it.
func TestConditionActorRidingFollowsMountType(t *testing.T) {
	ridingRequired := modelskill.Definition{ID: 325, Level: 1, Conditions: []modelskill.ConditionClause{{
		Root: modelskill.Condition{Kind: "player", Attrs: map[string]string{"riding": "true"}},
	}}}
	onFoot := modelskill.Definition{ID: 1, Level: 1, Conditions: []modelskill.ConditionClause{{
		Root: modelskill.Condition{Kind: "player", Attrs: map[string]string{"riding": "false"}},
	}}}

	cases := []struct {
		name         string
		npcID        int32
		riding       bool
		flying       bool
		passRiding   bool
		passOnFoot   bool
		wantMountTyp int32
	}{
		{name: "on foot", passOnFoot: true},
		{name: "wind strider", npcID: 12526, riding: true, passRiding: true, wantMountTyp: MountTypeStrider},
		{name: "star strider", npcID: 12527, riding: true, passRiding: true, wantMountTyp: MountTypeStrider},
		{name: "twilight strider", npcID: 12528, riding: true, passRiding: true, wantMountTyp: MountTypeStrider},
		{name: "wyvern", npcID: wyvernNPCID, flying: true, passOnFoot: true, wantMountTyp: MountTypeWyvern},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			if tc.npcID != 0 && !c.Mount(tc.npcID, 77) {
				t.Fatal("Mount() = false, want true")
			}
			if got := c.MountType(); got != tc.wantMountTyp {
				t.Fatalf("MountType() = %d, want %d", got, tc.wantMountTyp)
			}
			actor := c.ConditionActor()
			if got := actor.IsRiding(); got != tc.riding {
				t.Fatalf("IsRiding() = %v, want %v", got, tc.riding)
			}
			if got := actor.IsFlying(); got != tc.flying {
				t.Fatalf("IsFlying() = %v, want %v", got, tc.flying)
			}
			if _, ok := conditions.EvaluateSkill(ridingRequired, c, nil); ok != tc.passRiding {
				t.Fatalf(`riding="true" passes = %v, want %v`, ok, tc.passRiding)
			}
			if _, ok := conditions.EvaluateSkill(onFoot, c, nil); ok != tc.passOnFoot {
				t.Fatalf(`riding="false" passes = %v, want %v`, ok, tc.passOnFoot)
			}

			if tc.npcID == 0 {
				return
			}
			if !c.Dismount() {
				t.Fatal("Dismount() = false, want true")
			}
			if c.ConditionActor().IsRiding() || c.ConditionActor().IsFlying() {
				t.Fatal("riding or flying after the dismount")
			}
			if _, ok := conditions.EvaluateSkill(ridingRequired, c, nil); ok {
				t.Fatal(`riding="true" passes after the dismount`)
			}
			if _, ok := conditions.EvaluateSkill(onFoot, c, nil); !ok {
				t.Fatal(`riding="false" fails after the dismount`)
			}
		})
	}
}
