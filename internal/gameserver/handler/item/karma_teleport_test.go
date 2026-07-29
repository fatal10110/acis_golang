package item

import (
	"testing"

	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestIsTeleportOrRecallSkillType(t *testing.T) {
	tests := []struct {
		skillType string
		want      bool
	}{
		{"TELEPORT", true},
		{"RECALL", true},
		{"BUFF", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isTeleportOrRecallSkillType(tt.skillType); got != tt.want {
			t.Errorf("isTeleportOrRecallSkillType(%q) = %v, want %v", tt.skillType, got, tt.want)
		}
	}
}

func TestIsRecallSkillType(t *testing.T) {
	tests := []struct {
		skillType string
		want      bool
	}{
		{"RECALL", true},
		{"TELEPORT", false},
		{"BUFF", false},
	}
	for _, tt := range tests {
		if got := isRecallSkillType(tt.skillType); got != tt.want {
			t.Errorf("isRecallSkillType(%q) = %v, want %v", tt.skillType, got, tt.want)
		}
	}
}

func TestItemBlockedByKarmaTeleport(t *testing.T) {
	recall := modelskill.Definition{ID: 1050, Level: 1, SkillType: "RECALL"}
	buff := modelskill.Definition{ID: 2005, Level: 1, SkillType: "BUFF"}

	recallTmpl := &modelitem.Template{AttachedSkills: []modelitem.SkillRef{{ID: 1050, Level: 1}}}
	buffTmpl := &modelitem.Template{AttachedSkills: []modelitem.SkillRef{{ID: 2005, Level: 1}}}
	unresolvedTmpl := &modelitem.Template{AttachedSkills: []modelitem.SkillRef{{ID: 9999, Level: 1}}}

	tests := []struct {
		name                   string
		tmpl                   *modelitem.Template
		defs                   aiCastDefinitions
		karma                  int
		karmaPlayerCanTeleport bool
		want                   bool
	}{
		{"nil template", nil, aiCastDefinitions{}, 1, false, false},
		{"karma zero does not block", recallTmpl, aiCastDefinitions{{ID: 1050, Level: 1}: recall}, 0, false, false},
		{"karma positive but config allows teleport", recallTmpl, aiCastDefinitions{{ID: 1050, Level: 1}: recall}, 1, true, false},
		{"karma positive blocks recall item", recallTmpl, aiCastDefinitions{{ID: 1050, Level: 1}: recall}, 1, false, true},
		{"karma positive does not block non-teleport skill", buffTmpl, aiCastDefinitions{{ID: 2005, Level: 1}: buff}, 1, false, false},
		{"unresolved attached skill does not block", unresolvedTmpl, aiCastDefinitions{}, 1, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ItemBlockedByKarmaTeleport(tt.tmpl, tt.defs, tt.karma, tt.karmaPlayerCanTeleport); got != tt.want {
				t.Errorf("ItemBlockedByKarmaTeleport() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecallCastBlockedByKarma(t *testing.T) {
	tests := []struct {
		name                   string
		skillType              string
		karma                  int
		karmaPlayerCanTeleport bool
		want                   bool
	}{
		{"recall blocked with positive karma", "RECALL", 1, false, true},
		{"recall allowed by config", "RECALL", 1, true, false},
		{"recall allowed with zero karma", "RECALL", 0, false, false},
		{"teleport type is not gated by direct cast", "TELEPORT", 1, false, false},
		{"non-recall skill not blocked", "BUFF", 1, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RecallCastBlockedByKarma(tt.skillType, tt.karma, tt.karmaPlayerCanTeleport); got != tt.want {
				t.Errorf("RecallCastBlockedByKarma() = %v, want %v", got, tt.want)
			}
		})
	}
}
