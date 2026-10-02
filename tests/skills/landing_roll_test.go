package skills

import (
	"math/rand/v2"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// resistLandingRolls makes every skill landing roll of the player (a draw
// in [0, 100)) come up 99, so no landing at a rate of at most 99 succeeds.
// Every other roll is drawn from others.
func resistLandingRolls(t *testing.T, srv *gameservertest.Server, objID int32, others func(int) int) {
	t.Helper()
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetRollSource(resistLanding(others)) })
}

// resistLanding is a roll source that answers 99 to every roll out of 100
// and draws every other roll from others.
func resistLanding(others func(int) int) func(int) int {
	return func(n int) int {
		if n == 100 {
			return 99
		}
		return others(n)
	}
}

// randomRoll draws a uniform roll in [0, n), like an unseeded character.
func randomRoll(n int) int { return rand.IntN(n) }

// TestPlayerDebuffLandsAtItsPower casts a physical DEBUFF of power 80, with
// no effect power or effect type of its own, at the fixture monster (no
// level dependence, neutral vulnerability). The landing roll starts from
// the skill's power and resists as its own DEBUFF type: the rate is exactly
// 80, so a landing roll of 79 lands the debuff and one of 80 fails it with
// ATTACK_FAILED.
func TestPlayerDebuffLandsAtItsPower(t *testing.T) {
	for _, tc := range []struct {
		roll  int
		lands bool
	}{
		{roll: 79, lands: true},
		{roll: 80, lands: false},
	} {
		t.Run(map[bool]string{true: "lands", false: "resisted"}[tc.lands], func(t *testing.T) {
			t.Parallel()
			const skillID = 1500
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
					ID: skillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
					CastRange: 900, HitTime: 0, StaticHitTime: true,
					SkillType: "DEBUFF", Debuff: true, Offensive: true, Power: 80,
					Effects: []modelskill.EffectTemplate{{Name: "Debuff", Time: 60}},
				}})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, skillID, 1)
			startInWorld(t, c)
			hostile := srv.SpawnHostileNPC(t)
			drainUntilQuiet(t, c)
			targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)
			roll := tc.roll
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				pc.SetRollSource(func(n int) int {
					if n == 100 {
						return roll
					}
					return 0
				})
			})

			c.Send(encodeRequestMagicSkillUse(skillID, false, false))
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, skillID, 1)
			assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
			if !tc.lands {
				assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageAttackFailed)
				drainUntilQuiet(t, c)
				if got := len(hostile.EffectList().All()); got != 0 {
					t.Fatalf("monster effects after a landing roll of %d = %d, want 0", roll, got)
				}
				return
			}
			drainAssertingNoAttackFailed(t, c)
			if got := len(hostile.EffectList().All()); got != 1 {
				t.Fatalf("monster effects after a landing roll of %d = %d, want the debuff", roll, got)
			}
		})
	}
}
