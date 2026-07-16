package cast

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestSelectTarget(t *testing.T) {
	caster := castTarget{id: 1}
	selected := castTarget{id: 2}

	tests := []struct {
		name     string
		target   modelskill.Target
		selected any
		want     Target
		wantOK   bool
	}{
		{name: "self", target: modelskill.TargetSelf, want: caster, wantOK: true},
		{name: "none", target: modelskill.TargetNone, selected: selected, want: caster, wantOK: true},
		{name: "ground", target: modelskill.TargetGround, selected: selected, want: caster, wantOK: true},
		{name: "one", target: modelskill.TargetOne, selected: selected, want: selected, wantOK: true},
		{name: "one invalid", target: modelskill.TargetOne, selected: struct{}{}, wantOK: false},
		{name: "unsupported", target: modelskill.TargetParty, selected: selected, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := SelectTarget(caster, tt.selected, modelskill.Definition{Target: tt.target})
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("SelectTarget() = (%v, %v), want (%v, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

type castTarget struct {
	id int32
}

func (t castTarget) ObjectID() int32 { return t.id }

func (castTarget) Position() (int, int, int) { return 0, 0, 0 }
