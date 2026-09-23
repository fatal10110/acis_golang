package pets

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
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
	if !h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("summon cast already finished while the pets-row read was still in flight: the client can see the cast complete before the pet it summoned")
	}

	var endedEarly bool
	waitFor(t, "pet in world state", func() bool {
		if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
			return true
		}
		if !h.srv.PlayerCastingNow(t, h.ownerID) {
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

// TestAutoSoulShotRejectedDuringRestore pins the summon slot's state across
// the pets-row read: empty, the same answer the reference gives.
//
// The reference resolves the row first and only then claims the slot —
// Pet.restore at SummonCreature.java:58, World.addPet at :62,
// player.setSummon(pet) at :64 — so for the whole duration of that read
// getSummon() is still null. RequestAutoSoulShot is handled on a packet
// thread concurrently with the cast task and takes its else branch,
// answering NO_SERVITOR_CANNOT_AUTOMATE_USE (RequestAutoSoulShot.java:42,76)
// for a beast soulshot toggled across the window.
//
// Go must answer the same, so hasActiveSummon reads world.Summon alone. An
// early claim would both invert this message and leave hasActiveSummon
// reporting a summon that world.Summon cannot produce — the reference's own
// getSummon() != null branch dereferences it immediately (:53, :61, :73).
//
// The wyvern collar is rejected in this same window instead by the cast the
// hold keeps in flight, which is how the reference rejects it too
// (SummonItems.java:36-45 checks isCastingNow() before the summon slot) —
// see TestWyvernMountRejectedWhileSummonRestoreInFlight.
func TestAutoSoulShotRejectedDuringRestore(t *testing.T) {
	h := bootOwnerWithCollarOpts(t,
		[]gameservertest.Option{gameservertest.WithSlowStores(slowStoreDelay)},
		seedItem{TemplateID: beastSoulshotID, Count: 10},
	)

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET system message"), serverpackets.SystemMessageSummonAPet)
	assertFrameOpcode(t, mustRead(t, h.client, "collar MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")

	if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
		t.Fatal("pet already in world: the restore window this test needs was never open")
	}

	h.client.Send(encodeRequestAutoSoulShot(beastSoulshotID, 1))
	frames := drainFrames(t, h.client)
	var rejected bool
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeSystemMessage {
			r := wire.NewReader(frame[1:])
			if r.ReadInt32() == int32(serverpackets.SystemMessageNoServitorCannotAutomateUse) {
				rejected = true
			}
			continue
		}
		if isExAutoSoulShot(t, frame) {
			t.Fatalf("auto soulshot acknowledged during the restore: opcodes %x, want the reference's NO_SERVITOR_CANNOT_AUTOMATE_USE while the summon slot is still empty", frameOpcodes(frames))
		}
	}
	if !rejected {
		t.Fatalf("auto soulshot toggle during the restore = opcodes %x, want NO_SERVITOR_CANNOT_AUTOMATE_USE", frameOpcodes(frames))
	}
	if _, ok := h.srv.State.Summon(h.ownerID); !ok {
		t.Fatal("pet never reached the world")
	}
}

// isExAutoSoulShot reports whether frame is an ExAutoSoulShot acknowledgement
// specifically, rather than any extended packet that drifted into the drain.
// Layout: OpcodeExtended, sub-opcode, item id, enabled flag
// (serverpackets.FrameExAutoSoulShot).
func isExAutoSoulShot(t *testing.T, frame []byte) bool {
	t.Helper()
	if len(frame) == 0 || frame[0] != serverpackets.OpcodeExtended {
		return false
	}
	r := wire.NewReader(frame[1:])
	if r.ReadUint16() != serverpackets.OpcodeExAutoSoulShot {
		return false
	}
	itemID, enabled := r.ReadInt32(), r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read ExAutoSoulShot: %v", err)
	}
	return itemID == beastSoulshotID && enabled == 1
}

func encodeRequestAutoSoulShot(itemID, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestAutoSoulShot)
	w.WriteInt32(itemID)
	w.WriteInt32(typ)
	return w.Bytes()
}

// ceilingStoreDelay sits between summon_spawn.go's petRestoreHoldCeiling and
// its petRestoreTimeout, so the hold's deadline fires while the pets-row read
// is still outstanding and the read itself still succeeds afterwards.
const ceilingStoreDelay = 3 * time.Second

