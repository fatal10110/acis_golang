package effect

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestModOwnerIsEffect: only a running effect's owner reports IsEffect;
// a passive skill, an item, an augmentation and the zero owner do not.
func TestModOwnerIsEffect(t *testing.T) {
	inst := &item.Instance{ObjectID: 1, TemplateID: 101}
	for name, tc := range map[string]struct {
		owner ModOwner
		want  bool
	}{
		"effect":       {ModOwnerEffect(&Effect{}), true},
		"skill":        {ModOwnerSkill(modelskill.Ref{ID: 1068, Level: 1}), false},
		"item":         {ModOwnerItem(ItemOwner{Inst: inst}), false},
		"augmentation": {ModOwnerAugmentation(inst), false},
		"zero":         {ModOwner{}, false},
	} {
		if got := tc.owner.IsEffect(); got != tc.want {
			t.Errorf("%s: IsEffect() = %v, want %v", name, got, tc.want)
		}
	}
}
