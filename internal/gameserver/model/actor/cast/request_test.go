package cast

import (
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestStartPlayerSkillAcceptsKnownActiveSkill(t *testing.T) {
	ch := &player.Character{ID: 10, CurHP: 100, CurMP: 100}
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
			ch := &player.Character{ID: 10, CurHP: 100, CurMP: 100}
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
	ch := &player.Character{ID: 10, CurHP: 100, CurMP: 100}
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

type requestDefinitions map[modelskill.Ref]modelskill.Definition

func (d requestDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := d[ref]
	return def, ok
}

type requestTarget struct {
	id int32
}

func (t requestTarget) ObjectID() int32 { return t.id }

func (requestTarget) Position() (int, int, int) { return 1, 2, 3 }
