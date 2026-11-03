package ai

import (
	"encoding/binary"
	"slices"
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// hasDisappeared reports whether frames tell the character that itemID
// disappeared.
func hasDisappeared(frames [][]byte, itemID int32) bool {
	return slices.ContainsFunc(frames, func(f []byte) bool {
		return len(f) >= 17 && f[0] == serverpackets.OpcodeSystemMessage &&
			int32(binary.LittleEndian.Uint32(f[1:5])) == serverpackets.SystemMessageS1HasDisappeared &&
			int32(binary.LittleEndian.Uint32(f[9:13])) == serverpackets.SystemMessageParamItemName &&
			int32(binary.LittleEndian.Uint32(f[13:17])) == itemID
	})
}

// TestNPCPickUpWaitsOutClaimedItem: an item another pickup holds (a
// player's pickup or the ground cleanup in flight) is not the NPC's to take.
// While the claim is held the NPC, within reach, raises no picked-item hook
// and the item stays in the world with the desire still queued; once the
// claim is released the next think picks it up, once.
func TestNPCPickUpWaitsOutClaimedItem(t *testing.T) {
	t.Parallel()
	w := bootGround(t)
	h := w.spawn(t, pickerHostileID, 200).(*npc.Hostile)
	potion := w.drop(t, potionID, 215)
	if !potion.Claim() {
		t.Fatal("the fresh item is already claimed")
	}
	w.look(t, h.Queue(), script.NPCOf(h), 500, potionID)
	w.log.take()

	var frames [][]byte
	for range 3 {
		frames = append(frames, cycle(t, w.srv)...)
	}
	if got := w.log.take(); len(got) != 0 {
		t.Fatalf("a claimed item told %q", got)
	}
	if len(framesOf(frames, serverpackets.OpcodeDeleteObject, potion.ObjectID())) != 0 {
		t.Fatal("the character saw a claimed item deleted")
	}
	if _, ok := w.srv.State.Object(potion.ObjectID()); !ok {
		t.Fatal("the claimed item left the world")
	}
	if !h.AI().Desires().Has(&actorai.Desire{Kind: actorai.IntentionPickUp, ItemObjectID: potion.ObjectID()}) {
		t.Fatal("the pick-up desire of a claimed item left the queue")
	}

	potion.Release()
	frames = cycle(t, w.srv)
	frames = append(frames, cycle(t, w.srv)...)
	if want := []string{line("picked %d %d/%d", pickerHostileID, potion.ObjectID(), potionID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("pickup after the release: want %q", want)
	}
	if len(framesOf(frames, serverpackets.OpcodeDeleteObject, potion.ObjectID())) != 1 {
		t.Fatal("the character saw no single DeleteObject of the picked item")
	}
	if _, ok := w.srv.State.Object(potion.ObjectID()); ok {
		t.Fatal("the picked item is still in the world")
	}
}

// TestNPCPicksUpStrayCursedWeapon: an NPC picking up a cursed weapon item
// that is not the weapon's own ground object (here the weapon is not out
// at all) ends the weapon's life, and every player is told it disappeared,
// but the item itself stays on the ground, free for anyone to take.
func TestNPCPicksUpStrayCursedWeapon(t *testing.T) {
	t.Parallel()
	w := bootGround(t)
	h := w.spawn(t, pickerHostileID, 200).(*npc.Hostile)
	stray := w.drop(t, zaricheItemID, 215)
	w.look(t, h.Queue(), script.NPCOf(h), 500, zaricheItemID)
	w.log.take()

	frames := cycle(t, w.srv)
	frames = append(frames, cycle(t, w.srv)...)
	if want := []string{line("picked %d %d/%d", pickerHostileID, stray.ObjectID(), zaricheItemID)}; !slices.Equal(w.log.take(), want) {
		t.Fatalf("pickup: want %q", want)
	}
	if !hasDisappeared(frames, zaricheItemID) {
		t.Fatal("the character was not told Zariche disappeared")
	}
	if len(framesOf(frames, serverpackets.OpcodeDeleteObject, stray.ObjectID())) != 0 {
		t.Fatal("the character saw the stray item deleted")
	}
	if _, ok := w.srv.State.Object(stray.ObjectID()); !ok {
		t.Fatal("the stray item left the world")
	}
	if !stray.Claim() {
		t.Fatal("the stray item stayed claimed after the pickup")
	}
	stray.Release()
	if h.AI().Desires().Len() != 0 {
		t.Fatalf("desires after the pickup = %d, want none", h.AI().Desires().Len())
	}
	cycle(t, w.srv)
	if got := w.log.take(); len(got) != 0 {
		t.Fatalf("a later tick told %q, want the pickup once", got)
	}
}

// TestFolkPickUpOfGoneItem: a civilian NPC's pick-up of an item that left
// the ground raises no hook and walks nowhere; the desire stays queued.
func TestFolkPickUpOfGoneItem(t *testing.T) {
	t.Parallel()
	w := bootGround(t)
	f := w.spawn(t, pickerFolkID, 200).(*npc.Folk)
	potion := w.drop(t, potionID, 500)
	w.look(t, f.Queue(), script.NPCOf(f), 500, potionID)
	w.log.take()
	w.srv.DespawnGroundItem(t, potion.ObjectID())

	var frames [][]byte
	for range 3 {
		frames = append(frames, cycle(t, w.srv)...)
	}
	if got := w.log.take(); len(got) != 0 {
		t.Fatalf("a gone item told %q", got)
	}
	if len(framesOf(frames, serverpackets.OpcodeMoveToLocation, f.ObjectID())) != 0 {
		t.Fatal("the NPC walked toward a gone item")
	}
	if !f.Desires().Has(&actorai.Desire{Kind: actorai.IntentionPickUp, ItemObjectID: potion.ObjectID()}) {
		t.Fatal("the pick-up desire of a gone item left the queue")
	}
}

// TestFolkPickUpGates: a civilian NPC's pick-up of object 0 queues nothing,
// and one that cannot move does not walk to its item until it can again.
func TestFolkPickUpGates(t *testing.T) {
	t.Parallel()
	w := bootGround(t)
	f := w.spawn(t, pickerFolkID, 200).(*npc.Folk)
	before := f.Desires().Len()
	f.AddPickUpDesire(0, 1_000_000)
	if got := f.Desires().Len(); got != before {
		t.Fatalf("desires after a pick-up of object 0 = %d, want %d", got, before)
	}

	potion := w.drop(t, potionID, 500)
	f.SetImmobilized(true)
	w.look(t, f.Queue(), script.NPCOf(f), 500, potionID)
	w.log.take()
	var frames [][]byte
	for range 3 {
		frames = append(frames, cycle(t, w.srv)...)
	}
	if len(framesOf(frames, serverpackets.OpcodeMoveToLocation, f.ObjectID())) != 0 {
		t.Fatal("an NPC that cannot move walked toward the item")
	}
	if got := w.log.take(); len(got) != 0 {
		t.Fatalf("an NPC out of reach told %q", got)
	}
	if !f.Desires().Has(&actorai.Desire{Kind: actorai.IntentionPickUp, ItemObjectID: potion.ObjectID()}) {
		t.Fatal("the pick-up desire left the queue")
	}

	f.SetImmobilized(false)
	moves := framesOf(cycle(t, w.srv), serverpackets.OpcodeMoveToLocation, f.ObjectID())
	if len(moves) != 1 || moveDestination(moves[0]).X != w.at.X+500 {
		t.Fatalf("walks toward the item once it can move = %d, want 1 to x %d", len(moves), w.at.X+500)
	}
}
