package cast

import (
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestStartPlayerSkillAcceptsKnownActiveSkill(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(3, 1)
	target := requestTarget{id: 20}
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	defs := requestDefinitions{
		{ID: 3, Level: 1}: {
			ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 1200,
		},
	}

	started, err := StartPlayerSkill(PlayerSkillRequest{
		Now:         time.Unix(1000, 0),
		Controller:  ctrl,
		Caster:      ch,
		Selected:    target,
		SkillID:     3,
		Definitions: defs,
	})
	if err != nil {
		t.Fatalf("StartPlayerSkill() error: %v", err)
	}
	if started.Definition.ID != 3 || started.Definition.Level != 1 {
		t.Fatalf("Definition = %+v, want skill 3/1", started.Definition)
	}
	if started.Target != target {
		t.Fatalf("Target = %v, want selected target", started.Target)
	}
	if started.Plan.HitTime != 500*time.Millisecond || started.Plan.ReuseDelay != 1200*time.Millisecond {
		t.Fatalf("Plan timing = hit %s reuse %s, want 500ms/1.2s", started.Plan.HitTime, started.Plan.ReuseDelay)
	}
	if !ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = false, want started")
	}
}

func TestStartPlayerSkillRejectsUnavailableSkill(t *testing.T) {
	active := modelskill.Definition{ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	inactive := active
	inactive.Activation = modelskill.ActivationPassive

	tests := []struct {
		name    string
		skillID int
		level   int
		dead    bool
		defs    requestDefinitions
	}{
		{name: "nonpositive request", skillID: 0, level: 1, defs: requestDefinitions{{ID: 3, Level: 1}: active}},
		{name: "dead caster", skillID: 3, level: 1, dead: true, defs: requestDefinitions{{ID: 3, Level: 1}: active}},
		{name: "unknown level", skillID: 3, defs: requestDefinitions{{ID: 3, Level: 1}: active}},
		{name: "missing definition", skillID: 3, level: 1, defs: requestDefinitions{}},
		{name: "inactive definition", skillID: 3, level: 1, defs: requestDefinitions{{ID: 3, Level: 1}: inactive}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := newRequestCharacter(10)
			ch.SetSkillLevel(3, tt.level)
			if tt.dead {
				ch.MarkDead()
			}
			ctrl := NewController(&testActor{mp: 100, hp: 100})

			if _, err := StartPlayerSkill(PlayerSkillRequest{
				Now:         time.Unix(1000, 0),
				Controller:  ctrl,
				Caster:      ch,
				SkillID:     tt.skillID,
				Definitions: tt.defs,
			}); !errors.Is(err, ErrSkillUnavailable) {
				t.Fatalf("StartPlayerSkill() error = %v, want ErrSkillUnavailable", err)
			}
			if ctrl.CastingNow() {
				t.Fatal("controller CastingNow() = true after unavailable skill")
			}
		})
	}
}

func TestStartPlayerSkillRejectsInvalidTarget(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(3, 1)
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	defs := requestDefinitions{
		{ID: 3, Level: 1}: {ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne},
	}

	started, err := StartPlayerSkill(PlayerSkillRequest{
		Now:         time.Unix(1000, 0),
		Controller:  ctrl,
		Caster:      ch,
		Selected:    struct{}{},
		SkillID:     3,
		Definitions: defs,
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("StartPlayerSkill() error = %v, want ErrInvalidTarget", err)
	}
	if started.Definition.ID != 3 || started.Target != nil {
		t.Fatalf("started = %+v, want definition with nil target", started)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after invalid target")
	}
}

func TestStartItemSkillAcceptsResolvedSkill(t *testing.T) {
	ch := newRequestCharacter(10)
	target := requestTarget{id: 20}
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	def := modelskill.Definition{
		ID: 7, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		StaticHitTime: true, HitTime: 800, StaticReuse: true, ReuseDelay: 0,
	}
	defs := requestDefinitions{{ID: 7, Level: 1}: def}

	// A caster with no learned skill level for 7 still starts the cast:
	// unlike StartPlayerSkill, the definition comes from the item, not the
	// caster's own skill list.
	started, err := StartItemSkill(ItemSkillRequest{
		Now:         time.Unix(1000, 0),
		Controller:  ctrl,
		Caster:      ch,
		Selected:    target,
		Skill:       modelskill.Ref{ID: 7, Level: 1},
		Definitions: defs,
	})
	if err != nil {
		t.Fatalf("StartItemSkill() error: %v", err)
	}
	if started.Definition.ID != 7 || started.Definition.Level != 1 {
		t.Fatalf("Definition = %+v, want skill 7/1", started.Definition)
	}
	if started.Target != target {
		t.Fatalf("Target = %v, want selected target", started.Target)
	}
	if !ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = false, want started")
	}
}

