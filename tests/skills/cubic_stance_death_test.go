package skills

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// stormCubicFireSkill is the Storm Cubic's only action skill (Cubic id 1,
// skill 4049). The fixture makes it a DUMMY: these tests only watch whether
// the cubic acts, through its MagicSkillUse broadcast.
const stormCubicFireSkill = 4049

// bootStanceCubicSummoner is bootCubicSummoner with a Storm Cubic that
// always activates, its action skill, and a driven attack-stance clock.
func bootStanceCubicSummoner(t *testing.T, nowMS *atomic.Int64) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	defs := cubicSummonSkills(900*time.Second, 900*time.Second)
	for i := range defs {
		if defs[i].ID == summonStormCubicSkill {
			defs[i].CubicActivationChance = 100
		}
	}
	defs = append(defs, modelskill.Definition{ID: stormCubicFireSkill, Level: 1, SkillType: "DUMMY"})
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
		gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, summonLifeCubicSkill, 1)
	seedKnownSkill(t, srv, objID, summonStormCubicSkill, 1)
	startInWorld(t, c)
	setPlayerRollSource(t, srv, objID, func(int) int { return 0 })
	return srv, c, objID
}

func summonStormCubic(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(summonStormCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Storm)}) {
		t.Fatalf("cubics after the Storm summon = %v, want [%d]", got, cubic.Storm)
	}
}

// enterStanceOn selects and attacks hostile until a swing lands, putting
// the player in attack stance, which starts every non-Life cubic's action
// tick.
func enterStanceOn(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, hostile *npc.Hostile) {
	t.Helper()
	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	srv.AdvanceUntil(t, "opening swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	if !inAttackStance(t, srv, objID) {
		t.Fatal("a landed swing left the player out of attack stance")
	}
}

func inAttackStance(t *testing.T, srv *gameservertest.Server, objID int32) bool {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world player %d missing", objID)
	}
	actor, ok := obj.(task.AttackStanceActor)
	if !ok {
		t.Fatalf("world player %d = %T is not an attack-stance actor", objID, obj)
	}
	return srv.AttackStance.InAttackStance(actor)
}

func cubicFires(t *testing.T, frames [][]byte) int {
	t.Helper()
	n := 0
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeMagicSkillUse && magicSkillUseSkill(t, frame) == stormCubicFireSkill {
			n++
		}
	}
	return n
}

// TestStormCubicKeepsFiringAcrossTeleportUntilStanceExpires: a teleport
// keeps the attack stance (Creature.teleportTo, Creature.java:386-429, runs
// only abortAll(true)), so Cubic.fireAction's stance gate keeps passing and
// a Storm Cubic keeps acting on the owner's target after the jump. It goes
// idle (stopAction) only once the stance times out.
func TestStormCubicKeepsFiringAcrossTeleportUntilStanceExpires(t *testing.T) {
	t.Parallel()
	var nowMS atomic.Int64
	srv, c, objID := bootStanceCubicSummoner(t, &nowMS)
	summonStormCubic(t, srv, c, objID)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	enterStanceOn(t, srv, c, objID, hostile)
	drainUntilQuiet(t, c)

	teleportAndAppear(t, srv, c, objID)
	if !inAttackStance(t, srv, objID) {
		t.Fatal("teleport ended the attack stance")
	}
	// Leaving the old neighborhood dropped the selection; pick the monster
	// again without attacking it.
	c.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainUntilQuiet(t, c)
	srv.Advance(t, (lifeCubicInterval+1)*time.Second)
	if n := cubicFires(t, queueFrames(t, c)); n == 0 {
		t.Fatal("Storm Cubic stopped acting after the teleport; the stance it fires under was dropped")
	}

	nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	srv.Settle(t)
	if inAttackStance(t, srv, objID) {
		t.Fatal("stance timeout left the player in the tracker")
	}
	drainUntilQuiet(t, c)
	srv.Advance(t, 3*lifeCubicInterval*time.Second)
	if n := cubicFires(t, queueFrames(t, c)); n != 0 {
		t.Fatalf("Storm Cubic acted %d times after the stance expired, want 0", n)
	}
}

