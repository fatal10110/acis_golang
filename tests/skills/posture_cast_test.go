package skills

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// sitStandDelay is how long Player.sitDown/standUp hold the character
// before the SAT_DOWN/STOOD_UP event (Player.java:1542-1590).
const sitStandDelay = 2500 * time.Millisecond

// fakeDeathSkillID is the Fake Death toggle.
const fakeDeathSkillID int32 = 60

// postureSkills are the gate skills plus a Fake Death toggle shaped like
// skill 60 in the datapack, with a cheap MP drain.
func postureSkills() []modelskill.Definition {
	return append(gateSkills(), modelskill.Definition{
		ID: modelskill.ID(fakeDeathSkillID), Level: 1, Activation: modelskill.ActivationToggle, Target: modelskill.TargetSelf,
		MPConsume: 1, SkillType: "FAKE_DEATH",
		Effects: []modelskill.EffectTemplate{{Name: "FakeDeath", Count: 0x7fffffff, Time: 5, Value: 1}},
	})
}

func encodeRequestChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

// bootPostureCaster boots a player who knows postureSkills and is in the
// world with a quiet stream.
func bootPostureCaster(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, postureSkills())),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, def := range postureSkills() {
		seedKnownSkill(t, srv, objID, int(def.ID), 1)
	}
	startInWorld(t, c)
	return srv, c, objID
}

// readMatching reads frames until match accepts one, failing if none does
// within d on the clock reads wait on.
func readMatching(t *testing.T, c *testsupport.ScriptedClient, d time.Duration, what string, match func([]byte) bool) []byte {
	t.Helper()
	end := c.Now().Add(d)
	for {
		frame := c.ReadWithTimeout(end.Sub(c.Now()))
		if frame == nil {
			t.Fatalf("%s: not received within %v", what, d)
		}
		if match(frame) {
			return frame
		}
	}
}

func isWaitType(want serverpackets.WaitType) func([]byte) bool {
	return func(frame []byte) bool {
		return frame[0] == serverpackets.OpcodeChangeWaitType &&
			serverpackets.WaitType(binary.LittleEndian.Uint32(frame[5:9])) == want
	}
}

func isSkillUse(skillID int32) func([]byte) bool {
	return func(frame []byte) bool {
		return frame[0] == serverpackets.OpcodeMagicSkillUse && int32(binary.LittleEndian.Uint32(frame[9:13])) == skillID
	}
}

// fakeDeathDelays are the reference fake-death lie-down and get-up times,
// 3000ms and 2500ms over the movement speed multiplier
// (Player.java:7016-7056). With no speed modifier on the player, the
// multiplier is the DEX run-speed bonus alone (FuncMoveSpeed).
func fakeDeathDelays(t *testing.T, srv *gameservertest.Server, objID int32) (lieDown, getUp time.Duration) {
	t.Helper()
	var dex int
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { dex = pc.DEX() })
	mult := float32(statbonus.DEXBonus[dex])
	if mult == 1 {
		t.Fatalf("DEX %d gives multiplier 1; the durations would not tell the multiplier apart", dex)
	}
	return time.Duration(int32(3000/mult)) * time.Millisecond, time.Duration(int32(2500/mult)) * time.Millisecond
}

// startFakeDeath switches Fake Death on from the skill bar and returns when
// the lie-down began.
func startFakeDeath(t *testing.T, c *testsupport.ScriptedClient) time.Time {
	t.Helper()
	c.Send(encodeRequestMagicSkillUse(fakeDeathSkillID, false, false))
	readMatching(t, c, time.Second, "fake-death start ChangeWaitType", isWaitType(serverpackets.WaitFakeDeathStart))
	return c.Now()
}

// assertSittingRefusal expects CANT_MOVE_SITTING then ActionFailed, and no
// cast.
func assertSittingRefusal(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, what string) {
	t.Helper()
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotMoveWhileSitting)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, what+" ActionFailed")
	if srv.PlayerCastingNow(t, objID) {
		t.Fatalf("%s: refused cast is in flight", what)
	}
}

// TestSkillBarCastDuringSitDownIsRefusedOnceSeated pins the skill-bar
// sitting gate on Player.isSitting(), which stays false for the whole
// sit-down (Player.java:1542-1552): the request passes canAttemptCast, is
// queued by isSittingNow() with ActionFailed (PlayableAI.java:305-318), and
// SAT_DOWN refuses it with CANT_MOVE_SITTING alone (PlayerAI.java:241-242).
// No MP is spent.
func TestSkillBarCastDuringSitDownIsRefusedOnceSeated(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootPostureCaster(t)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestChangeWaitType(false))
	readMatching(t, c, time.Second, "sit ChangeWaitType", isWaitType(serverpackets.WaitSitting))
	sitAt := c.Now()
	c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued skill-bar cast")

	frame := c.Read()
	if elapsed := c.Now().Sub(sitAt); elapsed < sitStandDelay {
		t.Fatalf("queued skill-bar cast answered %v after sit-down, want no earlier than %v", elapsed, sitStandDelay)
	}
	assertStaticSystemMessage(t, frame, serverpackets.SystemMessageCannotMoveWhileSitting)
	if extra := c.ReadWithTimeout(time.Second); extra != nil {
		t.Fatalf("refused queued cast extra opcode = %#x, want none", extra[0])
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after refused cast = %d, want unchanged %d", got, mpBefore)
	}
}

