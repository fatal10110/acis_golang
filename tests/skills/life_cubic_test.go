package skills

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
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
// fill its one-cubic list and then summon past it. lifeLifetime and
// stormLifetime are the two cubics' granted lifetimes.
func cubicSummonSkills(lifeLifetime, stormLifetime time.Duration) []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: summonLifeCubicSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Life),
			CubicActivationTime: lifeCubicInterval, SummonTotalLifeTime: int(lifeLifetime / time.Millisecond),
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

func bootCubicSummoner(t *testing.T, lifeLifetime, stormLifetime time.Duration) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, cubicSummonSkills(lifeLifetime, stormLifetime))),
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
	srv, c, objID := bootCubicSummoner(t, 900*time.Second, 900*time.Second)
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
	srv, c, objID := bootCubicSummoner(t, 900*time.Second, lifetime)

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

// teleportAndAppear teleports the online player a short hop through the
// production teleport path, then completes it with the client's Appearing.
func teleportAndAppear(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("world player %d = %T is not an online character", objID, obj)
	}
	x, y, z := pc.Position()
	pc.TeleportTo(x+300, y, z, 0)
	if n := countOpcode(queueFrames(t, c), serverpackets.OpcodeTeleportToLocation); n == 0 {
		t.Fatal("teleport sent no TeleportToLocation")
	}
	c.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	drainUntilQuiet(t, c)
}

func cubicHeals(t *testing.T, frames [][]byte) int {
	t.Helper()
	n := 0
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeMagicSkillUse && magicSkillUseSkill(t, frame) == lifeCubicHealSkill {
			n++
		}
	}
	return n
}

// TestLifeCubicKeepsHealingAndExpiresAcrossTeleport: a teleport does not
// touch cubics. Player.teleportTo has no cubic call; CubicList.stopCubics is
// reached only from Player.doDie, dropAllSummons and setActiveClass
// (Player.java:2657,5238,5870). The Life Cubic's fixed-rate action task
// (Cubic.doAction, scheduleAtFixedRate) and its one-shot disappear task
// (scheduled once in the Cubic constructor for totalLifeTime) both keep
// running: the cubic heals once per interval after the jump, never twice,
// and stops at its original lifetime, broadcasting UserInfo (Cubic.stop).
//
// Times in the comments are the actor clock since the cast; each quiet read
// below also lets 300ms pass on the driven clock. Every check sits at least
// 0.5s away from a tick, a heal landing or an expiry.
func TestLifeCubicKeepsHealingAndExpiresAcrossTeleport(t *testing.T) {
	t.Parallel()
	const lifeLifetime = 19 * time.Second
	srv, c, objID := bootCubicSummoner(t, lifeLifetime, 900*time.Second)
	setPlayerRollSource(t, srv, objID, func(int) int { return 0 })
	// Down to 1 HP, so the three heals below never top the owner off
	// (pickFriendlyTarget skips a full-HP owner).
	if _, maxHP := damageToHealHeadroom(t, srv, objID, int32(srv.PlayerMaxHP(t, objID)-1)); maxHP <= 1+3*lifeCubicHealPower {
		t.Fatalf("max HP %d leaves no room for three %d-point heals", maxHP, lifeCubicHealPower)
	}
	drainUntilQuiet(t, c)

	// Granted at 0s: ticks at 5s, 10s, 15s, each heal landing 2s later;
	// expiry at 19s. A lifetime restarted by the teleport would end at ~21s.
	c.Send(encodeRequestMagicSkillUse(summonLifeCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	teleportAndAppear(t, srv, c, objID) // ~1.3s, done by ~1.9s
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Life)}) {
		t.Fatalf("cubics after the teleport = %v, want [%d]", got, cubic.Life)
	}

	hpBefore := srv.PlayerCurrentHP(t, objID)
	srv.Advance(t, 6*time.Second) // ~7.9s: the 5s tick and its heal at 7s
	if n := cubicHeals(t, queueFrames(t, c)); n != 1 {
		t.Fatalf("Life Cubic heals in the first interval after the teleport = %d, want 1", n)
	}
	if hp := srv.PlayerCurrentHP(t, objID); hp != hpBefore+lifeCubicHealPower {
		t.Fatalf("HP after the first post-teleport heal = %d, want %d", hp, hpBefore+lifeCubicHealPower)
	}
	srv.Advance(t, 9500*time.Millisecond) // ~17.7s: the 10s and 15s ticks
	if n := cubicHeals(t, queueFrames(t, c)); n != 2 {
		t.Fatalf("Life Cubic heals over the next two intervals = %d, want 2 (one timer, not two)", n)
	}
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Life)}) {
		t.Fatalf("cubics before the original lifetime ends = %v, want [%d]", got, cubic.Life)
	}

	srv.Advance(t, 1500*time.Millisecond) // ~19.5s
	frames := queueFrames(t, c)
	if got := liveCubicIDs(t, srv, objID); len(got) != 0 {
		t.Fatalf("cubics after the original lifetime = %v, want none", got)
	}
	if countOpcode(frames, serverpackets.OpcodeUserInfo) == 0 {
		t.Fatalf("expiry sent no UserInfo (frames %x)", opcodesOf(frames))
	}
	srv.Advance(t, 3*lifeCubicInterval*time.Second)
	if n := cubicHeals(t, queueFrames(t, c)); n != 0 {
		t.Fatalf("expired Life Cubic healed %d more times, want 0", n)
	}
}

// TestCubicKeepsItsDisappearTimerAcrossTeleport: a non-Life cubic's
// disappear task (Cubic constructor, ThreadPool.schedule(this::stop,
// totalLifeTime)) is not touched by Player.teleportTo, so the cubic still
// expires at its original lifetime after a teleport, with the UserInfo
// broadcast of Cubic.stop. Times as in the Life Cubic test above.
func TestCubicKeepsItsDisappearTimerAcrossTeleport(t *testing.T) {
	t.Parallel()
	const stormLifetime = 10 * time.Second
	srv, c, objID := bootCubicSummoner(t, 900*time.Second, stormLifetime)

	// Granted at 0s, expiry at 10s; restarted by the teleport it would end
	// at ~14s.
	c.Send(encodeRequestMagicSkillUse(summonStormCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	srv.Advance(t, 2*time.Second)
	drainUntilQuiet(t, c)
	teleportAndAppear(t, srv, c, objID) // ~3.6s, done by ~4.2s

	srv.Advance(t, 5*time.Second)
	drainUntilQuiet(t, c) // ~9.5s
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Storm)}) {
		t.Fatalf("cubics at ~9.5s = %v, want [%d] still held", got, cubic.Storm)
	}

	srv.Advance(t, time.Second) // ~10.5s
	frames := queueFrames(t, c)
	if got := liveCubicIDs(t, srv, objID); len(got) != 0 {
		t.Fatalf("cubics at ~10.5s = %v, want none: the teleport kept the 10s lifetime", got)
	}
	if countOpcode(frames, serverpackets.OpcodeUserInfo) == 0 {
		t.Fatalf("expiry sent no UserInfo (frames %x)", opcodesOf(frames))
	}
}
