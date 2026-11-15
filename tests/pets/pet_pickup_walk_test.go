package pets

import (
	"encoding/binary"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// walksTo counts the MoveToLocation frames of the object id heading for
// destination x.
func walksTo(frames [][]byte, id int32, x int) int {
	n := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeMoveToLocation && len(f) >= 9 &&
			int32(binary.LittleEndian.Uint32(f[1:5])) == id && int32(binary.LittleEndian.Uint32(f[5:9])) == int32(x) {
			n++
		}
	}
	return n
}

// TestPetIgnoresItemOutOfSight stops the wolf following its owner, then
// commands it to loot adena two regions away, out of its sight. The pet
// does not walk for it: the owner hears ActionFailed, the pet goes back to
// following its owner, and the adena stays on the ground, no row moved.
func TestPetIgnoresItemOutOfSight(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	h.sendAndTick(t, encodeRequestActionUse(petFollowToggleAction, false))
	if wolf.FollowActive() {
		t.Fatal("FollowActive() after the follow toggle = true, want false")
	}
	x, y, z := wolf.Position()
	groundID := h.seedGroundAt(t, item.AdenaID, 40, x+5000, y, z)

	h.client.Send(encodeRequestPetGetItem(groundID))
	frames := syncFrames(t, h.client)
	if frameIndex(frames, serverpackets.OpcodeMoveToLocation, wolf.ObjectID()) >= 0 {
		t.Fatalf("out-of-sight pickup frames = %x, want no pet walk", frameOpcodes(frames))
	}
	if !hasOpcode(frames, serverpackets.OpcodeActionFailed) {
		t.Fatalf("out-of-sight pickup frames = %x, want ActionFailed", frameOpcodes(frames))
	}
	if !wolf.FollowActive() {
		t.Fatal("FollowActive() after the out-of-sight pickup = false, want following its owner again")
	}
	h.srv.Settle(t)
	if _, ok := h.srv.State.Object(groundID); !ok {
		t.Fatal("out-of-sight adena left the ground")
	}
	if got := h.collarItemCount(t, item.AdenaID); got != 0 {
		t.Fatalf("pet adena rows = %d, want none", got)
	}
}

// TestPetWalkToItemIsBroadcastOnce commands the wolf to loot adena far
// enough away that AI cycles run while it walks. The walk is broadcast once:
// no cycle restarts it before the pet takes the adena.
func TestPetWalkToItemIsBroadcastOnce(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithAITask()})
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	x, y, z := wolf.Position()
	groundID := h.seedGroundAt(t, item.AdenaID, 40, x+600, y, z)

	h.client.Send(encodeRequestPetGetItem(groundID))
	frames := syncFrames(t, h.client)
	for range 3 {
		h.srv.Advance(t, time.Second)
		if err := h.srv.AI.Tick(); err != nil {
			t.Fatalf("AI.Tick() = %v", err)
		}
		frames = append(frames, syncFrames(t, h.client)...)
	}
	if _, ok := h.srv.State.Object(groundID); !ok {
		t.Fatal("pet took the adena before the AI cycles ran mid-walk")
	}
	h.srv.AdvanceUntil(t, "pet pickup after its walk", func() bool {
		_, ok := h.srv.State.Object(groundID)
		return !ok
	})
	frames = append(frames, drainFrames(t, h.client)...)
	taken := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeGetItem })
	if taken < 0 {
		t.Fatalf("frames = %x, want the pet's GetItem", frameOpcodes(frames))
	}
	if got := walksTo(frames[:taken], wolf.ObjectID(), x+600); got != 1 {
		t.Fatalf("pet walks to the adena before GetItem = %d (%x), want exactly 1", got, frameOpcodes(frames[:taken]))
	}
	if got := h.collarItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("pet adena rows = %d, want 40", got)
	}
}