// TestSkillBarCastDuringStandUpRunsWhenUp pins a skill-bar cast made during
// the stand-up: isStandingNow() queues it with ActionFailed, and STOOD_UP
// runs it (PlayerAI.onEvtStoodUp).
func TestSkillBarCastDuringStandUpRunsWhenUp(t *testing.T) {
	t.Parallel()
	srv, c, _ := bootPostureCaster(t)
	c.Send(encodeRequestChangeWaitType(false))
	readMatching(t, c, time.Second, "sit ChangeWaitType", isWaitType(serverpackets.WaitSitting))
	srv.Advance(t, sitStandDelay)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestChangeWaitType(true))
	readMatching(t, c, time.Second, "stand ChangeWaitType", isWaitType(serverpackets.WaitStanding))
	standAt := c.Now()
	c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued skill-bar cast")

	readMatching(t, c, 2*sitStandDelay, "queued skill-bar cast", isSkillUse(gateActiveSkillID))
	if elapsed := c.Now().Sub(standAt); elapsed < sitStandDelay {
		t.Fatalf("queued skill-bar cast ran %v after stand-up, want no earlier than %v", elapsed, sitStandDelay)
	}
}

// TestFakeDeathLieDownHoldsToggleOff pins the fake-death lie-down
// (Player.startFakeDeath, Player.java:7016-7033): it lasts 3000ms over the
// movement speed multiplier. Another skill is refused at once with
// CANT_MOVE_SITTING (PlayerCast.java:218-222); the Fake Death toggle itself
// passes and is queued behind the lie-down with ActionFailed, then switches
// fake death off when the lie-down ends.
func TestFakeDeathLieDownHoldsToggleOff(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootPostureCaster(t)
	lieDown, _ := fakeDeathDelays(t, srv, objID)
	startAt := startFakeDeath(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
	assertSittingRefusal(t, srv, c, objID, "skill during lie-down")
	c.Send(encodeRequestMagicSkillUse(fakeDeathSkillID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued fake-death toggle")

	readMatching(t, c, 2*lieDown, "fake-death stop ChangeWaitType", isWaitType(serverpackets.WaitFakeDeathStop))
	elapsed := c.Now().Sub(startAt)
	if elapsed < lieDown {
		t.Fatalf("toggle-off ran %v into the lie-down, want no earlier than %v", elapsed, lieDown)
	}
	if srv.DrivesClock() && elapsed >= lieDown+100*time.Millisecond {
		t.Fatalf("toggle-off ran %v into the lie-down, want at its end %v", elapsed, lieDown)
	}
}

// TestFakeDeathGetUpRefusesCastsUntilUp pins the fake-death get-up
// (Player.stopFakeDeath, Player.java:7035-7056): it lasts 2500ms over the
// movement speed multiplier, and the player plays dead until it ends, so a
// skill-bar cast in it is refused with CANT_MOVE_SITTING and ActionFailed
// (PlayerCast.java:218-222). Once it ends the same cast starts.
func TestFakeDeathGetUpRefusesCastsUntilUp(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootPostureCaster(t)
	lieDown, getUp := fakeDeathDelays(t, srv, objID)
	startFakeDeath(t, c)
	srv.Advance(t, lieDown)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestChangeWaitType(true))
	readMatching(t, c, time.Second, "fake-death stop ChangeWaitType", isWaitType(serverpackets.WaitFakeDeathStop))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeRevive, "fake-death Revive")
	standAt := c.Now()
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
	assertSittingRefusal(t, srv, c, objID, "skill during get-up")

	if srv.DrivesClock() {
		// Just short of the get-up's end the player still plays dead.
		srv.Advance(t, getUp-c.Now().Sub(standAt)-10*time.Millisecond)
		c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
		assertSittingRefusal(t, srv, c, objID, "skill at the end of get-up")
	}
	if rest := getUp - c.Now().Sub(standAt); rest > 0 {
		srv.Advance(t, rest)
	}
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
	readMatching(t, c, time.Second, "cast after get-up", isSkillUse(gateActiveSkillID))
}

// TestRestartRequestDuringFakeDeathGetUpKeepsGetUp pins a RequestRestartPoint
// sent during the fake-death get-up (RequestRestartPoint.java:38-42 calling
// Player.stopFakeDeath again, Player.java:7035-7056): the get-up and revive
// visuals are sent again, but the earlier get-up task is not cancelled, so
// fake death still ends at the first get-up's end.
func TestRestartRequestDuringFakeDeathGetUpKeepsGetUp(t *testing.T) {
	t.Parallel()
	srv, c, objID := bootPostureCaster(t)
	lieDown, getUp := fakeDeathDelays(t, srv, objID)
	startFakeDeath(t, c)
	srv.Advance(t, lieDown)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestChangeWaitType(true))
	readMatching(t, c, time.Second, "fake-death stop ChangeWaitType", isWaitType(serverpackets.WaitFakeDeathStop))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeRevive, "fake-death Revive")
	standAt := c.Now()
	drainUntilQuiet(t, c)

	c.Send(encodeRequestRestartPoint(0))
	readMatching(t, c, time.Second, "repeated fake-death stop ChangeWaitType", isWaitType(serverpackets.WaitFakeDeathStop))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeRevive, "repeated fake-death Revive")
	drainUntilQuiet(t, c)

	if rest := getUp - c.Now().Sub(standAt); rest > 0 {
		srv.Advance(t, rest)
	}
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(gateActiveSkillID, false, false))
	readMatching(t, c, time.Second, "cast after the first get-up's end", isSkillUse(gateActiveSkillID))
}

func encodeRequestRestartPoint(requestType int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestRestartPoint)
	w.WriteInt32(requestType)
	return w.Bytes()
}
