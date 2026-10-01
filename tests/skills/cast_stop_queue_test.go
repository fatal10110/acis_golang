package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Stop-queue fixture: a long magic cast to stop, and a physical skill to
// queue behind it that Mute leaves castable.
const (
	stopQueueMagicSkillID    int32 = 1011
	stopQueuePhysicalSkillID int32 = 3
)

func stopQueueSkills() []modelskill.Definition {
	return []modelskill.Definition{
		{
			ID: modelskill.ID(stopQueueMagicSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: queueHitTime, StaticHitTime: true, SkillType: "DUMMY", Magic: true,
		},
		{
			ID: modelskill.ID(stopQueuePhysicalSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: queueHitTime, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, MPConsume: 5, SkillType: "DUMMY",
		},
	}
}

func encodeRequestTargetCancel(unselect uint16) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestTargetCancel)
	w.WriteUint16(unselect)
	return w.Bytes()
}

// bootStopQueue boots a caster inside the long magic cast with the physical
// skill queued behind it, every frame read so far.
func bootStopQueue(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, stopQueueSkills())),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, def := range stopQueueSkills() {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(stopQueueMagicSkillID, false, false))
	if skill := magicSkillUseSkill(t, c.Read()); skill != stopQueueMagicSkillID {
		t.Fatalf("magic cast MagicSkillUse skill = %d, want %d", skill, stopQueueMagicSkillID)
	}
	drainUntilQuiet(t, c)
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("magic cast not in flight")
	}
	assertQueuedWithActionFailed(t, c, stopQueuePhysicalSkillID)
	return srv, c, objID
}

// assertQueuedSkillDropped asserts the queued physical skill never started:
// no MagicSkillUse for it, no cast in flight, no reuse stamped and no MP
// spent, even once the stopped cast's own hit time has passed.
func assertQueuedSkillDropped(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, frames [][]byte, mpBefore int) {
	t.Helper()
	if castStartedFor(t, frames, stopQueuePhysicalSkillID) {
		t.Fatalf("queued skill started after the stop: opcodes %x", opcodesOf(frames))
	}
	srv.Advance(t, 2*queueHitTime*time.Millisecond)
	if later := queueFrames(t, c); castStartedFor(t, later, stopQueuePhysicalSkillID) {
		t.Fatalf("queued skill started later: opcodes %x", opcodesOf(later))
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("a cast is in flight after the stop")
	}
	key := cast.ReuseKey(stopQueueSkills()[1])
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if pc.SkillDisabled(key) {
			t.Error("queued skill's reuse was stamped")
		}
	})
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after the stop = %d, want unchanged %d", got, mpBefore)
	}
}

// assertStopReply asserts frames are MagicSkillCanceled, then the two
// ActionFailed of a stop with a queued cast, then CASTING_INTERRUPTED when
// interrupted.
//
// CreatureCast.stop (CreatureCast.java:404-434) broadcasts
// MagicSkillCanceled and notifies FINISHED_CASTING while _isCastingNow is
// still true; PlayableAI.onEvtFinishedCasting (PlayableAI.java:43-63) runs
// the queued CAST through doIntention, and PlayerAI.thinkCast
// (PlayerAI.java:219-226) sees isCastingNow, goes idle and answers
// ActionFailed. Only then is the flag cleared, PlayableCast.stop's
// tryToIdle runs (silent here) and PlayerCast.stop (PlayerCast.java:381-387)
// answers its own ActionFailed. CreatureCast.interrupt sends
// CASTING_INTERRUPTED after the whole stop.
func assertStopReply(t *testing.T, frames [][]byte, interrupted bool) {
	t.Helper()
	want := []byte{serverpackets.OpcodeMagicSkillCanceled, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	if interrupted {
		want = append(want, serverpackets.OpcodeSystemMessage)
	}
	got := opcodesOf(frames)
	if string(got) != string(want) {
		t.Fatalf("stop with a queued skill sent opcodes %x, want %x", got, want)
	}
	if interrupted {
		assertStaticSystemMessage(t, frames[3], serverpackets.SystemMessageCastingInterrupted)
	}
}

// TestMuteStopDropsQueuedSkill pins Mute stopping a magic cast with a
// physical skill queued behind it: the queued skill, which Mute leaves
// castable, is dropped with ActionFailed instead of starting.
func TestMuteStopDropsQueuedSkill(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootStopQueue(t)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		e, err := effect.New(effect.Skill{ID: 1064, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Mute", Time: 30})
		if err != nil {
			t.Errorf("effect.New(Mute): %v", err)
			return
		}
		e.Effector, e.Effected = pc, pc
		pc.EffectList().Add(e)
	})
	frames := queueFrames(t, c)
	var stop [][]byte
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeMagicSkillCanceled || frame[0] == serverpackets.OpcodeActionFailed ||
			frame[0] == serverpackets.OpcodeMagicSkillUse {
			stop = append(stop, frame)
		}
	}
	assertStopReply(t, stop, false)
	assertQueuedSkillDropped(t, srv, c, objID, frames, mpBefore)
}