func TestStartItemSkillRejectsUnavailableSkill(t *testing.T) {
	active := modelskill.Definition{ID: 7, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf}
	inactive := active
	inactive.Activation = modelskill.ActivationPassive

	tests := []struct {
		name  string
		dead  bool
		skill modelskill.Ref
		defs  requestDefinitions
	}{
		{name: "dead caster", dead: true, skill: modelskill.Ref{ID: 7, Level: 1}, defs: requestDefinitions{{ID: 7, Level: 1}: active}},
		{name: "missing definition", skill: modelskill.Ref{ID: 7, Level: 1}, defs: requestDefinitions{}},
		{name: "inactive definition", skill: modelskill.Ref{ID: 7, Level: 1}, defs: requestDefinitions{{ID: 7, Level: 1}: inactive}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := newRequestCharacter(10)
			if tt.dead {
				ch.MarkDead()
			}
			ctrl := NewController(&testActor{mp: 100, hp: 100})

			if _, err := StartItemSkill(ItemSkillRequest{
				Now:         time.Unix(1000, 0),
				Controller:  ctrl,
				Caster:      ch,
				Skill:       tt.skill,
				Definitions: tt.defs,
			}); !errors.Is(err, ErrSkillUnavailable) {
				t.Fatalf("StartItemSkill() error = %v, want ErrSkillUnavailable", err)
			}
			if ctrl.CastingNow() {
				t.Fatal("controller CastingNow() = true after unavailable skill")
			}
		})
	}
}

func TestStartItemSkillRejectsInvalidTarget(t *testing.T) {
	ch := newRequestCharacter(10)
	ctrl := NewController(&testActor{mp: 100, hp: 100})
	defs := requestDefinitions{
		{ID: 7, Level: 1}: {ID: 7, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne},
	}

	started, err := StartItemSkill(ItemSkillRequest{
		Now:         time.Unix(1000, 0),
		Controller:  ctrl,
		Caster:      ch,
		Selected:    struct{}{},
		Skill:       modelskill.Ref{ID: 7, Level: 1},
		Definitions: defs,
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("StartItemSkill() error = %v, want ErrInvalidTarget", err)
	}
	if started.Definition.ID != 7 || started.Target != nil {
		t.Fatalf("started = %+v, want definition with nil target", started)
	}
	if ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = true after invalid target")
	}
}

func TestResolvePlayerToggleAcceptsKnownToggleSkill(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(288, 1)
	defs := requestDefinitions{
		{ID: 288, Level: 1}: {ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf},
	}

	def, target, err := ResolvePlayerToggle(PlayerToggleRequest{
		Caster:      ch,
		SkillID:     288,
		Definitions: defs,
	})
	if err != nil {
		t.Fatalf("ResolvePlayerToggle() error: %v", err)
	}
	if def.ID != 288 || def.Level != 1 {
		t.Fatalf("Definition = %+v, want skill 288/1", def)
	}
	if target != ch {
		t.Fatalf("Target = %v, want the caster (SELF target)", target)
	}
}

func TestResolvePlayerToggleRejectsUnavailableSkill(t *testing.T) {
	toggle := modelskill.Definition{ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf}
	active := toggle
	active.Activation = modelskill.ActivationActive

	tests := []struct {
		name    string
		skillID int
		level   int
		dead    bool
		defs    requestDefinitions
	}{
		{name: "nonpositive request", skillID: 0, level: 1, defs: requestDefinitions{{ID: 288, Level: 1}: toggle}},
		{name: "dead caster", skillID: 288, level: 1, dead: true, defs: requestDefinitions{{ID: 288, Level: 1}: toggle}},
		{name: "unknown level", skillID: 288, defs: requestDefinitions{{ID: 288, Level: 1}: toggle}},
		{name: "missing definition", skillID: 288, level: 1, defs: requestDefinitions{}},
		{name: "non-toggle definition", skillID: 288, level: 1, defs: requestDefinitions{{ID: 288, Level: 1}: active}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := newRequestCharacter(10)
			ch.SetSkillLevel(288, tt.level)
			if tt.dead {
				ch.MarkDead()
			}

			if _, _, err := ResolvePlayerToggle(PlayerToggleRequest{
				Caster:      ch,
				SkillID:     tt.skillID,
				Definitions: tt.defs,
			}); !errors.Is(err, ErrSkillUnavailable) {
				t.Fatalf("ResolvePlayerToggle() error = %v, want ErrSkillUnavailable", err)
			}
		})
	}
}

func TestResolvePlayerToggleRejectsInvalidTarget(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(288, 1)
	defs := requestDefinitions{
		{ID: 288, Level: 1}: {ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetOne},
	}

	def, target, err := ResolvePlayerToggle(PlayerToggleRequest{
		Caster:      ch,
		Selected:    struct{}{},
		SkillID:     288,
		Definitions: defs,
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("ResolvePlayerToggle() error = %v, want ErrInvalidTarget", err)
	}
	if def.ID != 288 || target != nil {
		t.Fatalf("resolved = %+v/%v, want definition with nil target", def, target)
	}
}

type requestDefinitions map[modelskill.Ref]modelskill.Definition

func newRequestCharacter(id int32) *player.Character {
	ch := &player.Character{ID: id}
	ch.SetResourceValues(player.Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 100, CurrentMP: 100})
	return ch
}

func (d requestDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := d[ref]
	return def, ok
}

type requestTarget struct {
	id int32
}

func (t requestTarget) ObjectID() int32 { return t.id }

func (requestTarget) Position() (int, int, int) { return 1, 2, 3 }
