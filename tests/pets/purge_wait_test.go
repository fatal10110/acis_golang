package pets

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestImmediatePurgeWaitsForQueuedPetSave returns a pet while its collar's
// persistence lane is backed up, so the pets-row save is still queued,
// restarts to character select and deletes the character with
// DeleteCharAfterDays = 0. The purge must wait for the queued save: once the
// lane drains, the character and its pets row are gone, rather than the row
// written back after the purge deleted it. The reference stores the row
// synchronously on unsummon, so its purge always follows the write.
func TestImmediatePurgeWaitsForQueuedPetSave(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithCharacterDeleteAfter(0),
		gameservertest.WithReuseDelays(0, 0),
	})
	release := heldCollarAtCharacterSelect(t, h)

	h.client.Send(encodeRequestCharacterDelete(0))
	// The delete is parked on the held lane: nothing is purged yet.
	h.srv.AwaitHandled(t)
	if _, err := h.srv.Chars.Get(context.Background(), h.ownerID); err != nil {
		t.Fatalf("character while the delete waits for the held lane: %v, want it kept", err)
	}

	release()
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharDeleteOk, "CharDeleteOk")
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo refresh")
	h.srv.FlushPersistence(t)
	if _, err := h.srv.Chars.Get(context.Background(), h.ownerID); !errors.Is(err, gamesql.ErrCharacterNotFound) {
		t.Fatalf("character after the purge: err %v, want ErrCharacterNotFound", err)
	}
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row after the purge: ok=%v err=%v, want no row", ok, err)
	}
}

// TestImmediatePurgeFailsWhenQueuedSavesStall deletes a character with
// DeleteCharAfterDays = 0 while its returned pet's save is stuck behind a
// held lane past the wait's bound. The delete fails with the deletion-failed
// reply and purges nothing, rather than purge under a save that could write
// a row back afterwards.
func TestImmediatePurgeFailsWhenQueuedSavesStall(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithCharacterDeleteAfter(0),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithPersistWait(200 * time.Millisecond),
		// The wait budget is a wall-clock deadline on the connection
		// goroutine; on a driven clock the read below would let virtual time
		// pass while the budget has not.
		gameservertest.WithRealPool(),
	})
	release := heldCollarAtCharacterSelect(t, h)

	h.client.Send(encodeRequestCharacterDelete(0))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeCharDeleteFail, "CharDeleteFail")
	fail := frames[len(frames)-1]
	if reason := wire.NewReader(fail[1:]).ReadInt32(); reason != int32(serverpackets.CharDeleteFailReasonDeletionFailed) {
		t.Fatalf("CharDeleteFail reason = %d, want deletion failed (%d)", reason, serverpackets.CharDeleteFailReasonDeletionFailed)
	}
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo refresh")
	if _, err := h.srv.Chars.Get(context.Background(), h.ownerID); err != nil {
		t.Fatalf("character after a failed delete: %v, want it kept", err)
	}

	release()
	h.srv.FlushPersistence(t)
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || !ok {
		t.Fatalf("pets row of the kept character: ok=%v err=%v, want the queued save written", ok, err)
	}
}

// heldCollarAtCharacterSelect summons h's wolf, holds its collar's
// persistence lane, returns the pet, so its pets-row save stays queued, and
// restarts to character select. It returns the lane's release.
func heldCollarAtCharacterSelect(t *testing.T, h *petWorld) (release func()) {
	t.Helper()
	if persist.LaneIndex(h.collarID) == persist.LaneIndex(h.ownerID) {
		t.Fatalf("owner %d and collar %d share a persistence lane; the scenario needs the restart to skip the collar's lane", h.ownerID, h.collarID)
	}
	h.spawnWolf(t)
	release = h.srv.HoldPersistenceLane(t, h.collarID)
	h.client.Send(encodeRequestActionUse(19, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	drainUntilQuiet(t, h.client)
	if _, ok, err := h.srv.Pets.Get(context.Background(), h.collarID); err != nil || ok {
		t.Fatalf("pets row while the lane is held: ok=%v err=%v, want no row yet", ok, err)
	}

	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, h.client, serverpackets.OpcodeRestartResponse, "RestartResponse")
	readUntilOpcode(t, h.client, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	drainUntilQuiet(t, h.client)
	return release
}

func encodeRequestCharacterDelete(slot int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterDelete)
	w.WriteInt32(slot)
	return w.Bytes()
}
