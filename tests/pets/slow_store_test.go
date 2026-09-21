package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// slowStoreDelay is longer than the sim pool's 50 ms slow-task budget, so a
// pets-table call still made on an actor queue would be logged by the
// watchdog.
const slowStoreDelay = 120 * time.Millisecond

// TestSlowPetStoreKeepsQueuesFree covers the two pets-table reads an in-world
// flow still needs: the row a summon restores from, and the name-uniqueness
// check a rename makes. Both run off the owner's queue — the restore on the
// control item's persistence lane, the uniqueness check on the connection
// goroutine that is already waiting — so a pets table slower than the sim
// pool's slow-task budget stalls neither the queue nor the client's replies.
func TestSlowPetStoreKeepsQueuesFree(t *testing.T) {
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithCapturedLog(),
		gameservertest.WithSlowStores(slowStoreDelay),
	})

	pet, _ := h.spawnWolf(t)
	if got := pet.Name(); got != "Wolf" {
		t.Fatalf("fresh pet Name() = %q, want template name", got)
	}

	frames := renameTo(t, h, "Fenrir")
	var sawRefresh bool
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodePetInfo {
			sawRefresh = true
			if _, name := readPetInfoName(t, frame); name != "Fenrir" {
				t.Fatalf("refreshed PetInfo name = %q, want Fenrir", name)
			}
		}
	}
	if !sawRefresh {
		t.Fatalf("rename frames = opcodes %x, want a refreshed PetInfo", frameOpcodes(frames))
	}

	h.srv.Settle(t)
	if lines := h.srv.SlowTaskLogs(); len(lines) != 0 {
		t.Fatalf("queue task blocked on the pets table: %v", lines)
	}
	if state := h.savedPetState(t); state.Name != "Fenrir" {
		t.Fatalf("pets row name = %q, want Fenrir", state.Name)
	}
}

// TestSummonDropsCollarDestroyedDuringRestore covers the window the pets-row
// read opens: the read runs off the owner's queue, so the owner's own
// handlers keep running while it is outstanding. Destroying the collar in
// that window must leave no pet — the reference re-resolves the control item
// from the caster's inventory at use time and returns silently when it is
// gone (SummonCreature.java:34-41), and a pet built from a destroyed collar
// would keep answering to a pets row nobody holds.
func TestSummonDropsCollarDestroyedDuringRestore(t *testing.T) {
	h := bootOwnerWithCollarOpts(t,
		[]gameservertest.Option{
			gameservertest.WithCapturedLog(),
			gameservertest.WithSlowStores(slowStoreDelay),
		},
		seedItem{TemplateID: wyvernCollarID, Count: 1},
	)

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET system message"), serverpackets.SystemMessageSummonAPet)
	assertFrameOpcode(t, mustRead(t, h.client, "collar MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")

	// Runs as an ordinary queue task while the cast and its pets-row read
	// are in flight.
	h.client.Send(encodeRequestDestroyItem(h.collarID, 1))
	drainFrames(t, h.client)
	h.srv.Settle(t)
	h.srv.FlushPersistence(t)

	if obj, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatalf("pet %v spawned from a destroyed collar", obj)
	}
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row for destroyed collar: ok=%v err=%v, want none", ok, err)
	}

	// The summon slot this cast reserved for its pets-row read has to come
	// back when the spawn drops out, or the owner is locked out of every
	// summon and mount for the rest of the session.
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	frames := drainFrames(t, h.client)
	var mounted bool
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeRide {
			mounted = true
		}
		if frame[0] == serverpackets.OpcodeSystemMessage {
			assertNotSystemMessage(t, frame, serverpackets.SystemMessageSummonOnlyOne)
		}
	}
	if !mounted {
		t.Fatalf("wyvern collar after a dropped spawn = opcodes %x, want Ride: the summon slot stayed reserved", frameOpcodes(frames))
	}
}

// assertNotSystemMessage fails when frame is the given SystemMessage id.
func assertNotSystemMessage(t *testing.T, frame []byte, messageID int) {
	t.Helper()
	if id := wire.NewReader(frame[1:]).ReadInt32(); id == int32(messageID) {
		t.Fatalf("unexpected system message %d", messageID)
	}
}

