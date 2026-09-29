package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	summonLifeCubicSkill = 67
	lifeCubicHealSkill   = 4051
	lifeCubicHealPower   = 10
	// lifeCubicInterval is the granted cubic's action tick, long enough
	// for the summon cast's own frames to be drained before the first one.
	lifeCubicInterval = 5
)

// TestLifeCubicHealSendsOwnerStatusBeforeRejuvenating grants a damaged
// player a Life Cubic and lets it heal its owner. Cubic.useHealSkill
// (Cubic.java:364-373) restores HP through addHp, whose setHp broadcast is,
// for a player, PlayerStatus.broadcastStatusUpdate (PlayerStatus.java:408-416):
// one self StatusUpdate carrying CUR_HP, CUR_MP, CUR_CP and MAX_CP, then
// REJUVENATING_HP, and no further StatusUpdate after the message.
func TestLifeCubicHealSendsOwnerStatusBeforeRejuvenating(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: summonLifeCubicSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Life),
				CubicActivationTime: lifeCubicInterval, SummonTotalLifeTime: 900_000,
				StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
			},
			{ID: lifeCubicHealSkill, Level: 1, Power: lifeCubicHealPower},
		})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, summonLifeCubicSkill, 1)
	startInWorld(t, c)

	worldObj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	owner, ok := worldObj.(interface{ SetRollSource(func(int) int) })
	if !ok {
		t.Fatalf("world player %d = %T, want SetRollSource", objID, worldObj)
	}
	// Every heal-chance roll passes (Cubic.pickFriendlyTarget: i0 > chance
	// skips the heal).
	owner.SetRollSource(func(int) int { return 0 })

	before, _ := damageToHealHeadroom(t, srv, objID, 2*lifeCubicHealPower)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(summonLifeCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	if hp := srv.PlayerCurrentHP(t, objID); hp != before {
		t.Fatalf("HP = %d before the cubic's first tick, want untouched %d", hp, before)
	}

	want := before + lifeCubicHealPower
	srv.AdvanceUntil(t, "the Life Cubic heal landing", func() bool { return srv.PlayerCurrentHP(t, objID) == want })

	statuses, _ := selfStatusesThenMessage(t, c, objID, serverpackets.SystemMessageRejuvenatingHP, 1)
	assertCasterStatus(t, srv, statuses[0], objID, want, srv.PlayerCurrentMP(t, objID))
}
