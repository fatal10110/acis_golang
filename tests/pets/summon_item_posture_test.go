package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The summon-item sitting gate reads Player.isSitting(), which a sit-down
// sets only when it ends and a stand-up clears as it starts
// (Player.java:1542-1590, SummonItems.java:28-32). A pet collar used while
// either transition runs passes that gate, and tryToCast parks its
// SUMMON_CREATURE as the next intention with ActionFailed because
// isSittingNow()/isStandingNow() hold (PlayableAI.java:313-318); SUMMON_A_PET
// follows (SummonItems.java:95-96). SAT_DOWN/STOOD_UP then run the parked cast
// (PlayerAI.java:100-124), and thinkCast's canAttemptCast refuses it with
// CANT_MOVE_SITTING alone once seated (PlayerAI.java:242-243,
// PlayerCast.java:212-216).

// collarPostureMP is the MP the posture scenarios' SUMMON_CREATURE costs, so
// a refused cast shows it left MP untouched.
const collarPostureMP = 2

// startOwnerSitDown starts the owner's sit-down and returns with it still
// in progress.
func startOwnerSitDown(t *testing.T, h *petWorld) {
	t.Helper()
	if !h.srv.DrivesClock() {
		t.Skip("holding a sit-down open needs the driven clock")
	}
	h.client.Send(encodeRequestChangeWaitType(false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeWaitType, "sit ChangeWaitType")
	h.srv.ReadQueued(t, h.client)
}

// startOwnerStandUp seats the owner, then starts its stand-up and returns
// with it still in progress.
func startOwnerStandUp(t *testing.T, h *petWorld) {
	t.Helper()
	if !h.srv.DrivesClock() {
		t.Skip("holding a stand-up open needs the driven clock")
	}
	sitOwner(t, h)
	h.client.Send(encodeRequestChangeWaitType(true))
	readUntilOpcode(t, h.client, serverpackets.OpcodeChangeWaitType, "stand ChangeWaitType")
	h.srv.ReadQueued(t, h.client)
}

// assertHeldCollar checks a collar use answered by ActionFailed then
// SUMMON_A_PET, with no cast started.
func assertHeldCollar(t *testing.T, h *petWorld, frames [][]byte, what string) {
	t.Helper()
	if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage}; string(got) != string(want) {
		t.Fatalf("%s = opcodes %x, want ActionFailed then SUMMON_A_PET", what, got)
	}
	assertStaticSystemMessage(t, frames[1], serverpackets.SystemMessageSummonAPet)
	if h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatalf("%s started the cast during the transition", what)
	}
}

// bootPostureCollar boots an owner whose collar casts a zero-time
// SUMMON_CREATURE costing collarPostureMP.
func bootPostureCollar(t *testing.T) *petWorld {
	t.Helper()
	summon := summonCreature()
	summon.HitTime = 0
	summon.MPConsume = collarPostureMP
	return bootCollarSkill(t, summon)
}

// TestCollarDuringSitDownIsHeldThenRefusedOnceSeated pins a collar used
// mid sit-down: held with ActionFailed and SUMMON_A_PET, then refused with
// CANT_MOVE_SITTING alone when the sit-down ends. No cast starts, no pet
// comes, and neither MP nor the collar is spent.
func TestCollarDuringSitDownIsHeldThenRefusedOnceSeated(t *testing.T) {
	t.Parallel()
	h := bootPostureCollar(t)
	mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)
	startOwnerSitDown(t, h)

	h.client.Send(encodeUseItem(h.collarID, false))
	assertHeldCollar(t, h, h.srv.ReadQueued(t, h.client), "collar mid sit-down")

	h.srv.SettlePosture(t, h.ownerID)
	frames := drainFrames(t, h.client)
	if len(frames) != 1 {
		t.Fatalf("held collar once seated = opcodes %x, want CANT_MOVE_SITTING alone", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageCannotMoveWhileSitting)
	if _, ok := h.srv.State.Summon(h.ownerID); ok || h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("held collar summoned or started a cast once seated")
	}
	if got := h.srv.PlayerCurrentMP(t, h.ownerID); got != mpBefore {
		t.Fatalf("MP after the refused collar = %d, want unchanged %d", got, mpBefore)
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("collar count = %d after the refused cast, want 1", got)
	}
}

