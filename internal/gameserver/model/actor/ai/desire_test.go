package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestDesireEqual(t *testing.T) {
	target := actor(2)
	otherTarget := actor(3)

	tests := []struct {
		name string
		a, b *Desire
		want bool
	}{
		{
			name: "idle always equal",
			a:    &Desire{Kind: IntentionIdle},
			b:    &Desire{Kind: IntentionIdle, Weight: 5},
			want: true,
		},
		{
			name: "wander always equal",
			a:    &Desire{Kind: IntentionWander},
			b:    &Desire{Kind: IntentionWander},
			want: true,
		},
		{
			name: "attack same final target",
			a:    &Desire{Kind: IntentionAttack, FinalTarget: target},
			b:    &Desire{Kind: IntentionAttack, FinalTarget: target},
			want: true,
		},
		{
			name: "attack different final target",
			a:    &Desire{Kind: IntentionAttack, FinalTarget: target},
			b:    &Desire{Kind: IntentionAttack, FinalTarget: otherTarget},
			want: false,
		},
		{
			name: "different kind",
			a:    &Desire{Kind: IntentionAttack, FinalTarget: target},
			b:    &Desire{Kind: IntentionFlee, FinalTarget: target},
			want: false,
		},
		{
			name: "cast requires same target and skill",
			a:    &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 1, Level: 2}},
			b:    &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 1, Level: 2}},
			want: true,
		},
		{
			name: "cast rejects different skill level",
			a:    &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 1, Level: 2}},
			b:    &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 1, Level: 3}},
			want: false,
		},
		{
			name: "pick up requires same item",
			a:    &Desire{Kind: IntentionPickUp, ItemObjectID: 7},
			b:    &Desire{Kind: IntentionPickUp, ItemObjectID: 7},
			want: true,
		},
		{
			name: "social requires same id",
			a:    &Desire{Kind: IntentionSocial, ItemObjectID: 7},
			b:    &Desire{Kind: IntentionSocial, ItemObjectID: 8},
			want: false,
		},
		{
			name: "move route requires same route name",
			a:    &Desire{Kind: IntentionMoveRoute, RouteName: "patrol"},
			b:    &Desire{Kind: IntentionMoveRoute, RouteName: "patrol"},
			want: true,
		},
		{
			name: "move route rejects different route name",
			a:    &Desire{Kind: IntentionMoveRoute, RouteName: "patrol"},
			b:    &Desire{Kind: IntentionMoveRoute, RouteName: "guard"},
			want: false,
		},
		{
			name: "move to within tolerance",
			a:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 0, Y: 0, Z: 0}},
			b:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 10, Y: 10, Z: 20}},
			want: true,
		},
		{
			name: "move to beyond ground tolerance",
			a:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 0, Y: 0, Z: 0}},
			b:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 30, Y: 0, Z: 0}},
			want: false,
		},
		{
			name: "move to beyond height tolerance",
			a:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 0, Y: 0, Z: 0}},
			b:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 0, Y: 0, Z: 31}},
			want: false,
		},
		{
			name: "interact never merges, even with matching target",
			a:    &Desire{Kind: IntentionInteract, Target: target},
			b:    &Desire{Kind: IntentionInteract, Target: target},
			want: false,
		},
		{
			name: "use item never merges",
			a:    &Desire{Kind: IntentionUseItem, ItemObjectID: 5},
			b:    &Desire{Kind: IntentionUseItem, ItemObjectID: 5},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Errorf("Equal() = %v, want %v", got, tc.want)
			}
			// Equal must be symmetric.
			if got := tc.b.Equal(tc.a); got != tc.want {
				t.Errorf("reverse Equal() = %v, want %v", got, tc.want)
			}
		})
	}
}
