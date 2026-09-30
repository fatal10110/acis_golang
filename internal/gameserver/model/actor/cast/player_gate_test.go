package cast

import (
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func newGatePlayer(t *testing.T) (*Controller, *player.Character) {
	t.Helper()
	ch := &player.Character{ID: 1}
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, ch)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	ch.Live = live
	return NewController(PlayerActor{Character: ch}, nil), ch
}

func fakeDie(t *testing.T, ch *player.Character) {
	t.Helper()
	fake, err := effect.New(effect.Skill{ID: fakeDeathSkillID}, modelskill.EffectTemplate{Name: "FakeDeath"})
	if err != nil {
		t.Fatal(err)
	}
	fake.Effected = ch
	ch.EffectList().Add(fake)
	if !ch.FakeDead() {
		t.Fatal("fake-death effect did not make the caster fake dead")
	}
}

// TestPlayerPreAttemptGateOrderAndExemptions pins PlayerCast.canAttemptCast
// (PlayerCast.java:188-259): the rule order when several apply at once, the
// fishing-skill and Fake Death exemptions, and the siege-summon refusal.
func TestPlayerPreAttemptGateOrderAndExemptions(t *testing.T) {
	active := modelskill.Definition{ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, SkillType: "DUMMY"}
	pumping := modelskill.Definition{ID: 1312, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, SkillType: "PUMPING"}
	fakeDeath := modelskill.Definition{ID: fakeDeathSkillID, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf, SkillType: "FAKE_DEATH"}
	siege := modelskill.Definition{ID: 13, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, SkillType: "SUMMON", SiegeSummonSkill: true}

	for _, tt := range []struct {
		name  string
		def   modelskill.Definition
		setup func(*testing.T, *player.Character)
		want  error
	}{
		{name: "standing", def: active},
		{name: "sitting", def: active, setup: func(_ *testing.T, ch *player.Character) { ch.SetStanding(false) }, want: ErrSitting},
		{
			name: "reuse answers before sitting", def: active,
			setup: func(_ *testing.T, ch *player.Character) {
				ch.SetStanding(false)
				ch.DisableSkill(ReuseKey(active), time.Minute)
			},
			want: ErrSkillDisabled,
		},
		{name: "fishing non-fishing skill", def: active, setup: func(_ *testing.T, ch *player.Character) { ch.SetFishing(true) }, want: ErrFishingSkillsOnly},
		{name: "fishing skill while fishing", def: pumping, setup: func(_ *testing.T, ch *player.Character) { ch.SetFishing(true) }},
		{
			name: "fishing answers before sitting", def: active,
			setup: func(_ *testing.T, ch *player.Character) {
				ch.SetStanding(false)
				ch.SetFishing(true)
			},
			want: ErrFishingSkillsOnly,
		},
		{name: "fake death other skill", def: active, setup: func(t *testing.T, ch *player.Character) { fakeDie(t, ch) }, want: ErrSitting},
		{name: "fake death recast", def: fakeDeath, setup: func(t *testing.T, ch *player.Character) { fakeDie(t, ch) }},
		{name: "siege summon outside siege", def: siege, want: ErrSiegeSummonUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, ch := newGatePlayer(t)
			if tt.setup != nil {
				tt.setup(t, ch)
			}
			err := ctrl.CanPlayerAttemptCast(ch, ch, tt.def)
			if tt.want == nil && err != nil {
				t.Fatalf("CanPlayerAttemptCast = %v, want nil", err)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("CanPlayerAttemptCast = %v, want %v", err, tt.want)
			}
		})
	}
}

// olympiadActor is a playable caster whose acting player may be in Olympiad
// mode and whose skill <cond> clauses may fail.
type olympiadActor struct {
	testActor
	olympiad bool
	condFail bool
}

func (a *olympiadActor) ActingPlayerInOlympiad() bool { return a.olympiad }

func (a *olympiadActor) SkillConditions(Target, modelskill.Definition) (modelskill.ConditionClause, bool) {
	return modelskill.ConditionClause{MessageID: 113}, !a.condFail
}