// TestCollarDuringStandUpIsHeldUntilUp pins a collar used mid stand-up:
// held with ActionFailed and SUMMON_A_PET, no MagicSkillUse while the
// stand-up runs, then the cast starts when it ends and summons the pet.
func TestCollarDuringStandUpIsHeldUntilUp(t *testing.T) {
	t.Parallel()
	h := bootCollarSkill(t, summonCreature())
	startOwnerStandUp(t, h)

	h.client.Send(encodeUseItem(h.collarID, false))
	assertHeldCollar(t, h, h.srv.ReadQueued(t, h.client), "collar mid stand-up")

	// Well short of the stand-up's end the cast is still held.
	h.srv.Advance(t, 2*time.Second)
	if frames := h.srv.ReadQueued(t, h.client); len(frames) != 0 {
		t.Fatalf("held collar before the stand-up ended = opcodes %x, want none", frameOpcodes(frames))
	}
	if h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("held collar started before the stand-up ended")
	}

	h.srv.SettlePosture(t, h.ownerID)
	frame := mustRead(t, h.client, "held collar MagicSkillUse")
	assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "held collar MagicSkillUse")
	r := wire.NewReader(frame[1:])
	if caster, target, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != h.ownerID || target != h.ownerID || skill != summonCreatureID {
		t.Fatalf("held collar cast = caster %d target %d skill %d, want %d/%d/%d", caster, target, skill, h.ownerID, h.ownerID, summonCreatureID)
	}
	assertFrameOpcode(t, mustRead(t, h.client, "held collar SetupGauge"), serverpackets.OpcodeSetupGauge, "held collar SetupGauge")
	// SUMMON_A_PET went out with the hold; the resumed cast does not repeat it.
	if frames := h.srv.ReadQueued(t, h.client); len(frames) != 0 {
		t.Fatalf("held collar after its cast start = opcodes %x, want none", frameOpcodes(frames))
	}
	if !h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("held collar cast not in flight once up")
	}
	h.srv.AdvanceUntil(t, "pet in world state", func() bool {
		_, ok := h.srv.State.Summon(h.ownerID)
		return ok
	})
	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("collar count = %d after the summon, want 1", got)
	}
}

// TestHeldCollarDestroyedBeforeUpIsRefused pins the held cast's item check
// (PlayableCast.canCast, PlayableCast.java:81-85): a collar destroyed while
// its cast is held refuses the cast with NOT_ENOUGH_ITEMS once the stand-up
// ends. No cast starts, no pet comes, and no MP is spent.
func TestHeldCollarDestroyedBeforeUpIsRefused(t *testing.T) {
	t.Parallel()
	h := bootPostureCollar(t)
	mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)
	startOwnerStandUp(t, h)

	h.client.Send(encodeUseItem(h.collarID, false))
	assertHeldCollar(t, h, h.srv.ReadQueued(t, h.client), "collar mid stand-up")
	h.client.Send(encodeRequestDestroyItem(h.collarID, 1))
	h.srv.ReadQueued(t, h.client)
	if got := h.ownerItemCount(t, wolfCollarID); got != 0 {
		t.Fatalf("collar count = %d after the destroy, want 0", got)
	}

	h.srv.SettlePosture(t, h.ownerID)
	frames := drainFrames(t, h.client)
	if len(frames) != 1 {
		t.Fatalf("held collar gone once up = opcodes %x, want NOT_ENOUGH_ITEMS alone", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageNotEnoughItems)
	if _, ok := h.srv.State.Summon(h.ownerID); ok || h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("destroyed collar summoned or started a cast")
	}
	if got := h.srv.PlayerCurrentMP(t, h.ownerID); got != mpBefore {
		t.Fatalf("MP after the refused collar = %d, want unchanged %d", got, mpBefore)
	}
}

// TestSummonItemsDuringPostureTransitionProceed pins the tree kit and the
// wyvern collar during a sit-down or stand-up: neither is a cast, so with
// the sitting gate passed they plant and mount at once
// (SummonItems.java:66-87, 99-102).
func TestSummonItemsDuringPostureTransitionProceed(t *testing.T) {
	t.Parallel()
	for _, posture := range []struct {
		name  string
		start func(*testing.T, *petWorld)
	}{
		{name: "sit-down", start: startOwnerSitDown},
		{name: "stand-up", start: startOwnerStandUp},
	} {
		t.Run("tree kit mid "+posture.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t, seedItem{TemplateID: treeKitID, Count: 1})
			posture.start(t, h)

			h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
			assertNoSittingRefusal(t, h.srv.ReadQueued(t, h.client), "tree kit mid "+posture.name)
			if decorationCount(h) != 1 {
				t.Fatalf("tree kit mid %s planted %d decorations, want 1", posture.name, decorationCount(h))
			}
			if got := h.ownerItemCount(t, treeKitID); got != 0 {
				t.Fatalf("tree kit count = %d after planting, want 0", got)
			}
		})
		t.Run("wyvern collar mid "+posture.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t, seedItem{TemplateID: wyvernCollarID, Count: 1})
			posture.start(t, h)

			h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
			assertNoSittingRefusal(t, readUntilOpcode(t, h.client, serverpackets.OpcodeRide, "mount Ride"), "wyvern collar mid "+posture.name)
			if !h.character(t).Mounted() {
				t.Fatalf("wyvern collar mid %s did not mount", posture.name)
			}
		})
	}
}

