package combat

import (
	"encoding/binary"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// framesUntilQuiet reads every frame c receives until it goes quiet.
func framesUntilQuiet(c *scriptedClient) [][]byte {
	var frames [][]byte
	for {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
}

// punchOfDoomID is the skill whose StunSelf effect stuns its own caster.
const punchOfDoomID = 81

// landStunSelf applies a real StunSelf effect to target on its own queue,
// under a skill of its own so it stacks beside any stun already held, and
// waits for its start hook to finish.
func landStunSelf(t *testing.T, target effectHolder) {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: punchOfDoomID, Level: 1}, modelskill.EffectTemplate{Name: "StunSelf", Time: 9})
	if err != nil {
		t.Fatalf("effect.New(StunSelf): %v", err)
	}
	e.Effector, e.Effected = target, target
	done := make(chan struct{})
	if !target.Queue().Post(func() { target.EffectList().Add(e); close(done) }) {
		t.Fatal("post StunSelf: queue closed")
	}
	<-done
}

// onlineEffectHolder returns objID's live player as the surface a landed
// effect reaches.
func onlineEffectHolder(t *testing.T, srv *gameservertest.Server, objID int32) effectHolder {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	holder, ok := obj.(effectHolder)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}
	return holder
}

// TestStunSelfStopsWalkingPlayer pins EffectStunSelf.onStart sending a
// walking player idle: nothing is waited out, so the idle stops the walk
// (StopMove) and answers nothing.
func TestStunSelfStopsWalkingPlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
	expectGroundClickAck(t, c)
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	drainUntilQuiet(t, c)

	landStunSelf(t, onlineEffectHolder(t, srv, objID))

	frames := framesUntilQuiet(c)
	var stop []byte
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeStopMove {
			stop = f
			break
		}
	}
	if stop == nil {
		t.Fatalf("frames after StunSelf = %d, none a StopMove: the walk was not stopped", len(frames))
	}
	if got := int32(binary.LittleEndian.Uint32(stop[1:5])); got != objID {
		t.Fatalf("StopMove object = %d, want %d", got, objID)
	}
	if n := countOpcode(frames, len(frames), serverpackets.OpcodeActionFailed); n != 0 {
		t.Fatalf("ActionFailed frames after StunSelf = %d, want 0 for a player with nothing to wait out", n)
	}
}

// TestStunSelfMidSwingWaitsOutTheSwing pins PlayableAI.tryToIdle for a
// player mid-swing: the idle is answered ActionFailed and replaces the cast
// queued behind the swing, which never starts once the swing ends.
func TestStunSelfMidSwingWaitsOutTheSwing(t *testing.T) {
	t.Parallel()
	srv, _ := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)

	c.Send(encodeRequestMagicSkillUse(midSwingSkillID, false, false))
	assertFrameOpcode(t, mustRead(t, c, "queued ActionFailed"), serverpackets.OpcodeActionFailed, "queued ActionFailed")

	landStunSelf(t, onlineEffectHolder(t, srv, objID))
	assertFrameOpcode(t, mustRead(t, c, "idle ActionFailed"), serverpackets.OpcodeActionFailed, "idle ActionFailed")

	for passed := time.Duration(0); passed < 2*time.Second; passed += 10 * time.Millisecond {
		srv.Advance(t, 10*time.Millisecond)
		if srv.PlayerCastingNow(t, objID) {
			t.Fatalf("queued cast started %v after StunSelf: the idle must replace it", passed)
		}
	}
}

// TestStunSelfOnStunnedPlayerAnswersActionFailed pins the denied branch: a
// player already unable to act keeps its intentions and is only answered
// ActionFailed.
func TestStunSelfOnStunnedPlayerAnswersActionFailed(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	holder := onlineEffectHolder(t, srv, objID)
	landStun(t, holder)
	drainUntilQuiet(t, c)

	landStunSelf(t, holder)
	frames := framesUntilQuiet(c)
	if n := countOpcode(frames, len(frames), serverpackets.OpcodeActionFailed); n != 1 {
		t.Fatalf("ActionFailed frames after StunSelf on a stunned player = %d, want 1", n)
	}
}
