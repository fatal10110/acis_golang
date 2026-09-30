package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// fistFurySkillID is the datapack's Fist Fury: a self TOGGLE whose
// DamOverTime (val 13, the default 1 s period) is its HP upkeep.
const fistFurySkillID = 222

// fistFury mirrors aCis_datapack/data/xml/skills/0200-0299.xml's skill 222
// (mpConsume 8, target SELF, skillType CONT, operateType TOGGLE,
// DamOverTime count 0x7fffffff val 13 with no time attribute, which the
// reference loader defaults to 1).
func fistFury() modelskill.Definition {
	return modelskill.Definition{
		ID: fistFurySkillID, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
		MPConsume: 8, SkillType: "CONT",
		Effects: []modelskill.EffectTemplate{{Name: "DamOverTime", Value: 13, Count: 0x7fffffff, Time: 1, Icon: true}},
	}
}

// TestToggleUpkeepTickLeavesSleepAndSitting runs a DamOverTime upkeep tick
// on a sleeping or seated player (issue #2894). Reference: the tick goes
// through Creature.reduceCurrentHpByDOT into Player.reduceCurrentHp, which
// hands PlayerStatus.reduceHp isHPConsumption = skill.isToggle()
// (Player.java:6149-6155); reduceHp runs its SLEEP /
// IMMOBILE_UNTIL_ATTACKED stop and its stand-up only under !isHPConsumption
// (PlayerStatus.java:117-132). So a toggle's upkeep tick takes its HP and
// leaves the player asleep or seated, while the same tick from a non-toggle
// skill still wakes the player and stands it up.
func TestToggleUpkeepTickLeavesSleepAndSitting(t *testing.T) {
	t.Parallel()
	const upkeep = 13
	for _, tc := range []struct {
		name    string
		toggle  bool
		posture string
	}{
		{"toggle/sleeping", true, "sleeping"},
		{"toggle/sitting", true, "sitting"},
		{"non-toggle/sleeping", false, "sleeping"},
		{"non-toggle/sitting", false, "sitting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{fistFury()})),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, fistFurySkillID, 1)
			startInWorld(t, c)

			upkeeping := func() (held bool) {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) {
					for _, e := range pc.EffectList().All() {
						held = held || (e.Type == effect.TypeDamOverTime && e.Skill.ID == fistFurySkillID)
					}
				})
				return held
			}
			if tc.toggle {
				c.Send(encodeRequestMagicSkillUse(fistFurySkillID, false, false))
				readMatching(t, c, time.Second, "toggle MagicSkillUse", isSkillUse(fistFurySkillID))
			} else {
				// The same DamOverTime tick from a skill that is not a
				// toggle, landed by the player on itself.
				onPlayerQueue(t, srv, objID, func(pc *player.Character) {
					e, err := effect.New(effect.Skill{ID: fistFurySkillID, Level: 1}, fistFury().Effects[0])
					if err != nil {
						t.Errorf("effect.New(DamOverTime): %v", err)
						return
					}
					e.Effector, e.Effected = pc, pc
					pc.EffectList().Add(e)
				})
			}
			if !upkeeping() {
				t.Fatal("the DamOverTime effect is not running before the tick")
			}
			drainUntilQuiet(t, c)

			asleep := func() (sleeping bool) {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) {
					for _, e := range pc.EffectList().All() {
						sleeping = sleeping || e.Type == effect.TypeSleep
					}
				})
				return sleeping
			}
			seated := func() (sitting bool) {
				onPlayerQueue(t, srv, objID, func(pc *player.Character) { sitting = pc.Seated() })
				return sitting
			}
			switch tc.posture {
			case "sleeping":
				onPlayerQueue(t, srv, objID, func(pc *player.Character) {
					e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Sleep", Time: 60})
					if err != nil {
						t.Errorf("effect.New(Sleep): %v", err)
						return
					}
					e.Effector, e.Effected = pc, pc
					pc.EffectList().Add(e)
				})
				if !asleep() {
					t.Fatal("the player is not asleep before the tick")
				}
			case "sitting":
				c.Send(encodeRequestChangeWaitType(false))
				readMatching(t, c, time.Second, "sit ChangeWaitType", isWaitType(serverpackets.WaitSitting))
				srv.AdvanceUntil(t, "the player seated before the tick", seated)
			}
			drainUntilQuiet(t, c)
			hp := srv.PlayerCurrentHP(t, objID)
			if hp <= upkeep {
				t.Fatalf("player HP %d before the tick, want more than the %d upkeep", hp, upkeep)
			}

			srv.Advance(t, 1100*time.Millisecond)
			srv.TickEffects()

			if !upkeeping() {
				t.Fatal("the DamOverTime effect ended on its tick, want it still running")
			}
			log := readFrameLog(c)
			if got := srv.PlayerCurrentHP(t, objID); got != hp-upkeep {
				t.Fatalf("player HP after the tick = %d, want %d - %d upkeep", got, hp, upkeep)
			}
			switch tc.posture {
			case "sleeping":
				if got := asleep(); got != tc.toggle {
					t.Fatalf("asleep after the tick = %v, want %v (toggle upkeep %v)", got, tc.toggle, tc.toggle)
				}
			case "sitting":
				if got := seated(); got != tc.toggle {
					t.Fatalf("seated after the tick = %v, want %v (toggle upkeep %v)", got, tc.toggle, tc.toggle)
				}
				stoodUp := log.index(isWaitType(serverpackets.WaitStanding)) >= 0
				if stoodUp == tc.toggle {
					t.Fatalf("stand-up ChangeWaitType after the tick = %v, want %v", stoodUp, !tc.toggle)
				}
			}
			if at := log.index(isSystemMessage(serverpackets.SystemMessageS1GaveYouS2Dmg)); at >= 0 {
				t.Fatalf("player read S1_GAVE_YOU_S2_DMG at frame %d for its own DOT tick, want none", at)
			}
		})
	}
}