// TestSummonHoldReleasesAtItsCeiling pins the bound on how long a summon
// cast is held open for its pets-row read. The hold exists to keep the spawn
// ahead of the cast's completion, but the job first waits its turn on a
// persistence lane shared by every owner, and nothing bounds that wait — a
// burst of logout saves on the same lane would otherwise keep the caster
// casting long after the client's own cast bar ended, blocking target
// changes, pickup, unequip and item-skill casts. The reference never waits
// that way: its read has nothing queued ahead of it (SummonCreature.java:58).
//
// Past the ceiling the ordering guarantee yields to keeping the player
// responsive, so the cast completes first and the pet lands afterwards, as
// it did before the hold existed.
func TestSummonHoldReleasesAtItsCeiling(t *testing.T) {
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithSlowStores(ceilingStoreDelay),
	})

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET system message"), serverpackets.SystemMessageSummonAPet)
	assertFrameOpcode(t, mustRead(t, h.client, "collar MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")

	deadline := time.Now().Add(ceilingStoreDelay + 5*time.Second)
	var castEnded bool
	for time.Now().Before(deadline) {
		if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
			break
		}
		if !h.srv.PlayerCastingNow(t, h.ownerID) {
			castEnded = true
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, spawned := h.srv.State.Summon(h.ownerID); !spawned {
		t.Fatal("pet never reached the world")
	}
	if !castEnded {
		t.Fatal("the summon cast was still in flight for the whole read: the hold has no ceiling, so a stalled persistence lane keeps the caster casting for as long as it stalls")
	}
}

// TestWyvernMountRejectedAfterHoldCeiling covers the window the hold's own
// ceiling opens. Past the ceiling the cast is over while the pets-row read
// is still outstanding, so the barrier that rejects a wyvern collar during a
// normal restore — the cast itself, matching SummonItems.java:36-37's
// isCastingNow() return before the summon-slot check at :41-45 — is gone,
// and world.Summon is still empty because the pet has not landed.
//
// The reference never reaches that state: its read is one synchronous call
// inside useSkill, with isCastingNow() true throughout. Go has to hold the
// slot explicitly for as long as the restore is in flight, ceiling or not,
// or the owner ends up mounted with a pet arriving beside them.
func TestWyvernMountRejectedAfterHoldCeiling(t *testing.T) {
	h := bootOwnerWithCollarOpts(t,
		[]gameservertest.Option{gameservertest.WithSlowStores(ceilingStoreDelay)},
		seedItem{TemplateID: wyvernCollarID, Count: 1},
	)
	wyvernCollar := h.seeded[wyvernCollarID][0]

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET system message"), serverpackets.SystemMessageSummonAPet)
	assertFrameOpcode(t, mustRead(t, h.client, "collar MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")

	// Wait for the ceiling to end the cast, with the read still running.
	deadline := time.Now().Add(ceilingStoreDelay)
	for time.Now().Before(deadline) {
		if !h.srv.PlayerCastingNow(t, h.ownerID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("cast never ended: the ceiling window this test needs was never open")
	}
	if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
		t.Fatal("pet already in world: the restore was no longer in flight")
	}

	h.client.Send(encodeUseItem(wyvernCollar, false))
	frames := drainFrames(t, h.client)
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeRide {
			t.Fatalf("wyvern mounted after the hold ceiling while the pet was still inbound: opcodes %x", frameOpcodes(frames))
		}
	}

	// The rejection is silent, so the drain above returns long before the
	// read lands; wait the rest of it out to prove the pet still arrives.
	petDeadline := time.Now().Add(ceilingStoreDelay + 5*time.Second)
	for time.Now().Before(petDeadline) {
		if _, ok := h.srv.State.Summon(h.ownerID); ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("pet never reached the world")
}

// TestSecondCollarSilentAfterHoldCeiling covers the pet branch of item use
// in the window the hold ceiling opens. Past the ceiling the cast is over
// while the pets-row read is still in flight, so StartItemSkill's
// already-casting rejection no longer fires and nothing else stood between a
// second collar use and a full second cast.
//
// The reference returns silently: SummonItems.useItem checks
// isAllSkillsDisabled() || isCastingNow() at :37-38, above the
// switch (sitem.getValue()) at :64-65, so the pet case never starts a cast
// while the first one's read is outstanding. A cast that runs and then
// rejects broadcasts MagicSkillUse and MagicSkillLaunched to everyone nearby
// before answering, which the reference never sends here.
func TestSecondCollarSilentAfterHoldCeiling(t *testing.T) {
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithSlowStores(ceilingStoreDelay),
	})

	h.client.Send(encodeUseItem(h.collarID, false))
	assertStaticSystemMessage(t, mustRead(t, h.client, "SUMMON_A_PET system message"), serverpackets.SystemMessageSummonAPet)
	assertFrameOpcode(t, mustRead(t, h.client, "collar MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "collar MagicSkillUse")

	deadline := time.Now().Add(ceilingStoreDelay)
	for time.Now().Before(deadline) {
		if !h.srv.PlayerCastingNow(t, h.ownerID) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("cast never ended: the ceiling window this test needs was never open")
	}
	if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
		t.Fatal("pet already in world: the restore was no longer in flight")
	}

	// Clear the first cast's own tail (its MagicSkillLaunched broadcast)
	// so what the drain below collects is the second use's answer alone.
	drainFrames(t, h.client)

	// The same collar object: the fixture's owner holds exactly one, and a
	// second seeded stack of the same template is not in the inventory at
	// enter-world, so useItem would bail before reaching this path.
	h.client.Send(encodeUseItem(h.collarID, false))
	frames := drainFrames(t, h.client)
	if len(frames) != 0 {
		t.Fatalf("second collar past the hold ceiling = opcodes %x, want silence: the reference returns at SummonItems.java:37-38 without starting a cast", frameOpcodes(frames))
	}

	petDeadline := time.Now().Add(ceilingStoreDelay + 5*time.Second)
	for time.Now().Before(petDeadline) {
		if _, ok := h.srv.State.Summon(h.ownerID); ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("pet never reached the world")
}