// TestSummonItemsWhileSeatedAnswerCannotMoveSitting pins the sitting gate
// once seated (SummonItems.java:28-32): every summon kind is refused with
// CANT_MOVE_SITTING alone, and nothing is cast, mounted, planted or spent.
func TestSummonItemsWhileSeatedAnswerCannotMoveSitting(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		template int32
	}{
		{name: "tree kit", template: treeKitID},
		{name: "pet collar", template: wolfCollarID},
		{name: "wyvern collar", template: wyvernCollarID},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var seeds []seedItem
			if tt.template != wolfCollarID {
				seeds = append(seeds, seedItem{TemplateID: tt.template, Count: 1})
			}
			h := bootOwnerWithCollar(t, seeds...)
			used := h.collarID
			if tt.template != wolfCollarID {
				used = h.seededItem(t, tt.template)
			}
			sitOwner(t, h)

			h.client.Send(encodeUseItem(used, false))
			frames := drainFrames(t, h.client)
			if len(frames) != 1 {
				t.Fatalf("%s seated = opcodes %x, want CANT_MOVE_SITTING alone", tt.name, frameOpcodes(frames))
			}
			assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageCannotMoveWhileSitting)
			if h.srv.PlayerCastingNow(t, h.ownerID) || h.character(t).Mounted() || decorationCount(h) != 0 {
				t.Fatalf("%s seated cast, mounted or planted", tt.name)
			}
			if got := h.ownerItemCount(t, tt.template); got != 1 {
				t.Fatalf("%s count = %d after a seated use, want 1", tt.name, got)
			}
		})
	}
}

// assertNoSittingRefusal fails when frames carry CANT_MOVE_SITTING.
func assertNoSittingRefusal(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == serverpackets.SystemMessageCannotMoveWhileSitting {
			t.Fatalf("%s answered CANT_MOVE_SITTING: opcodes %x", what, frameOpcodes(frames))
		}
	}
}

// TestCollarAttemptGateRefusesBeforeHoldOrStart pins tryToCast running
// canAttemptCast before it decides to hold or start the cast
// (PlayableAI.java:304-318): in formal wear the collar is refused with
// CANNOT_USE_ITEMS_SKILLS_WITH_FORMALWEAR (PlayerCast.java:193-197) and
// ActionFailed, SUMMON_A_PET follows (SummonItems.java:95-96), and nothing
// is held, cast or summoned, standing or mid stand-up alike.
func TestCollarAttemptGateRefusesBeforeHoldOrStart(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		start func(*testing.T, *petWorld)
	}{
		{name: "standing", start: func(*testing.T, *petWorld) {}},
		{name: "mid stand-up", start: startOwnerStandUp},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bootOwnerWithCollar(t, seedItem{TemplateID: gameservertest.FormalWearID, Count: 1})
			h.client.Send(encodeUseItem(h.seededItem(t, gameservertest.FormalWearID), false))
			drainUntilQuiet(t, h.client)
			tt.start(t, h)

			h.client.Send(encodeUseItem(h.collarID, false))
			frames := h.srv.ReadQueued(t, h.client)
			if got, want := frameOpcodes(frames), []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeSystemMessage}; string(got) != string(want) {
				t.Fatalf("collar in formal wear %s = opcodes %x, want %x", tt.name, got, want)
			}
			assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageCannotUseSkillsWithFormalWear)
			assertStaticSystemMessage(t, frames[2], serverpackets.SystemMessageSummonAPet)

			h.srv.Advance(t, 3*time.Second)
			if frames := drainFrames(t, h.client); len(frames) != 0 {
				t.Fatalf("refused collar %s later sent opcodes %x, want none", tt.name, frameOpcodes(frames))
			}
			if _, ok := h.srv.State.Summon(h.ownerID); ok || h.srv.PlayerCastingNow(t, h.ownerID) {
				t.Fatalf("collar in formal wear %s summoned or started a cast", tt.name)
			}
		})
	}
}
