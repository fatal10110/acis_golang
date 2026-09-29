package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Cast-queue fixture: a long cast to be mid-cast with, two skills to queue
// behind it, and a toggle. The harness's reads move the driven clock while
// they wait, so every cast here is long enough that reading frames never
// reaches its launch.
const (
	queueLongSkillID   int32 = 3
	queueSkillID       int32 = 4
	queueOtherSkillID  int32 = 5
	queueToggleSkillID int32 = 288

	queueHitTime = 5000
)

func queueSkills() []modelskill.Definition {
	short := func(id int32) modelskill.Definition {
		return modelskill.Definition{
			ID: modelskill.ID(id), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: queueHitTime, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true, MPConsume: 5, SkillType: "DUMMY",
		}
	}
	return []modelskill.Definition{
		{
			ID: modelskill.ID(queueLongSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: queueHitTime, StaticHitTime: true, SkillType: "DUMMY",
		},
		short(queueSkillID),
		short(queueOtherSkillID),
		{
			ID: modelskill.ID(queueToggleSkillID), Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
			MPConsume: 12, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
		},
	}
}

// bootMidCast boots a caster knowing queueSkills and leaves it inside the
// long cast, every frame of its start already read. Holding the cast open
// needs the driven clock.
func bootMidCast(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, queueSkills())),
	)
	if !srv.DrivesClock() {
		t.Skip("holding a cast open needs the driven clock")
	}
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, def := range queueSkills() {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(queueLongSkillID, false, false))
	if skill := magicSkillUseSkill(t, c.Read()); skill != queueLongSkillID {
		t.Fatalf("long cast MagicSkillUse skill = %d, want %d", skill, queueLongSkillID)
	}
	drainUntilQuiet(t, c)
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("long cast not in flight")
	}
	return srv, c, objID
}

// advanceUntilStarted lets the driven clock run until skillID has started,
// which stamps its reuse.
func advanceUntilStarted(t *testing.T, srv *gameservertest.Server, objID, skillID int32) {
	t.Helper()
	key := cast.ReuseKey(queueSkillDef(skillID))
	srv.AdvanceUntil(t, "queued skill start", func() bool {
		var started bool
		onPlayerQueue(t, srv, objID, func(pc *player.Character) { started = pc.SkillDisabled(key) })
		return started
	})
}

// advanceUntilCastEnds lets the driven clock run until no cast is in flight.
func advanceUntilCastEnds(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	srv.AdvanceUntil(t, "cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
}

func queueSkillDef(skillID int32) modelskill.Definition {
	for _, def := range queueSkills() {
		if def.ID == modelskill.ID(skillID) {
			return def
		}
	}
	panic("unknown queue fixture skill")
}

// magicSkillUseSkill asserts frame is a MagicSkillUse and returns its skill id.
func magicSkillUseSkill(t *testing.T, frame []byte) int32 {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // caster
	r.ReadInt32() // target
	return r.ReadInt32()
}

// queueFrames reads every frame until the client goes quiet.
func queueFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return nil
}

// castStartedFor reports whether frames carry a MagicSkillUse for skillID.
func castStartedFor(t *testing.T, frames [][]byte, skillID int32) bool {
	t.Helper()
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeMagicSkillUse && magicSkillUseSkill(t, frame) == skillID {
			return true
		}
	}
	return false
}

func countOpcode(frames [][]byte, opcode byte) int {
	n := 0
	for _, frame := range frames {
		if frame[0] == opcode {
			n++
		}
	}
	return n
}

// assertQueuedWithActionFailed sends skillID mid-cast and asserts the
// request is answered with ActionFailed alone.
func assertQueuedWithActionFailed(t *testing.T, c *testsupport.ScriptedClient, skillID int32) {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(skillID, false, false))
	frames := queueFrames(t, c)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("skill %d requested mid-cast = opcodes %x, want ActionFailed alone", skillID, opcodesOf(frames))
	}
}

// TestMidCastSkillWaitsForTheCast pins the casting-now queue: a skill
// requested while another cast is in flight is answered with ActionFailed,
// pays nothing, and starts once the cast in flight ends.
func TestMidCastSkillWaitsForTheCast(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootMidCast(t)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	assertQueuedWithActionFailed(t, c, queueSkillID)
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after a queued request = %d, want unchanged %d", got, mpBefore)
	}

	advanceUntilStarted(t, srv, objID, queueSkillID)
	frames := queueFrames(t, c)
	if !castStartedFor(t, frames, queueSkillID) {
		t.Fatalf("queued skill started without its MagicSkillUse: opcodes %x", opcodesOf(frames))
	}
	if n := countOpcode(frames, serverpackets.OpcodeActionFailed); n != 0 {
		t.Fatalf("queued skill's start sent %d ActionFailed, want none", n)
	}
	if !srv.PlayerCastingNow(t, objID) {
		t.Fatal("queued skill is not in flight")
	}
}

// TestMidCastLaterRequestReplacesQueued pins the single next-intention
// slot: a second request mid-cast replaces the first, which never runs.
func TestMidCastLaterRequestReplacesQueued(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootMidCast(t)

	assertQueuedWithActionFailed(t, c, queueSkillID)
	assertQueuedWithActionFailed(t, c, queueOtherSkillID)

	advanceUntilStarted(t, srv, objID, queueOtherSkillID)
	frames := queueFrames(t, c)
	if !castStartedFor(t, frames, queueOtherSkillID) {
		t.Fatalf("latest queued skill never started: opcodes %x", opcodesOf(frames))
	}
	if castStartedFor(t, frames, queueSkillID) {
		t.Fatal("replaced queued skill started")
	}
}

