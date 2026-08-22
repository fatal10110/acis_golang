package cast

import (
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestStartPlayerSkillAcceptsKnownActiveSkill(t *testing.T) {
	ch := newRequestCharacter(10)
	ch.SetSkillLevel(3, 1)
	target := &requestTarget{id: 20}
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
		Ctrl:        true,
		Shift:       true,
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
	if !started.Ctrl || !started.Shift {
		t.Fatalf("Ctrl = %v, Shift = %v, want both true", started.Ctrl, started.Shift)
	}
	if started.Plan.HitTime != 500*time.Millisecond || started.Plan.ReuseDelay != 1200*time.Millisecond {
		t.Fatalf("Plan timing = hit %s reuse %s, want 500ms/1.2s", started.Plan.HitTime, started.Plan.ReuseDelay)
	}
	if !ctrl.CastingNow() {
		t.Fatal("controller CastingNow() = false, want started")
	}
}

// TestStartPlayerSkillClearsRecentFakeDeath covers PlayerCast.doCast's
// unconditional _actor.clearRecentFakeDeath() (PlayerCast.java:181-185),
// which runs right after super.doCast() commits to the cast — at cast
// start, not cast finish. A rejected start (invalid target here) never
// reaches that line in Java either, so the grace must survive it.
func TestStartPlayerSkillClearsRecentFakeDeath(t *testing.T) {
	def := modelskill.Definition{
		ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		StaticHitTime: true, HitTime: 500, StaticReuse: true, ReuseDelay: 1200,
	}

	t.Run("accepted start clears the grace", func(t *testing.T) {
		ch := newRequestCharacter(10)
		ch.SetSkillLevel(3, 1)
		ch.MarkRecentFakeDeath()
		if !ch.RecentFakeDeath() {
			t.Fatal("MarkRecentFakeDeath did not set the grace")
		}
		ctrl := NewController(&testActor{mp: 100, hp: 100})
		defs := requestDefinitions{{ID: 3, Level: 1}: def}

		if _, err := StartPlayerSkill(PlayerSkillRequest{
			Now: time.Unix(1000, 0), Controller: ctrl, Caster: ch,
			Selected: &requestTarget{id: 20}, SkillID: 3, Definitions: defs,
		}); err != nil {
			t.Fatalf("StartPlayerSkill() error: %v", err)
		}
		if ch.RecentFakeDeath() {
			t.Fatal("RecentFakeDeath() = true after an accepted cast start, want cleared")
		}
	})

	t.Run("rejected start leaves the grace running", func(t *testing.T) {
		ch := newRequestCharacter(10)
		ch.SetSkillLevel(3, 1)
		ch.MarkRecentFakeDeath()
		ctrl := NewController(&testActor{mp: 100, hp: 100})
		defs := requestDefinitions{{ID: 3, Level: 1}: def}

		if _, err := StartPlayerSkill(PlayerSkillRequest{
			Now: time.Unix(1000, 0), Controller: ctrl, Caster: ch,
			SkillID: 3, Definitions: defs,
		}); !errors.Is(err, ErrInvalidTarget) {
			t.Fatalf("StartPlayerSkill() error = %v, want ErrInvalidTarget", err)
		}
		if !ch.RecentFakeDeath() {
			t.Fatal("RecentFakeDeath() = false after a rejected cast start, want still running")
		}
	})

	t.Run("FUSION start leaves the grace running", func(t *testing.T) {
		fusionDef := def
		fusionDef.SkillType = "FUSION"
		ch := newRequestCharacter(10)
		ch.SetSkillLevel(3, 1)
		ch.MarkRecentFakeDeath()
		ctrl := NewController(&testActor{mp: 100, hp: 100})
		defs := requestDefinitions{{ID: 3, Level: 1}: fusionDef}

		if _, err := StartPlayerSkill(PlayerSkillRequest{
			Now: time.Unix(1000, 0), Controller: ctrl, Caster: ch,
			Selected: &requestTarget{id: 20}, SkillID: 3, Definitions: defs,
		}); err != nil {
			t.Fatalf("StartPlayerSkill() error: %v", err)
		}
		// PlayerAI dispatches FUSION (and SIGNET_CASTTIME) to
		// doFusionCast instead of doCast (PlayerAI.java:299-301), which
		// never calls clearRecentFakeDeath.
		if !ch.RecentFakeDeath() {
			t.Fatal("RecentFakeDeath() = false after a FUSION cast start, want still running")
		}
	})
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
	target := &requestTarget{id: 20}
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

// TestResolvePlayerToggleAllowsFakeDeathSkillWhileFaking matches
// PlayerCast.canAttemptCast's exception: `if (_actor.isFakeDeath() &&
// skill.getId() != 60)` (PlayerCast.java:218-222) rejects every cast while
// fake-dead except skill 60 itself, so a faking player can always re-cast
// the toggle to un-fake.
func TestResolvePlayerToggleAllowsFakeDeathSkillWhileFaking(t *testing.T) {
	ch := requestCharacterFakingDeath(t)
	ch.SetSkillLevel(fakeDeathSkillID, 1)
	defs := requestDefinitions{
		{ID: fakeDeathSkillID, Level: 1}: {ID: fakeDeathSkillID, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf},
	}

	def, target, err := ResolvePlayerToggle(PlayerToggleRequest{
		Caster:      ch,
		SkillID:     fakeDeathSkillID,
		Definitions: defs,
	})
	if err != nil {
		t.Fatalf("ResolvePlayerToggle() error: %v, want nil while faking for skill %d", err, fakeDeathSkillID)
	}
	if def.ID != fakeDeathSkillID || target != ch {
		t.Fatalf("resolved = %+v/%v, want skill %d targeting the caster", def, target, fakeDeathSkillID)
	}
}

// TestResolvePlayerToggleRejectsOtherSkillsWhileFakeDeath asserts the
// exception above is scoped to skill 60 only: any other toggle stays
// rejected while faking, matching the same Java condition.
func TestResolvePlayerToggleRejectsOtherSkillsWhileFakeDeath(t *testing.T) {
	ch := requestCharacterFakingDeath(t)
	ch.SetSkillLevel(288, 1)
	defs := requestDefinitions{
		{ID: 288, Level: 1}: {ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf},
	}

	if _, _, err := ResolvePlayerToggle(PlayerToggleRequest{
		Caster:      ch,
		SkillID:     288,
		Definitions: defs,
	}); !errors.Is(err, ErrSkillUnavailable) {
		t.Fatalf("ResolvePlayerToggle() error = %v, want ErrSkillUnavailable while faking for a non-60 skill", err)
	}
}

func requestCharacterFakingDeath(t *testing.T) *player.Character {
	t.Helper()
	ch := newRequestCharacter(10)
	live, err := creature.NewLive(location.Location{}, 0, requestGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	ch.Live = live

	e, err := effect.New(effect.Skill{ID: fakeDeathSkillID}, modelskill.EffectTemplate{Name: "FakeDeath"})
	if err != nil {
		t.Fatalf("effect.New(FakeDeath) error: %v", err)
	}
	e.Effected = ch
	ch.EffectList().Add(e)
	return ch
}

type requestGeo struct{}

func (requestGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (requestGeo) Height(_, _, _ int) int16          { return 0 }
func (requestGeo) FindPath(_, _ location.Location) ([]location.Location, bool) {
	return nil, false
}
func (requestGeo) Walkable(int, int, int) bool { return true }
func (requestGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
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
	world.Presence
	id int32
}

func (t *requestTarget) ObjectID() int32 { return t.id }