// TestFirstSummonSpawnPrecedesCastFinish pins the packet order for the
// pets-row read a session's first summon has to make. The read runs off the
// owner's queue, so the spawn lands in a later task; the cast is held in
// flight until it has, and the client therefore sees the pet before the
// cast completes, exactly as the reference orders them.
//
// The reference resolves the row and spawns inside useSkill and only then
// schedules its finalizer (SummonCreature.java:28-77,
// PlayerCast.java:150-173). SUMMON_CREATURE carries no cool time, so
// Plan.FinalDelay is 0 and nothing but a deliberate hold keeps Finish from
// being armed the instant the Hit phase returns — which no database round
// trip could beat.
//
// Pet.restore is a synchronous query inside useSkill, so the reference's
// caster is in its cast across the identical read: the hold reproduces that
// rather than adding waiting of its own.
func TestFirstSummonSpawnPrecedesCastFinish(t *testing.T) {
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithSlowStores(slowStoreDelay),
	})

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET system message"), serverpackets.SystemMessageSummonAPet)
	assertFrameOpcode(t, mustRead(t, h.client, "collar MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")

	// The read is in flight and the pet is not in the world yet, so this is
	// the window the ordering is about. Without the hold the cast is
	// already over here: Finish is armed for zero delay off the Hit phase.
	if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
		t.Fatal("pet already in world: the restore window this test needs was never open")
	}
	if !petOwnerCastingNow(t, h.srv, h.ownerID) {
		t.Fatal("summon cast already finished while the pets-row read was still in flight: the client can see the cast complete before the pet it summoned")
	}

	var endedEarly bool
	waitFor(t, "pet in world state", func() bool {
		if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
			return true
		}
		if !petOwnerCastingNow(t, h.srv, h.ownerID) {
			endedEarly = true
		}
		return false
	})
	if endedEarly {
		t.Fatal("summon cast finished before the pet reached the world")
	}

	burst := drainFrames(t, h.client)
	var sawPetInfo, sawItemList bool
	for _, frame := range burst {
		switch frame[0] {
		case serverpackets.OpcodePetInfo:
			sawPetInfo = true
			if _, name := readPetInfoName(t, frame); name != "Wolf" {
				t.Fatalf("PetInfo name = %q, want Wolf", name)
			}
		case serverpackets.OpcodePetItemList:
			if !sawPetInfo {
				t.Fatalf("PetItemList arrived before PetInfo: opcodes %x", frameOpcodes(burst))
			}
			sawItemList = true
		}
	}
	if !sawPetInfo || !sawItemList {
		t.Fatalf("spawn burst = opcodes %x, want PetInfo then PetItemList", frameOpcodes(burst))
	}
}

// TestWyvernMountRejectedWhileSummonRestoreInFlight covers the summon slot
// across the same window. The reference takes it at hit time —
// SummonCreature.useSkill calls player.setSummon(pet) synchronously
// (SummonCreature.java:63) — and SummonItems.useItem rejects any pet or
// wyvern item while getSummon() != null || isMounted()
// (SummonItems.java:41-45). A wyvern collar used here must not mount the
// owner and leave the arriving pet on a mounted player, a state no
// reference path reaches.
func TestWyvernMountRejectedWhileSummonRestoreInFlight(t *testing.T) {
	h := bootOwnerWithCollarOpts(t,
		[]gameservertest.Option{gameservertest.WithSlowStores(slowStoreDelay)},
		seedItem{TemplateID: wyvernCollarID, Count: 1},
	)
	wyvernCollar := h.seeded[wyvernCollarID][0]

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET system message"), serverpackets.SystemMessageSummonAPet)
	assertFrameOpcode(t, mustRead(t, h.client, "collar MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")

	if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
		t.Fatal("pet already in world: the restore window this test needs was never open")
	}

	h.client.Send(encodeUseItem(wyvernCollar, false))
	frames := drainFrames(t, h.client)
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeRide {
			t.Fatalf("wyvern mounted while the summon restore was in flight: opcodes %x", frameOpcodes(frames))
		}
	}
	if _, ok := h.srv.State.Summon(h.ownerID); !ok {
		t.Fatal("pet never reached the world")
	}
}

// petOwnerCastingNow reports whether the owner has a cast in flight.
func petOwnerCastingNow(t *testing.T, srv *gameservertest.Server, objID int32) bool {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	caster, ok := obj.(interface{ CastingNow() bool })
	if !ok {
		t.Fatalf("world.Player(%d) = %T does not expose CastingNow", objID, obj)
	}
	return caster.CastingNow()
}