// TestCanCastRefusesOlympiadSkills pins PlayableCast.canCast's Olympiad ban
// (PlayableCast.java:72-79): while the acting player is in Olympiad mode a
// hero skill or a RESURRECT skill is refused, after the skill's <cond>
// clauses and before its item cost; any other skill, or a caster outside
// Olympiad, passes the ban.
func TestCanCastRefusesOlympiadSkills(t *testing.T) {
	hero := modelskill.NewDefinition(395, 1, "Heroic Miracle", modelskill.DefinitionAttrs{})
	hero.SkillType = "BUFF"
	resurrect := modelskill.Definition{ID: 1016, Level: 1, SkillType: "RESURRECT"}
	plain := modelskill.Definition{ID: 1011, Level: 1, SkillType: "HEAL"}
	if !hero.HeroSkill {
		t.Fatal("skill 395 is not a hero skill")
	}
	withItem := func(def modelskill.Definition) modelskill.Definition {
		def.ItemConsumeID, def.ItemConsumeCount = 3031, 1
		return def
	}

	for _, tt := range []struct {
		name     string
		def      modelskill.Definition
		olympiad bool
		condFail bool
		want     error
	}{
		{name: "hero skill in olympiad", def: hero, olympiad: true, want: ErrOlympiadSkill},
		{name: "resurrect in olympiad", def: resurrect, olympiad: true, want: ErrOlympiadSkill},
		{name: "other skill in olympiad", def: plain, olympiad: true},
		{name: "hero skill outside olympiad", def: hero},
		{name: "resurrect outside olympiad", def: resurrect},
		{name: "condition answers first", def: resurrect, olympiad: true, condFail: true, want: new(ConditionError)},
		{name: "ban answers before item cost", def: withItem(resurrect), olympiad: true, want: ErrOlympiadSkill},
		{name: "item cost outside olympiad", def: withItem(resurrect), want: ErrNotEnoughItems},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actor := &olympiadActor{testActor: testActor{mp: 100, hp: 100}, olympiad: tt.olympiad, condFail: tt.condFail}
			err := NewController(actor, nil).CanCast(testTarget{}, tt.def)
			switch want := tt.want.(type) {
			case nil:
				if err != nil {
					t.Fatalf("CanCast = %v, want nil", err)
				}
			case *ConditionError:
				if !errors.As(err, &want) {
					t.Fatalf("CanCast = %v, want a condition failure", err)
				}
			default:
				if !errors.Is(err, want) {
					t.Fatalf("CanCast = %v, want %v", err, want)
				}
			}
		})
	}
}

// olympiadOwner is a summon owner in Olympiad mode.
type olympiadOwner struct{ *player.Character }

func (olympiadOwner) OlympiadMode() bool { return true }

// TestSummonActorAsksItsOwnerAboutOlympiad pins that a summon's Olympiad ban
// follows its acting player, the owner: the summon itself is never in
// Olympiad mode.
func TestSummonActorAsksItsOwnerAboutOlympiad(t *testing.T) {
	_, ch := newGatePlayer(t)
	for _, tt := range []struct {
		name  string
		owner summon.Owner
		want  bool
	}{
		{name: "owner in olympiad", owner: olympiadOwner{ch}, want: true},
		{name: "owner outside olympiad", owner: ch},
		{name: "no owner"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pet, err := summon.NewPet(summon.PetConfig{ObjectID: 2, Owner: tt.owner})
			if err != nil {
				t.Fatal(err)
			}
			if got := (SummonActor{Summon: pet}).ActingPlayerInOlympiad(); got != tt.want {
				t.Fatalf("ActingPlayerInOlympiad = %t, want %t", got, tt.want)
			}
		})
	}
}

// swingState is a player's swing, in flight or not.
type swingState bool

func (s swingState) AttackingNow() bool { return bool(s) }

// TestServitorSummonMidSwingAnswersCannotSummonInCombat pins the in-combat
// half of the servitor gate, PlayerCast.canCast's
// YOU_CANNOT_SUMMON_IN_COMBAT (PlayerCast.java:279-283). A request made
// mid-swing waits for the swing's end, which clears the swing first, so no
// request packet reaches it; only a swing in flight at the cost check does.
// A cubic summon is never refused by it.
func TestServitorSummonMidSwingAnswersCannotSummonInCombat(t *testing.T) {
	_, ch := newGatePlayer(t)
	servitor := modelskill.Definition{ID: 1111, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf, SkillType: "SUMMON", NpcID: 12600}
	cubic := servitor
	cubic.ID, cubic.IsCubic = 10, true
	for _, tt := range []struct {
		name     string
		def      modelskill.Definition
		swinging swingState
		want     error
	}{
		{name: "servitor mid-swing", def: servitor, swinging: true, want: ErrSummonInCombat},
		{name: "servitor between swings", def: servitor},
		{name: "cubic mid-swing", def: cubic, swinging: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := NewController(PlayerActor{Character: ch, Attack: tt.swinging}, nil)
			if err := ctrl.CanCast(ch, tt.def); !errors.Is(err, tt.want) {
				t.Fatalf("CanCast = %v, want %v", err, tt.want)
			}
		})
	}
}
