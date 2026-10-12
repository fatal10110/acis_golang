package skills

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPlayerCastEffectNamesRegisteredPlayer pins one identity for a player
// caster: an effect its cast lands names, as its effector and effected, the
// very object the world registers the player under.
func TestPlayerCastEffectNamesRegisteredPlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t,
			[]modelskill.Definition{
				{
					ID: 1220, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
					HitTime: 500, StaticHitTime: true, SkillType: "HOT",
					Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Value: 100, Count: 3, Time: 1}},
				},
			},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 1220, 1)
	startInWorld(t, c)
	registered, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}

	c.Send(encodeRequestMagicSkillUse(1220, false, false))
	readCastStartFrames(t, c, objID, 1220, 1, 500, 0, objID)
	srv.Advance(t, 700*time.Millisecond)
	drainUntilQuiet(t, c)

	var landed *effect.Effect
	for _, e := range registered.(interface{ EffectList() *effect.List }).EffectList().All() {
		if e.Skill.ID == 1220 {
			landed = e
		}
	}
	if landed == nil {
		t.Fatal("the cast landed no effect on its caster")
	}
	if any(landed.Effector) != any(registered) {
		t.Fatalf("effector = %T %p, want the registered player %T %p", landed.Effector, landed.Effector, registered, registered)
	}
	if any(landed.Effected) != any(registered) {
		t.Fatalf("effected = %T %p, want the registered player %T %p", landed.Effected, landed.Effected, registered, registered)
	}
}
