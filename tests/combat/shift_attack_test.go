package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// assertHeldAttackIdle lets d pass after a shift-held attack on a target out
// of reach and fails on any MoveToPawn, MoveToLocation or Attack by the
// player, or when the player left from. wantFailed also requires the
// ActionFailed answering it among those frames.
func assertHeldAttackIdle(t *testing.T, srv *gameservertest.Server, c *scriptedClient, objID int32, from location.Location, d time.Duration, wantFailed bool, what string) {
	t.Helper()
	sawFailed := false
	for passed := time.Duration(0); passed < d; passed += 100 * time.Millisecond {
		srv.Advance(t, 100*time.Millisecond)
		for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
			switch {
			case frame[0] == serverpackets.OpcodeActionFailed:
				sawFailed = true
			case frame[0] == serverpackets.OpcodeMoveToPawn, frame[0] == serverpackets.OpcodeMoveToLocation:
				t.Fatalf("%s: movement opcode %#x, want no walk", what, frame[0])
			case attackFrameBy(frame, objID):
				t.Fatalf("%s: Attack by the player, want none", what)
			}
		}
	}
	if wantFailed && !sawFailed {
		t.Fatalf("%s: no ActionFailed", what)
	}
	x, y, z := onlinePlayer(t, srv, objID).Position()
	if got := (location.Location{X: x, Y: y, Z: z}); got != from {
		t.Fatalf("%s: player at %+v, want still at %+v", what, got, from)
	}
}

// TestShiftAttackOnDistantTargetFailsWithoutWalking pins the shift-held
// attack: a shift click (or shift AttackRequest) on a selected monster out of
// weapon range never walks — it is answered ActionFailed alone, with no
// MoveToPawn and no Attack, and the player stays put. The same click without
// shift walks (TestSecondActionClickWalksTowardDistantTarget).
func TestShiftAttackOnDistantTargetFailsWithoutWalking(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		encode func(objectID int32, x, y, z int32, shift bool) []byte
	}{
		{"Action", encodeAction},
		{"AttackRequest", encodeAttackRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			startInWorld(t, c)
			hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)

			targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)
			c.Send(tc.encode(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), true))
			assertFrameOpcode(t, mustRead(t, c, "shift attack ActionFailed"), serverpackets.OpcodeActionFailed, "shift attack ActionFailed")
			assertHeldAttackIdle(t, srv, c, objID, playerOrigin, time.Second, false, "after the shift attack")
			if got := hostile.CurrentHP(); got != hostile.MaxHP() {
				t.Fatalf("hostile HP = %d after the shift attack, want untouched %d", got, hostile.MaxHP())
			}
		})
	}
}

// TestShiftNextActionAttackCastOnDistantTargetEndsWithoutWalking pins the
// cast's shift carrying into its follow-up: a shift-held nextActionAttack
// skill cast on a monster inside cast range but out of weapon range ends in
// an attack intention that never walks — ActionFailed, no MoveToPawn, no
// Attack.
func TestShiftNextActionAttackCastOnDistantTargetEndsWithoutWalking(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	far := f.srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 250, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, f.c)
	f.selectAmidFrames(t, far.ObjectID())
	drainUntilQuiet(t, f.c)

	f.c.Send(encodeRequestMagicSkillUse(followUpOneSkillID, false, true))
	f.readUntilOwnMagicSkillUse(t, "shift follow-up skill MagicSkillUse")
	f.srv.AdvanceUntil(t, "shift follow-up skill end", func() bool { return !f.pc.CastingNow() })
	assertHeldAttackIdle(t, f.srv, f.c, f.objID, f.origin, time.Second, true, "after the shift follow-up skill")
}

// TestNextActionAttackCastOnDistantTargetWalks is the unshifted half: the
// same cast without shift walks to the monster once it ends.
func TestNextActionAttackCastOnDistantTargetWalks(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	far := f.srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 250, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, f.c)
	f.selectAmidFrames(t, far.ObjectID())
	drainUntilQuiet(t, f.c)

	f.c.Send(encodeRequestMagicSkillUse(followUpOneSkillID, false, false))
	f.readUntilOwnMagicSkillUse(t, "follow-up skill MagicSkillUse")
	readUntil(t, f.c, serverpackets.OpcodeMoveToPawn, "follow-up walk MoveToPawn")
}

// TestShiftNextActionAttackSkillRefusedForMPOnDistantTargetEndsWithoutWalking
// pins the shift carried by the cost-check refusal's hand-off: a shift-held
// nextActionAttack skill the player cannot pay MP for, cast on a monster
// inside cast range but out of weapon range, is answered NOT_ENOUGH_MP and
// its attack never walks — no MoveToPawn, no Attack, and the player stays
// put.
func TestShiftNextActionAttackSkillRefusedForMPOnDistantTargetEndsWithoutWalking(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	far := f.srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 250, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, f.c)
	f.selectAmidFrames(t, far.ObjectID())
	drainUntilQuiet(t, f.c)

	f.c.Send(encodeRequestMagicSkillUse(followUpCostlySkillID, false, true))
	refusal := readUntil(t, f.c, serverpackets.OpcodeSystemMessage, "shift NOT_ENOUGH_MP")[0]
	if got := wireReader(refusal[1:]).ReadInt32(); got != int32(serverpackets.SystemMessageNotEnoughMP) {
		t.Fatalf("refusal system message = %d, want NOT_ENOUGH_MP (%d)", got, serverpackets.SystemMessageNotEnoughMP)
	}
	assertHeldAttackIdle(t, f.srv, f.c, f.objID, f.origin, time.Second, false, "after the shift refused follow-up skill")
	if f.pc.CastingNow() {
		t.Fatal("refused skill started casting")
	}
}

// TestNextActionAttackSkillRefusedForMPOnDistantTargetWalks is the unshifted
// half: the same refusal without shift walks to the monster.
func TestNextActionAttackSkillRefusedForMPOnDistantTargetWalks(t *testing.T) {
	t.Parallel()
	f := bootFollowUpCaster(t)
	far := f.srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 250, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, f.c)
	f.selectAmidFrames(t, far.ObjectID())
	drainUntilQuiet(t, f.c)

	f.c.Send(encodeRequestMagicSkillUse(followUpCostlySkillID, false, false))
	readUntil(t, f.c, serverpackets.OpcodeSystemMessage, "NOT_ENOUGH_MP")
	readUntil(t, f.c, serverpackets.OpcodeMoveToPawn, "refused follow-up walk MoveToPawn")
}
