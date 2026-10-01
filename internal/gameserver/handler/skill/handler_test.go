package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// ---- from handler_test.go ----
type recordingHandler struct {
	types []string
	uses  int
}

func (h *recordingHandler) Types() []string { return h.types }

func (h *recordingHandler) Use(Cast) { h.uses++ }

func TestRegistryDispatchesBySkillType(t *testing.T) {
	h := &recordingHandler{types: []string{"HEAL_PERCENT", "MANAHEAL_PERCENT"}}
	registry := NewRegistry(h)

	if _, ok := registry.Handler("heal_percent"); !ok {
		t.Fatal("Handler() did not normalize skill type keys")
	}
	if !registry.Use(Cast{Skill: modelskill.Definition{SkillType: "MANAHEAL_PERCENT"}}) {
		t.Fatal("Use() returned false for a registered skill type")
	}
	if h.uses != 1 {
		t.Fatalf("handler uses = %d, want 1", h.uses)
	}
	if registry.Use(Cast{Skill: modelskill.Definition{SkillType: "NOT_REGISTERED"}}) {
		t.Fatal("Use() returned true for an unregistered skill type")
	}
}

func TestRegistryReportsAttackFailedForPhysicalSkillWithNoDamage(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{
		physicalOK: true,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: -1, Defence: 1,
			RandomMul: 1, RaceMul: 1, PvPMul: 1, ElementalMul: 1, WeaponVulnMul: 1,
		},
	}

	result, ok := registry.UseResult(Cast{
		Skill:   modelskill.Definition{SkillType: "PDAM"},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true for PDAM")
	}
	if result.AttackFailed != 1 {
		t.Fatalf("AttackFailed = %d, want 1", result.AttackFailed)
	}
}

func TestDefaultRegistryHasRepresentativeHandlers(t *testing.T) {
	registry := NewDefaultRegistry()

	for _, skillType := range []string{
		"PDAM", "FATAL", "MDAM", "DEATHLINK", "BLOW", "MANADAM",
		"HEAL", "HEAL_STATIC", "HEAL_PERCENT", "MANAHEAL_PERCENT", "MANAHEAL", "MANARECHARGE",
		"COMBATPOINTHEAL", "BALANCE_LIFE", "REAL_DAMAGE", "GIVE_SP",
		"CPDAMPERCENT", "DUMMY", "BEAST_FEED",
		"SUMMON_CREATURE", "SUMMON_FRIEND", "SUMMON_PARTY", "ERASE",
	} {
		if _, ok := registry.Handler(skillType); !ok {
			t.Fatalf("default registry missing %s", skillType)
		}
	}
}
