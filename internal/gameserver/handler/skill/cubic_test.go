package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type fakeCubicSummoner struct {
	added        map[cubic.ID]bool
	givenByOther map[cubic.ID]bool
	nextAdded    bool
}

func newFakeCubicSummoner(nextAdded bool) *fakeCubicSummoner {
	return &fakeCubicSummoner{added: map[cubic.ID]bool{}, givenByOther: map[cubic.ID]bool{}, nextAdded: nextAdded}
}

func (f *fakeCubicSummoner) AddOrRefreshCubic(id cubic.ID, givenByOther bool) bool {
	f.added[id] = true
	f.givenByOther[id] = givenByOther
	return f.nextAdded
}

func TestCubicHandlerAddsToSelfWhenSingleTarget(t *testing.T) {
	caster := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm)},
		Targets: []any{caster},
	})

	if !result.CubicAdded {
		t.Fatal("UseResult().CubicAdded = false, want true")
	}
	if !caster.added[cubic.Storm] {
		t.Fatal("caster's cubic list was never touched")
	}
	if caster.givenByOther[cubic.Storm] {
		t.Fatal("caster's own cast reported givenByOther=true, want false")
	}
}

func TestCubicHandlerNoOpForServitorBranch(t *testing.T) {
	caster := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster: caster,
		// IsCubic false: the servitor branch, out of this handler's scope.
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: false, NpcID: 14848},
		Targets: []any{caster},
	})

	if result.CubicAdded {
		t.Fatal("UseResult().CubicAdded = true for a non-cubic SUMMON skill, want false")
	}
	if len(caster.added) != 0 {
		t.Fatal("servitor-branch cast touched the cubic list, want untouched")
	}
}

func TestCubicHandlerMassCubicMarksOthersGivenByOther(t *testing.T) {
	caster := newFakeCubicSummoner(true)
	other := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm)},
		Targets: []any{caster, other},
	})

	if !result.CubicAdded {
		t.Fatal("UseResult().CubicAdded = false, want true (caster's own admission)")
	}
	if caster.givenByOther[cubic.Storm] {
		t.Fatal("caster's own admission reported givenByOther=true, want false")
	}
	if !other.givenByOther[cubic.Storm] {
		t.Fatal("other recipient's admission reported givenByOther=false, want true")
	}
}

func TestCubicHandlerRegisteredForSummonType(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newFakeCubicSummoner(true)

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Vampiric)},
		Targets: []any{caster},
	}) {
		t.Fatal("Use() returned false for SUMMON")
	}
	if !caster.added[cubic.Vampiric] {
		t.Fatal("registry dispatch never reached cubicHandler")
	}
}
