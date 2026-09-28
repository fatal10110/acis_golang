package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// landActor is an effect participant at a fixed X on the Y=Z=0 line.
type landActor struct {
	world.Presence
	neutralActor
	id       int32
	x        int
	list     *List
	invul    bool
	noDamage bool
}

func (a *landActor) ObjectID() int32                    { return a.id }
func (a *landActor) Dead() bool                         { return false }
func (a *landActor) Position() (x, y, z int)            { return a.x, 0, 0 }
func (a *landActor) EffectList() *List                  { return a.list }
func (a *landActor) Invul() bool                        { return a.invul }
func (a *landActor) CanGiveDamage() bool                { return !a.noDamage }
func (a *landActor) StopSkillEffectsByID(modelskill.ID) {}

func newLandActor(id int32, x int) *landActor {
	return &landActor{id: id, x: x, list: newTestList(activityTestOwner{})}
}

// TestLandsEffectRangeIsStrict pins the landing radius as strict: a target
// exactly at the effect range is outside it, one unit closer is inside, and
// a self-application or a sourceless landing skips the check.
func TestLandsEffectRangeIsStrict(t *testing.T) {
	def := modelskill.Definition{EffectRange: 100}
	effector := newLandActor(1, 0)
	for _, tc := range []struct {
		name     string
		effector Actor
		effected *landActor
		want     bool
	}{
		{"exactly at range", effector, newLandActor(2, 100), false},
		{"inside range", effector, newLandActor(2, 99), true},
		{"self far away", effector, &landActor{id: 1, x: 5000}, true},
		{"sourceless", nil, newLandActor(2, 5000), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Lands(tc.effector, tc.effected, def); got != tc.want {
				t.Fatalf("Lands() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestLandsRefusesOffensiveOnInvulnerableOrDamageDenied pins the offensive
// refusals: an invulnerable effected or an effector denied damage refuses an
// offensive or debuff skill aimed at someone else, never a friendly one or a
// self-application; a sourceless landing still honors invulnerability.
func TestLandsRefusesOffensiveOnInvulnerableOrDamageDenied(t *testing.T) {
	offensive := modelskill.Definition{Offensive: true}
	debuff := modelskill.Definition{Debuff: true}
	friendly := modelskill.Definition{}
	invul := &landActor{id: 2, invul: true}
	denied := &landActor{id: 1, noDamage: true}
	plain := &landActor{id: 1}
	for _, tc := range []struct {
		name     string
		effector Actor
		effected Actor
		def      modelskill.Definition
		want     bool
	}{
		{"offensive at invulnerable", plain, invul, offensive, false},
		{"debuff at invulnerable", plain, invul, debuff, false},
		{"friendly at invulnerable", plain, invul, friendly, true},
		{"offensive from damage-denied", denied, &landActor{id: 2}, offensive, false},
		{"friendly from damage-denied", denied, &landActor{id: 2}, friendly, true},
		{"offensive on invulnerable self", invul, invul, offensive, true},
		{"sourceless offensive at invulnerable", nil, invul, offensive, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Lands(tc.effector, tc.effected, tc.def); got != tc.want {
				t.Fatalf("Lands() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestApplyHostsSelfTargetKindOnEffector pins owner resolution: a
// self-target kind (StunSelf) is held by its effector while still naming
// the other side as effected, every other kind is held by the effected, and
// a self-target kind with no effector is dropped.
func TestApplyHostsSelfTargetKindOnEffector(t *testing.T) {
	effector := newLandActor(1, 0)
	effected := newLandActor(2, 0)
	templates := []modelskill.EffectTemplate{{Name: "StunSelf", Time: 9}, {Name: "Debuff", Time: 9}}
	Apply(effector, effected, Skill{ID: 81, Level: 1}, templates)

	held := effector.list.All()
	if len(held) != 1 || held[0].Type != TypeStunSelf || held[0].Effector != Actor(effector) || held[0].Effected != Actor(effected) {
		t.Fatalf("effector-held effects = %+v, want one StunSelf of effector on effected", held)
	}
	landed := effected.list.All()
	if len(landed) != 1 || landed[0].Type != TypeDebuff || landed[0].Effector != Actor(effector) || landed[0].Effected != Actor(effected) {
		t.Fatalf("effected-held effects = %+v, want one Debuff of effector on effected", landed)
	}

	sourceless := newLandActor(3, 0)
	Apply(nil, sourceless, Skill{ID: 81, Level: 1}, templates[:1])
	if got := sourceless.list.All(); len(got) != 0 {
		t.Fatalf("sourceless StunSelf landed on %+v, want dropped", got)
	}
}
