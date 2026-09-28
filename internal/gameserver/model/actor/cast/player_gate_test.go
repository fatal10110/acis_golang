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
