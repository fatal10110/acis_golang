package pets

import (
	"context"
	"testing"
	"time"

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
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithCapturedLog(),
		gameservertest.WithSlowStores(slowStoreDelay),
	})

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
}
