package skills

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestToggleSendsUsePerEffectTemplate switches on a toggle carrying two
// effects, its buff and its MP drain: each effect created tells the player
// USE_S1 naming the toggle, ahead of the icon refresh its own add sends,
// after the MP status.
func TestToggleSendsUsePerEffectTemplate(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: 288, Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
			MPConsume: 12, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{
				{Name: "Buff", Time: 60, Icon: true},
				{Name: "ManaDamOverTime", Count: 0x7fffffff, Time: 5, Value: 1},
			},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 288, 1)
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(288, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMagicSkillUse, "toggle activation")
	assertCasterMPStatus(t, srv, c.Read(), objID, 18)
	for i := 1; i <= 2; i++ {
		assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageUseS1, 288, 1)
		if entries := readAbnormalStatusUpdateEntries(t, c); len(entries) != 1 || entries[0].SkillID != 288 {
			t.Fatalf("AbnormalStatusUpdate after effect %d = %+v, want the toggle's one icon", i, entries)
		}
	}
	drainUntilQuiet(t, c)
}

// failedStartEffect builds a held-but-unstarted debuff of the named kind,
// one tick of 30 s, as the reference's Betray and Spoil templates run.
func failedStartEffect(t *testing.T, name string, skillID modelskill.ID) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: skillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: name, Count: 1, Time: 30, Icon: true})
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	return e
}

// TestFailedStartEffectHeldUntilFirstTick lands effects whose start fails on
// a player: Betray needs a summon and Spoil a monster. Each is held all the
// same — the player sees its icon and its flag counts — until its first tick
// 30 s later ends it with the worn-off message.
func TestFailedStartEffectHeldUntilFirstTick(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		skillID modelskill.ID
		flag    effect.Flag
	}{
		{name: "Betray", skillID: 1380, flag: effect.FlagBetrayed},
		{name: "Spoil", skillID: 254},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			drainUntilQuiet(t, c)

			var e *effect.Effect
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				e = failedStartEffect(t, tc.name, tc.skillID)
				e.Effector, e.Effected = pc, pc
				pc.EffectList().Add(e)
			})
			entries := readAbnormalStatusUpdateEntries(t, c)
			if len(entries) != 1 || entries[0].SkillID != int32(tc.skillID) {
				t.Fatalf("AbnormalStatusUpdate after a failed start = %+v, want the %s icon", entries, tc.name)
			}
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				if !e.InUse() || (tc.flag != 0 && !pc.EffectList().IsAffected(tc.flag)) {
					t.Errorf("%s in use = %v, flag affected = %v, want it held", tc.name, e.InUse(), pc.EffectList().IsAffected(tc.flag))
				}
			})
			drainUntilQuiet(t, c)

			srv.AdvanceWithEffects(t, 30*time.Second)
			assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1HasWornOff, int32(tc.skillID), 1)
			if entries := readAbnormalStatusUpdateEntries(t, c); len(entries) != 0 {
				t.Fatalf("AbnormalStatusUpdate after the first tick = %+v, want none", entries)
			}
			onPlayerQueue(t, srv, objID, func(pc *player.Character) {
				if n := len(pc.EffectList().All()); n != 0 {
					t.Errorf("player holds %d effects after the first tick, want none", n)
				}
			})
		})
	}
}

// TestTwoBuffersLandSameBuffLeaveNoStats has two casters land the same buff
// on one player at once, each from its own goroutine as a caster's queue
// does. However the adds interleave, the player ends up holding one copy,
// and once it is removed its attack is back where it started.
func TestTwoBuffersLandSameBuffLeaveNoStats(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	var pc *player.Character
	var base float64
	onPlayerQueue(t, srv, objID, func(p *player.Character) { pc, base = p, p.PAtk() })
	might := func() *effect.Effect {
		e, err := effect.New(effect.Skill{ID: 1068, Level: 1}, modelskill.EffectTemplate{
			Name: "Buff", Count: 1, Time: 1200, Icon: true, StackType: "pa_up", StackOrder: 1,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 100}},
		})
		if err != nil {
			t.Fatalf("effect.New(Buff): %v", err)
		}
		e.Effector, e.Effected = pc, pc
		return e
	}
	buffs := []*effect.Effect{might(), might()}
	var wg sync.WaitGroup
	for _, e := range buffs {
		wg.Go(func() { effect.Attach(e, pc, pc) })
	}
	wg.Wait()
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(p *player.Character) {
		held := p.EffectList().All()
		if len(held) != 1 || !held[0].InUse() {
			t.Fatalf("player holds %d copies of the buff (first in use: %v), want one active", len(held), len(held) > 0 && held[0].InUse())
		}
		if got := p.PAtk(); got <= base {
			t.Errorf("buffed P. Atk = %v, want above the base %v", got, base)
		}
		p.EffectList().Remove(held[0])
		if got := p.PAtk(); got != base {
			t.Errorf("P. Atk after the buff left = %v, want the base %v: a replaced copy left its stats attached", got, base)
		}
	})
}
