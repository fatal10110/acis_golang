package skills

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
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

const (
	summonStormCubicSkill = 10
	stormCubicMPCost      = 5
)

// cubicSummonSkills is a self Life Cubic and a self Storm Cubic summon, both
// instant, plus the Life Cubic's heal, so a caster with no Cubic Mastery can
// fill its one-cubic list and then summon past it. stormLifetime is the
// Storm Cubic's granted lifetime.
func cubicSummonSkills(stormLifetime time.Duration) []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: summonLifeCubicSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Life),
			CubicActivationTime: lifeCubicInterval, SummonTotalLifeTime: 900_000,
			StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{
			ID: summonStormCubicSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Storm), MPConsume: stormCubicMPCost,
			CubicActivationTime: lifeCubicInterval, SummonTotalLifeTime: int(stormLifetime / time.Millisecond),
			StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{ID: lifeCubicHealSkill, Level: 1, Power: lifeCubicHealPower},
	}
}

func bootCubicSummoner(t *testing.T, stormLifetime time.Duration) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, cubicSummonSkills(stormLifetime))),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, summonLifeCubicSkill, 1)
	seedKnownSkill(t, srv, objID, summonStormCubicSkill, 1)
	startInWorld(t, c)
	return srv, c, objID
}

func liveCubicIDs(t *testing.T, srv *gameservertest.Server, objID int32) []int {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	holder, ok := obj.(interface{ CubicIDs() []int })
	if !ok {
		t.Fatalf("world player %d = %T, want CubicIDs", objID, obj)
	}
	return holder.CubicIDs()
}

func assertNoCubicSummoningFailed(t *testing.T, frames [][]byte) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id := int32(binary.LittleEndian.Uint32(frame[1:5])); id == serverpackets.SystemMessageCubicSummoningFailed {
			t.Fatal("server sent CUBIC_SUMMONING_FAILED; a full cubic list never refuses the cast")
		}
	}
}

// TestSelfCubicCastPastFullListEvictsOldest: L2SkillSummon.checkCondition
// (L2SkillSummon.java:64-105), the only sender of CUBIC_SUMMONING_FAILED, has
// no caller; PlayerCast.canCast's SUMMON case (PlayerCast.java:268-290) gates
// only non-cubic summons. The cast therefore runs and pays its cost, and at
// hit CubicList.addOrRefreshCubic (CubicList.java:45-59) finds the list full
// (isFull: size() > Cubic Mastery level, :110-113 — 1 > 0 here), polls the
// oldest cubic and stops it (Cubic.stop: action, disappear and cast tasks
// cancelled), then admits the new one. The evicted Life Cubic must never
// heal again.
func TestSelfCubicCastPastFullListEvictsOldest(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootCubicSummoner(t, 900*time.Second)
	setPlayerRollSource(t, srv, objID, func(int) int { return 0 })
	damageToHealHeadroom(t, srv, objID, int32(srv.PlayerMaxHP(t, objID)/2))
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(summonLifeCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Life)}) {
		t.Fatalf("cubics after the Life summon = %v, want [%d]", got, cubic.Life)
	}
	// The Life Cubic is live: its next tick heals the damaged owner.
	srv.Advance(t, lifeCubicInterval*time.Second)
	if frames := queueFrames(t, c); !castStartedFor(t, frames, lifeCubicHealSkill) {
		t.Fatal("Life Cubic never healed before the eviction; the fixture cannot prove it stops")
	}

	mpBefore := srv.PlayerCurrentMP(t, objID)
	c.Send(encodeRequestMagicSkillUse(summonStormCubicSkill, false, false))
	srv.Advance(t, time.Second)
	frames := queueFrames(t, c)
	assertNoCubicSummoningFailed(t, frames)
	if !castStartedFor(t, frames, summonStormCubicSkill) {
		t.Fatalf("Storm Cubic summon on a full list never started (frames %x)", opcodesOf(frames))
	}
	if mp := srv.PlayerCurrentMP(t, objID); mp > mpBefore-stormCubicMPCost {
		t.Fatalf("MP after the Storm summon = %d, want at most %d (cost %d paid)", mp, mpBefore-stormCubicMPCost, stormCubicMPCost)
	}
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Storm)}) {
		t.Fatalf("cubics after summoning past the cap = %v, want the oldest (Life) evicted: [%d]", got, cubic.Storm)
	}

	srv.Advance(t, 3*lifeCubicInterval*time.Second)
	if castStartedFor(t, queueFrames(t, c), lifeCubicHealSkill) {
		t.Fatal("evicted Life Cubic kept healing; its runtime was not stopped")
	}
}

// TestSelfCubicRecastOnFullListRefreshes: recasting the held cubic while the
// list is full is not refused either. addOrRefreshCubic (CubicList.java:47-49)
// only restarts that cubic's disappear task (Cubic.refreshDisappearTask,
// cancel then reschedule for the full lifetime) and broadcasts nothing: the
// cubic outlives its first grant and expires one lifetime after the recast.
func TestSelfCubicRecastOnFullListRefreshes(t *testing.T) {
	t.Parallel()
	const lifetime = 10 * time.Second
	srv, c, objID := bootCubicSummoner(t, lifetime)

	c.Send(encodeRequestMagicSkillUse(summonStormCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	srv.Advance(t, 5*time.Second)
	drainUntilQuiet(t, c)

	// Six seconds into a ten-second grant: the recast restarts the lifetime.
	c.Send(encodeRequestMagicSkillUse(summonStormCubicSkill, false, false))
	srv.Advance(t, time.Second)
	frames := queueFrames(t, c)
	assertNoCubicSummoningFailed(t, frames)
	if !castStartedFor(t, frames, summonStormCubicSkill) {
		t.Fatalf("Storm Cubic recast on a full list never started (frames %x)", opcodesOf(frames))
	}
	if n := countOpcode(frames, serverpackets.OpcodeUserInfo); n != 0 {
		t.Fatalf("refresh sent %d UserInfo, want 0 (only a newly admitted cubic broadcasts)", n)
	}

	// Past the first grant's end (10s) but inside the refreshed one (6s+10s).
	srv.Advance(t, 6*time.Second)
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Storm)}) {
		t.Fatalf("cubics after the first grant's lifetime = %v, want [%d] still held by the refresh", got, cubic.Storm)
	}

	// Past the refreshed lifetime: the cubic expires.
	srv.Advance(t, 5*time.Second)
	if got := liveCubicIDs(t, srv, objID); len(got) != 0 {
		t.Fatalf("cubics after the refreshed lifetime = %v, want none", got)
	}
}
