package skills

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: CreatureCast.onMagicLaunch (CreatureCast.java:180-195) stops
// the cast when AbstractAI.isTargetLost(target, skill) holds. That check
// (AbstractAI.java:602-613) finds a target missing from the world lost
// before it lets a SUMMON_FRIEND skill reach a target the caster does not
// know, so a friend who logs out mid-cast stops the launch: the caster sees
// MagicSkillCanceled and ActionFailed, no launch, and pays no MP.

// TestSummonFriendTargetLeavingTheWorldStopsTheLaunch casts the direct
// SUMMON_FRIEND on the friend, who logs out before the launch.
func TestSummonFriendTargetLeavingTheWorldStopsTheLaunch(t *testing.T) {
	t.Parallel()
	w := bootSummonWorld(t)
	w.selectTarget(t, w.targetID)
	mpBefore := w.srv.PlayerCurrentMP(t, w.casterID)

	w.caster.Send(encodeRequestMagicSkillUse(summonDirectSkill, false, false))
	w.srv.Settle(t)
	drainUntilQuiet(t, w.target)
	w.target.Send(encodeLogout())
	if reply := w.target.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout reply opcode = %#x, want LeaveWorld", reply[0])
	}
	w.target.ExpectClosed()
	w.srv.Settle(t)
	if _, ok := w.srv.State.Player(w.targetID); ok {
		t.Fatal("the friend is still in the world after logging out")
	}

	w.srv.Advance(t, summonFriendCastSettle)
	frames := queueFrames(t, w.caster)
	got := opcodesOf(frames)
	if !slices.Contains(got, serverpackets.OpcodeMagicSkillUse) {
		t.Fatalf("caster opcodes %x: the cast never started", got)
	}
	if slices.Contains(got, serverpackets.OpcodeMagicSkillLaunched) {
		t.Fatalf("caster opcodes %x: the cast launched on a friend gone from the world", got)
	}
	canceled := slices.Index(got, serverpackets.OpcodeMagicSkillCanceled)
	if canceled < 0 || !slices.Contains(got[canceled:], serverpackets.OpcodeActionFailed) {
		t.Fatalf("caster opcodes %x, want MagicSkillCanceled then ActionFailed", got)
	}
	if mp := w.srv.PlayerCurrentMP(t, w.casterID); mp != mpBefore {
		t.Fatalf("caster MP = %d, want unchanged %d", mp, mpBefore)
	}
}