// dieOnQueue kills objID on its own queue.
func dieOnQueue(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if !pc.Die(nil) {
			t.Error("Die() = false for a living player")
		}
	})
	srv.Settle(t)
}

func reviveOnQueue(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if !pc.Revive() {
			t.Error("Revive() = false for a dead player")
		}
	})
	srv.Settle(t)
}

// TestOwnerDeathRemovesIdleStormCubic: Player.doDie runs
// CubicList.stopCubics(false) (Player.java:2656-2657), which stops and
// removes every cubic, idle or not. A Storm Cubic whose owner dies out of
// combat stance is gone at the death and does not come back with the
// revive: the next stance entry finds no cubic to start.
func TestOwnerDeathRemovesIdleStormCubic(t *testing.T) {
	t.Parallel()
	var nowMS atomic.Int64
	srv, c, objID := bootStanceCubicSummoner(t, &nowMS)
	summonStormCubic(t, srv, c, objID)
	if inAttackStance(t, srv, objID) {
		t.Fatal("player in attack stance before the death; the case needs an idle cubic")
	}

	dieOnQueue(t, srv, objID)
	if got := liveCubicIDs(t, srv, objID); len(got) != 0 {
		t.Fatalf("cubics after the death = %v, want none", got)
	}
	reviveOnQueue(t, srv, objID)
	drainUntilQuiet(t, c)
	if got := liveCubicIDs(t, srv, objID); len(got) != 0 {
		t.Fatalf("cubics after the revive = %v, want none", got)
	}

	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	enterStanceOn(t, srv, c, objID, hostile)
	srv.Advance(t, 2*lifeCubicInterval*time.Second)
	if n := cubicFires(t, queueFrames(t, c)); n != 0 {
		t.Fatalf("Storm Cubic acted %d times after its owner died and revived, want 0", n)
	}
}

// TestOwnerDeathRemovesLifeCubicSilently: the Life Cubic is removed at the
// death itself, not on its next action tick, and the removal broadcasts
// nothing (stopCubics(false); Cubic.stop(false), Cubic.java:272-283): no
// UserInfo beyond the death's own frames, and none one interval later from
// a lazy expiry.
func TestOwnerDeathRemovesLifeCubicSilently(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootCubicSummoner(t, 900*time.Second, 900*time.Second)
	c.Send(encodeRequestMagicSkillUse(summonLifeCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	if got := liveCubicIDs(t, srv, objID); !slices.Equal(got, []int{int(cubic.Life)}) {
		t.Fatalf("cubics after the Life summon = %v, want [%d]", got, cubic.Life)
	}

	dieOnQueue(t, srv, objID)
	if got := liveCubicIDs(t, srv, objID); len(got) != 0 {
		t.Fatalf("cubics right after the death = %v, want none", got)
	}
	// The death's own frames run up to and including Die; the cubic goes
	// with it, so nothing after Die may refresh the owner's appearance.
	frames := queueFrames(t, c)
	die := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeDie })
	if die < 0 {
		t.Fatalf("death sent no Die (frames %x)", opcodesOf(frames))
	}
	if n := countOpcode(frames[die+1:], serverpackets.OpcodeUserInfo); n != 0 {
		t.Fatalf("death sent %d UserInfo after Die, want 0: the cubic removal broadcasts nothing (frames %x)", n, opcodesOf(frames))
	}

	srv.Advance(t, 2*lifeCubicInterval*time.Second)
	frames = queueFrames(t, c)
	if n := countOpcode(frames, serverpackets.OpcodeUserInfo); n != 0 {
		t.Fatalf("%d UserInfo after the death, want 0: no lazy cubic expiry is left", n)
	}
	if n := cubicHeals(t, frames); n != 0 {
		t.Fatalf("Life Cubic healed %d times after its owner died, want 0", n)
	}
}