// TestInterruptDropsQueuedSkill pins the window-gated interrupt (an abort
// cast effect, a damage break) with a skill queued behind the cast: the
// queued skill is dropped with ActionFailed, and CASTING_INTERRUPTED comes
// last.
func TestInterruptDropsQueuedSkill(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootStopQueue(t)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.InterruptCast() })
	frames := queueFrames(t, c)
	assertStopReply(t, frames, true)
	assertQueuedSkillDropped(t, srv, c, objID, frames, mpBefore)
}

// TestTargetCancelDropsQueuedSkill pins Esc mid-cast with a skill queued
// behind the cast: RequestTargetCancel's CANCEL stops the cast
// (PlayerAI.onEvtCancel, PlayerAI.java:161-167), which drops the queued
// skill with ActionFailed.
func TestTargetCancelDropsQueuedSkill(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootStopQueue(t)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestTargetCancel(0))
	frames := queueFrames(t, c)
	assertStopReply(t, frames, false)
	assertQueuedSkillDropped(t, srv, c, objID, frames, mpBefore)
}

// TestQueuedSkillStillRunsAfterNaturalFinish pins the other side of the
// stop: a cast that completes clears its casting flag before it notifies
// the AI (CreatureCast.onMagicFinalizer), so the skill queued behind it
// starts.
func TestQueuedSkillStillRunsAfterNaturalFinish(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootStopQueue(t)

	srv.AdvanceUntil(t, "queued skill start", func() bool {
		var started bool
		onPlayerQueue(t, srv, objID, func(pc *player.Character) { started = pc.SkillDisabled(cast.ReuseKey(stopQueueSkills()[1])) })
		return started
	})
	if frames := queueFrames(t, c); !castStartedFor(t, frames, stopQueuePhysicalSkillID) {
		t.Fatalf("queued skill never started after the cast finished: opcodes %x", opcodesOf(frames))
	}
}

// teleportMidCastActionFailed teleports the caster in place mid-cast, with
// the physical skill queued behind the cast when queue is set, and returns
// how many ActionFailed follow the stop's MagicSkillCanceled.
func teleportMidCastActionFailed(t *testing.T, queue bool) int {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, stopQueueSkills())),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, def := range stopQueueSkills() {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	startInWorld(t, c)
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(stopQueueMagicSkillID, false, false))
	drainUntilQuiet(t, c)
	if queue {
		assertQueuedWithActionFailed(t, c, stopQueuePhysicalSkillID)
	}
	mpBefore := srv.PlayerCurrentMP(t, objID)

	x, y, z := srv.PlayerPosition(t, objID)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.TeleportTo(x, y, z, 0) })
	frames := queueFrames(t, c)
	canceled := -1
	failed := 0
	for i, frame := range frames {
		switch {
		case frame[0] == serverpackets.OpcodeMagicSkillCanceled:
			canceled = i
		case frame[0] == serverpackets.OpcodeActionFailed && canceled >= 0:
			failed++
		}
	}
	if canceled < 0 {
		t.Fatalf("teleport mid-cast sent no MagicSkillCanceled: opcodes %x", opcodesOf(frames))
	}
	if queue {
		assertQueuedSkillDropped(t, srv, c, objID, frames, mpBefore)
	}
	return failed
}

// TestTeleportStopDropsQueuedSkill pins a teleport mid-cast with a skill
// queued: Creature.teleportTo's abortAll stops the cast
// (Creature.java:1298-1306), and the queued CAST's PlayerAI.thinkCast
// answers one more ActionFailed than the same teleport with nothing queued.
func TestTeleportStopDropsQueuedSkill(t *testing.T) {
	t.Parallel()
	var bare, queued int
	t.Run("nothing queued", func(t *testing.T) { bare = teleportMidCastActionFailed(t, false) })
	t.Run("skill queued", func(t *testing.T) { queued = teleportMidCastActionFailed(t, true) })
	if t.Failed() {
		return
	}
	if queued != bare+1 {
		t.Fatalf("teleport stop sent %d ActionFailed with a queued skill and %d without, want one more", queued, bare)
	}
}
