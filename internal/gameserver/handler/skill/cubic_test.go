package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestCubicHandlerAddsToSelfWhenSingleTarget(t *testing.T) {
	caster := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm)},
		Targets: []Actor{caster},
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

// TestCubicHandlerDelegatesServitorBranch casts a non-cubic SUMMON from a
// real player: it asks for the servitor and admits no cubic.
func TestCubicHandlerDelegatesServitorBranch(t *testing.T) {
	caster := &player.Character{ID: 1}
	rec := &event.Recorder{}
	caster.Attach(nil, rec)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: false, NpcID: 14848},
		Targets: []Actor{caster},
	})

	if result.CubicAdded || result.CubicTouched {
		t.Fatalf("UseResult() CubicAdded/CubicTouched = %v/%v for a non-cubic SUMMON skill, want false/false", result.CubicAdded, result.CubicTouched)
	}
	if got := caster.CubicIDs(); len(got) != 0 {
		t.Fatalf("caster cubics = %v, want none", got)
	}
	requests := event.Of[event.ServitorSummonRequested](rec)
	if len(requests) != 1 || requests[0].Skill.NpcID != 14848 {
		t.Fatalf("servitor summon requests = %+v, want one for npc 14848", requests)
	}
}

func TestCubicHandlerMassCubicMarksOthersGivenByOther(t *testing.T) {
	caster := newFakeCubicSummoner(true)
	other := newFakeCubicSummoner(true)

	result := cubicHandler{}.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm)},
		Targets: []Actor{caster, other},
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
	if got := result.CubicTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicTargets = %v, want other", got)
	}
	if got := result.CubicAddedTargets; len(got) != 1 || got[0] != other {
		t.Fatalf("CubicAddedTargets = %v, want other", got)
	}
	if result.CubicID != cubic.Storm {
		t.Fatalf("CubicID = %d, want %d", result.CubicID, cubic.Storm)
	}
}

// TestCubicHandlerRegisteredForSummonType dispatches a cubic SUMMON through
// the default registry and finds the cubic in a real player's active list.
func TestCubicHandlerRegisteredForSummonType(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &player.Character{ID: 1}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Vampiric)},
		Targets: []Actor{caster},
	}) {
		t.Fatal("Use() returned false for SUMMON")
	}
	if got := caster.CubicIDs(); len(got) != 1 || got[0] != int(cubic.Vampiric) {
		t.Fatalf("caster cubics = %v, want [%d]: registry dispatch never reached cubicHandler", got, cubic.Vampiric)
	}
}
