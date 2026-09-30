package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// zeroDOTDebuff is a debuff whose DamOverTime ticks for no damage, shaped
// like the datapack's augmentation skill 3196 Item Skill: Bleed
// (DamOverTime val="0").
func zeroDOTDebuff() modelskill.Definition {
	return modelskill.Definition{
		ID: feedbackStrikeSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
		SkillType: "DEBUFF", EffectType: "DEBUFF", Debuff: true, BaseLandRate: 100, IgnoreResists: true,
		Effects: []modelskill.EffectTemplate{{Name: "DamOverTime", Value: 0, Count: 9, Time: 1}},
	}
}

// TestZeroDamageDOTTickWakesAndStandsPlayer lands a zero-damage
// damage-over-time debuff from one player on another (issue #2864), then
// puts the victim to sleep or sits it down. Reference: PlayerStatus.reduceHp
// (PlayerStatus.java:95-150) runs its !isHPConsumption block with no value
// check, so every tick, zero included, stops SLEEP and stands a sitting
// player up; the zero value then passes through the CP branch of another
// playable's hit, rewriting CP unchanged, and writes no HP. A DOT tick never
// reports S1_GAVE_YOU_S2_DMG.
func TestZeroDamageDOTTickWakesAndStandsPlayer(t *testing.T) {
	t.Parallel()
	for _, posture := range []string{"sleeping", "sitting"} {
		t.Run(posture, func(t *testing.T) {
			t.Parallel()
			def := zeroDOTDebuff()
			p := bootPVPPair(t, def, 60)
			bleeding := func() (held bool) {
				onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) {
					for _, e := range pc.EffectList().All() {
						held = held || (e.Type == effect.TypeDamOverTime && e.Skill.ID == def.ID)
					}
				})
				return held
			}
			p.strike(t, int32(def.ID), 1, bleeding)
			drainUntilQuiet(t, p.c)

			asleep := func() (sleeping bool) {
				onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) {
					for _, e := range pc.EffectList().All() {
						sleeping = sleeping || e.Type == effect.TypeSleep
					}
				})
				return sleeping
			}
			seated := func() (sitting bool) {
				onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) { sitting = pc.Seated() })
				return sitting
			}
			switch posture {
			case "sleeping":
				onPlayerQueue(t, p.srv, p.victimID, func(pc *player.Character) {
					e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Sleep", Time: 60})
					if err != nil {
						t.Errorf("effect.New(Sleep): %v", err)
						return
					}
					e.Effector, e.Effected = pc, pc
					pc.EffectList().Add(e)
				})
				if !asleep() {
					t.Fatal("the Victim is not asleep before the tick")
				}
			case "sitting":
				p.vc.Send(encodeRequestChangeWaitType(false))
				readMatching(t, p.vc, time.Second, "sit ChangeWaitType", isWaitType(serverpackets.WaitSitting))
				p.srv.Advance(t, sitStandDelay)
				if !seated() {
					t.Fatal("the Victim is not seated before the tick")
				}
			}
			drainUntilQuiet(t, p.vc)
			hp, cp := p.srv.PlayerCurrentHP(t, p.victimID), p.srv.PlayerCurrentCP(t, p.victimID)

			p.srv.Advance(t, 1100*time.Millisecond)
			p.srv.TickEffects()

			if !bleeding() {
				t.Fatal("the zero-damage DOT ended on its first tick, want it still running")
			}
			log := readFrameLog(p.vc)
			switch posture {
			case "sleeping":
				if asleep() {
					t.Fatal("the Victim still sleeps after a zero-damage DOT tick, want it woken")
				}
			case "sitting":
				if seated() {
					t.Fatal("the Victim still sits after a zero-damage DOT tick, want it standing up")
				}
				if log.index(isWaitType(serverpackets.WaitStanding)) < 0 {
					t.Fatal("the Victim never read its stand-up ChangeWaitType after the zero-damage DOT tick")
				}
			}
			if got := p.srv.PlayerCurrentHP(t, p.victimID); got != hp {
				t.Fatalf("Victim HP after a zero-damage DOT tick = %d, want unchanged %d", got, hp)
			}
			if got := p.srv.PlayerCurrentCP(t, p.victimID); got != cp {
				t.Fatalf("Victim CP after a zero-damage DOT tick = %d, want unchanged %d", got, cp)
			}
			if at := log.index(isSystemMessage(serverpackets.SystemMessageS1GaveYouS2Dmg)); at >= 0 {
				t.Fatalf("Victim read S1_GAVE_YOU_S2_DMG at frame %d for a DOT tick, want none", at)
			}
			status := log.index(func(frame []byte) bool {
				_, ok := statusValue(frame, p.victimID, serverpackets.StatusCurrentCP)
				return ok
			})
			if status < 0 {
				t.Fatal("the Victim read no StatusUpdate for the zero-damage DOT tick's unchanged CP write")
			}
		})
	}
}