// TestMidCastRequestOnReuseIsRefusedNotQueued pins the pre-attempt gate
// running ahead of the casting-now queue: a skill still on reuse, requested
// mid-cast, answers S1_PREPARED_FOR_REUSE then ActionFailed at once, and is
// not replayed once the cast ends, even with its reuse over by then.
func TestMidCastRequestOnReuseIsRefusedNotQueued(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootMidCast(t)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.DisableSkill(cast.ReuseKey(queueSkillDef(queueSkillID)), 50*time.Millisecond)
	})

	c.Send(encodeRequestMagicSkillUse(queueSkillID, false, false))
	assertSystemMessageSkillFrame(t, c.Read(), serverpackets.SystemMessageS1PreparedForReuse, queueSkillID, 1)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "reuse ActionFailed")

	advanceUntilCastEnds(t, srv, objID)
	if frames := queueFrames(t, c); castStartedFor(t, frames, queueSkillID) {
		t.Fatal("skill refused on reuse mid-cast started once the cast ended")
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("a cast is in flight after the long cast ended with nothing queued")
	}
}

// TestQueuedSkillFailingItsResumeSendsReasonAlone pins the resume's own
// gate: a queued skill that went on reuse while it waited is dropped with
// its reason message and no ActionFailed, unlike a fresh request.
func TestQueuedSkillFailingItsResumeSendsReasonAlone(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootMidCast(t)

	assertQueuedWithActionFailed(t, c, queueSkillID)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.DisableSkill(cast.ReuseKey(queueSkillDef(queueSkillID)), time.Minute)
	})

	advanceUntilCastEnds(t, srv, objID)
	frames := queueFrames(t, c)
	if castStartedFor(t, frames, queueSkillID) {
		t.Fatal("queued skill on reuse started")
	}
	reasons := 0
	for _, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeActionFailed:
			t.Fatalf("failed resume sent ActionFailed: opcodes %x", opcodesOf(frames))
		case serverpackets.OpcodeSystemMessage:
			if wire.NewReader(frame[1:]).ReadInt32() == int32(serverpackets.SystemMessageS1PreparedForReuse) {
				assertSystemMessageSkillFrame(t, frame, serverpackets.SystemMessageS1PreparedForReuse, queueSkillID, 1)
				reasons++
			}
		}
	}
	if reasons != 1 {
		t.Fatalf("failed resume sent %d S1_PREPARED_FOR_REUSE, want 1: opcodes %x", reasons, opcodesOf(frames))
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("a cast is in flight after the queued skill failed its resume")
	}
}

// TestMidCastToggleWaitsForTheCast pins toggles sharing the queue: a toggle
// requested mid-cast answers ActionFailed, pays nothing and applies nothing
// until the cast ends, then turns on.
func TestMidCastToggleWaitsForTheCast(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootMidCast(t)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	assertQueuedWithActionFailed(t, c, queueToggleSkillID)
	if _, on := liveEffectList(t, srv, objID).ActiveBySkillID(int(queueToggleSkillID)); on {
		t.Fatal("toggle turned on mid-cast")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after a queued toggle = %d, want unchanged %d", got, mpBefore)
	}

	srv.AdvanceUntil(t, "queued toggle on", func() bool {
		_, on := liveEffectList(t, srv, objID).ActiveBySkillID(int(queueToggleSkillID))
		return on
	})
	frames := queueFrames(t, c)
	if !castStartedFor(t, frames, queueToggleSkillID) {
		t.Fatalf("queued toggle never cast after the long cast ended: opcodes %x", opcodesOf(frames))
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("the long cast is still in flight with the queued toggle on")
	}
}

func opcodesOf(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, frame := range frames {
		out = append(out, frame[0])
	}
	return out
}

// TestQueuedSkillWhenCasterCannotActSendsActionFailedAlone pins the resume's
// can-act check: a queued skill whose caster can no longer act once the
// cast ends is dropped with ActionFailed alone — no reason message and no
// cast — unlike a fresh request, which would run the pre-attempt gate.
func TestQueuedSkillWhenCasterCannotActSendsActionFailedAlone(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootMidCast(t)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	assertQueuedWithActionFailed(t, c, queueSkillID)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetParalyzed(true) })

	advanceUntilCastEnds(t, srv, objID)
	frames := queueFrames(t, c)
	if castStartedFor(t, frames, queueSkillID) {
		t.Fatal("queued skill started for a caster that cannot act")
	}
	if n := countOpcode(frames, serverpackets.OpcodeActionFailed); n != 1 {
		t.Fatalf("resume for a caster that cannot act sent %d ActionFailed, want 1: opcodes %x", n, opcodesOf(frames))
	}
	if n := countOpcode(frames, serverpackets.OpcodeSystemMessage); n != 0 {
		t.Fatalf("resume for a caster that cannot act sent %d system messages, want none: opcodes %x", n, opcodesOf(frames))
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("a cast is in flight after the queued skill was dropped")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after the dropped queued skill = %d, want unchanged %d", got, mpBefore)
	}
}
