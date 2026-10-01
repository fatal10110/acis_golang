package pets

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// Reference for the tests below (#3069): a pet collar passes SummonItems'
// own gates (SummonItems.java:28-62), then calls PlayableAI.tryToCast and
// sends SUMMON_A_PET whatever tryToCast did (SummonItems.java:95-96).
// tryToCast refuses with ActionFailed alone while denyAiAction() holds
// (PlayableAI.java:297-302). Of denyAiAction()'s states (Creature.java:636-639,
// Player.java:627-630), only teleporting reaches it: UseItem refuses
// alike-dead, stun, sleep, paralysis and fear (UseItem.java:66), store and
// observer mode stop earlier, and SummonItems' isAllSkillsDisabled() check
// (SummonItems.java:37-38) returns silently on ImmobileUntilAttacked, which
// that predicate includes (Creature.java:620-623). A collar cast held behind a
// stand-up runs through PlayerAI.thinkCast when the stand-up ends, and the
// same denyAiAction() idles it there with ActionFailed alone
// (PlayerAI.java:219-226).

// assertCollarRefusedUnspent requires no cast in flight, no pet, MP at
// mpBefore and the collar still held.
func assertCollarRefusedUnspent(t *testing.T, h *petWorld, mpBefore int, what string) {
	t.Helper()
	if _, ok := h.srv.State.Summon(h.ownerID); ok || h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatalf("%s summoned or started a cast", what)
	}
	if got := h.srv.PlayerCurrentMP(t, h.ownerID); got != mpBefore {
		t.Fatalf("MP after %s = %d, want unchanged %d", what, got, mpBefore)
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("collar count after %s = %d, want 1", what, got)
	}
}

// TestCollarWhileTeleportingAnswersActionFailedThenSummonAPet: a collar used
// between a teleport and Appearing is answered with ActionFailed, then
// SUMMON_A_PET. No cast starts, no pet comes, and neither MP nor the collar
// is spent.
func TestCollarWhileTeleportingAnswersActionFailedThenSummonAPet(t *testing.T) {
	t.Parallel()
	h := bootPostureCollar(t)
	mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)
	if !h.character(t).SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}

	h.client.Send(encodeUseItem(h.collarID, false))
	assertHeldCollar(t, h, drainFrames(t, h.client), "collar while teleporting")
	assertCollarRefusedUnspent(t, h, mpBefore, "the collar used while teleporting")
}

// TestCollarWhileImmobileUntilAttackedIsSilent: a collar used while an
// ImmobileUntilAttacked effect holds the owner never reaches the cast: the
// all-skills-disabled check returns with no packet, not even SUMMON_A_PET.
func TestCollarWhileImmobileUntilAttackedIsSilent(t *testing.T) {
	t.Parallel()
	h := bootPostureCollar(t)
	mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)
	owner := h.character(t)
	e, err := effect.New(effect.Skill{ID: 4101, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "ImmobileUntilAttacked", Time: 60})
	if err != nil {
		t.Fatalf("effect.New(ImmobileUntilAttacked): %v", err)
	}
	e.Effector, e.Effected = owner, owner
	var denied bool
	runOn(t, h.srv.PlayerQueue(t, h.ownerID), func() {
		owner.EffectList().Add(e)
		denied = owner.DenyAIAction()
	})
	if !denied {
		t.Fatal("ImmobileUntilAttacked landed but the owner's AI actions are not denied")
	}
	drainFrames(t, h.client)

	h.client.Send(encodeUseItem(h.collarID, false))
	if frames := drainFrames(t, h.client); len(frames) != 0 {
		t.Fatalf("collar while immobile until attacked = opcodes %x, want silence", frameOpcodes(frames))
	}
	assertCollarRefusedUnspent(t, h, mpBefore, "the collar used while immobile until attacked")
}

// TestHeldCollarResumedWhileTeleportingGoesIdle: a collar held behind a
// stand-up, whose owner is teleporting when the stand-up ends, is answered
// with ActionFailed alone. SUMMON_A_PET is not repeated, no cast starts, and
// nothing is spent.
func TestHeldCollarResumedWhileTeleportingGoesIdle(t *testing.T) {
	t.Parallel()
	h := bootPostureCollar(t)
	mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)
	startOwnerStandUp(t, h)

	h.client.Send(encodeUseItem(h.collarID, false))
	assertHeldCollar(t, h, h.srv.ReadQueued(t, h.client), "collar mid stand-up")
	owner := h.character(t)
	if !owner.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}

	h.srv.SettlePosture(t, h.ownerID)
	frames := drainFrames(t, h.client)
	if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeActionFailed}; string(got) != string(want) {
		t.Fatalf("held collar resumed while teleporting = opcodes %x, want ActionFailed alone", got)
	}
	// The held cast went idle: clearing the teleport does not revive it.
	owner.SetTeleporting(false)
	if frames := drainFrames(t, h.client); len(frames) != 0 {
		t.Fatalf("after the teleport cleared = opcodes %x, want none", frameOpcodes(frames))
	}
	assertCollarRefusedUnspent(t, h, mpBefore, "the held collar resumed while teleporting")
}

// TestHeldCollarDroppedByTeleport: a real teleport idles the AI, which drops
// the collar cast held behind a stand-up (CreatureAI.onEvtTeleported ->
// doIdleIntention, whose prepareIntention clears the next intention,
// CreatureAI.java:90-93, PlayableAI.java:31-40). After Appearing and the
// stand-up's end, no cast starts, no pet comes, and nothing is spent.
func TestHeldCollarDroppedByTeleport(t *testing.T) {
	t.Parallel()
	h := bootPostureCollar(t)
	mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)
	startOwnerStandUp(t, h)

	h.client.Send(encodeUseItem(h.collarID, false))
	assertHeldCollar(t, h, h.srv.ReadQueued(t, h.client), "collar mid stand-up")

	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.character(t).TeleportTo(x+300, y, z, 0)
	readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner TeleportToLocation")
	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	h.srv.SettlePosture(t, h.ownerID)
	for _, frame := range drainFrames(t, h.client) {
		if frame[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatal("held collar cast started after the teleport")
		}
	}
	assertCollarRefusedUnspent(t, h, mpBefore, "the held collar across a teleport")
}
