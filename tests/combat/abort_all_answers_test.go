package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Death. Playable.doDie (Playable.java:127-142) marks the player dead, so
// denyAiAction holds (Creature.java:636-639), then runs abortAll(true)
// (Creature.java:1298-1306): the move stop broadcasts StopMove when a walk
// is under way (CreatureMove.java:452-463); the attack stop answers twice,
// its refused idle's ActionFailed (PlayableAttack.java:45-50,
// PlayableAI.java:354-360) and its own (PlayerAttack.java:58-63); the cast
// stop answers twice the same way (PlayableCast.java:101-107,
// PlayerCast.java:381-387); and the target reset answers once more, then
// tells observers the selection is gone when there was one
// (Player.setTarget, Player.java:2496-2505). Die comes after
// (CreatureAI DEAD).

// abortAnswers keeps the frames abortAll answers with, in order.
func abortAnswers(frames [][]byte) []byte {
	var got []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeStopMove, serverpackets.OpcodeTargetUnselected, serverpackets.OpcodeMagicSkillCanceled:
			got = append(got, f[0])
		}
	}
	return got
}

func TestDeathAbortsAllBeforeDie(t *testing.T) {
	t.Parallel()
	const (
		af = serverpackets.OpcodeActionFailed
		sm = serverpackets.OpcodeStopMove
		tu = serverpackets.OpcodeTargetUnselected
	)
	cases := []struct {
		name  string
		stage func(t *testing.T, srv *gameservertest.Server, c *scriptedClient)
		want  []byte
	}{
		{name: "idle", want: []byte{af, af, af, af, af}},
		{name: "monster selected", stage: func(t *testing.T, srv *gameservertest.Server, c *scriptedClient) {
			hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			targetHostile(t, c, hostile.ObjectID())
		}, want: []byte{af, af, af, af, af, tu}},
		{name: "walking", stage: func(t *testing.T, _ *gameservertest.Server, c *scriptedClient) {
			c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
			assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
		}, want: []byte{sm, af, af, af, af, af}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			drainUntilQuiet(t, c)
			if tc.stage != nil {
				tc.stage(t, srv, c)
				drainUntilQuiet(t, c)
			}

			onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.Kill(nil) })
			frames := framesUntilQuiet(c)
			die := indexOf(frames, 0, serverpackets.OpcodeDie, objID)
			if die < 0 {
				t.Fatal("the death sent no Die")
			}
			if got := abortAnswers(frames[:die]); string(got) != string(tc.want) {
				t.Fatalf("abort answers before Die = %x, want %x", got, tc.want)
			}
			if got := abortAnswers(frames[die:]); len(got) != 0 {
				t.Fatalf("abort answers after Die = %x, want none", got)
			}
			if target := onlinePlayer(t, srv, objID).Target(); target != nil {
				t.Fatalf("dead player still selects %d", target.ObjectID())
			}
		})
	}
}

// SilenceMagicPhysical stops the cast unconditionally
// (EffectSilenceMagicPhysical.java:23-27). With no cast in flight
// PlayableCast.stop still runs tryToIdle (PlayableAI.java:354-371): a
// walking player goes idle, which stops the walk (StopMove), and
// PlayerCast.stop answers ActionFailed after it.

func landSilenceMagicPhysical(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	landEffect(t, onlineEffectHolder(t, srv, objID), "SilenceMagicPhysical")
}

func TestSilenceMagicPhysicalStopsWalkingPlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	drainUntilQuiet(t, c)

	landSilenceMagicPhysical(t, srv, objID)
	frames := framesUntilQuiet(c)
	want := []byte{serverpackets.OpcodeStopMove, serverpackets.OpcodeActionFailed}
	if got := abortAnswers(frames); string(got) != string(want) {
		t.Fatalf("silence on a walking player answered %x, want %x", got, want)
	}
	if stop := indexOf(frames, 0, serverpackets.OpcodeStopMove, objID); stop < 0 {
		t.Fatal("the StopMove is not the silenced player's")
	}
	x, y, z := srv.PlayerPosition(t, objID)
	srv.Advance(t, 2*time.Second)
	if nx, ny, nz := srv.PlayerPosition(t, objID); nx != x || ny != y || nz != z {
		t.Fatalf("silenced player kept walking: (%d,%d,%d) -> (%d,%d,%d)", x, y, z, nx, ny, nz)
	}
}

// Mid-swing, tryToIdle finds the player attacking: it only replaces the next
// intention with idle and answers ActionFailed (PlayableAI.java:362-367).
// The attack stays current and goes on after the swing
// (PlayableAI.onEvtFinishedAttack, PlayableAI.java:66-77); a walk queued
// behind the swing is gone.
func TestSilenceMagicPhysicalMidSwingKeepsAttacking(t *testing.T) {
	t.Parallel()
	srv, _ := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)

	c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
	assertFrameOpcode(t, mustRead(t, c, "queued walk ActionFailed"), serverpackets.OpcodeActionFailed, "queued walk ActionFailed")

	landSilenceMagicPhysical(t, srv, objID)
	for i := 0; i < 50; i++ {
		frame := c.ReadWithTimeout(3 * time.Second)
		if frame == nil {
			t.Fatal("the attack stopped after the silence")
		}
		switch frame[0] {
		case serverpackets.OpcodeMoveToLocation:
			t.Fatal("the walk queued behind the swing ran after the silence")
		case serverpackets.OpcodeAttack:
			if wireReader(frame[1:]).ReadInt32() == objID {
				return
			}
		}
	}
	t.Fatal("no next swing within 50 frames")
}

// TestSilenceMagicPhysicalMidSwingAnswers pins the answers: the busy idle's
// ActionFailed, then the stop's own.
func TestSilenceMagicPhysicalMidSwingAnswers(t *testing.T) {
	t.Parallel()
	srv, _ := bootMidSwing(t)
	c, objID := srv.Client, srv.SoleObjectID(t)

	landSilenceMagicPhysical(t, srv, objID)
	if got := readUntilAttackCountingActionFailed(t, c, "next swing"); got != 2 {
		t.Fatalf("ActionFailed between the silence and the next swing = %d, want 2", got)
	}
}
