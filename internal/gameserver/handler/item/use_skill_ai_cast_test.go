package item

import (
	"testing"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type aiCastDefinitions map[modelskill.Ref]modelskill.Definition

func (d aiCastDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := d[ref]
	return def, ok
}

func TestResolveAICastSkill(t *testing.T) {
	scroll := modelskill.Definition{ID: 2005, Level: 1, Activation: modelskill.ActivationActive}
	potion := modelskill.Definition{ID: 2031, Level: 1, Potion: true}

	tests := []struct {
		name   string
		tmpl   *modelitem.Template
		defs   actorcast.Definitions
		wantID modelskill.ID
		wantOK bool
	}{
		{
			name: "non-potion carried skill resolves",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
				AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}},
			},
			defs:   aiCastDefinitions{{ID: 2005, Level: 1}: scroll},
			wantID: 2005, wantOK: true,
		},
		{
			name: "potion carried skill is left to the instant-cast path",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
				AttachedSkills: []modelitem.SkillRef{{ID: 2031, Level: 1}},
			},
			defs:   aiCastDefinitions{{ID: 2031, Level: 1}: potion},
			wantOK: false,
		},
		{
			name: "non-ItemSkills handler is not handled",
			tmpl: &modelitem.Template{
				Kind:           modelitem.KindEtcItem,
				EtcItem:        &modelitem.EtcItemDetail{Handler: "SomeOtherHandler"},
				AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}},
			},
			defs:   aiCastDefinitions{{ID: 2005, Level: 1}: scroll},
			wantOK: false,
		},
		{
			name: "no attached skills is not handled",
			tmpl: &modelitem.Template{
				Kind:    modelitem.KindEtcItem,
				EtcItem: &modelitem.EtcItemDetail{Handler: ItemSkillsHandler},
			},
			defs:   aiCastDefinitions{},
			wantOK: false,
		},
		{
			name:   "nil template is not handled",
			tmpl:   nil,
			defs:   aiCastDefinitions{},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := ResolveAICastSkill(tt.tmpl, tt.defs)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && def.ID != tt.wantID {
				t.Fatalf("Definition.ID = %v, want %v", def.ID, tt.wantID)
			}
		})
	}
}
