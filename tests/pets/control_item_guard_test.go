package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: Player.checkItemManipulation and Player.validateItemManipulation
// (Player.java:2061-2089, 6201-6222) refuse the control item of the summoned
// or mounted pet: `_summon != null && _summon.getControlItemId() == objectId
// || _mountObjectId == objectId`. Trade adds (AddTradeItem.java:67-72,
// NOTHING_HAPPENED), trade settlement (TradeList.java:295-299,338: the trade
// is cancelled), drop (RequestDropItem.java:39-44, CANNOT_DISCARD_THIS_ITEM,
// ahead of the distance check at :88-92) and the hand-over to the pet
// (RequestGiveItemToPet.java:89 -> Player.transferItem, Player.java:1998,
// silent) go through them; RequestDestroyItem.java:79-86 answers
// PET_SUMMONED_MAY_NOT_DESTROYED. aCis revision in the outer repo.

// petSummonedMayNotDestroyed is SystemMessageId.PET_SUMMONED_MAY_NOT_DESTROYED
// (SystemMessageId.java: new SystemMessageId(557)).
const petSummonedMayNotDestroyed = 557

// discardableCollarItems is the fixture catalog with both collars
// droppable, as the datapack's collars are (no is_dropable, so the default
// true), so a drop of one is refused only for the pet it calls out.
func discardableCollarItems() *item.Table {
	all := gameservertest.ItemTemplates().All()
	for _, tmpl := range all {
		if tmpl.ID == wolfCollarID || tmpl.ID == wyvernCollarID {
			tmpl.Dropable = true
		}
	}
	return item.NewTable(all)
}

// TestSummonedPetCollarStaysWithItsOwner calls out the wolf, then tries to
// trade, drop (in reach and out of it), destroy and hand its collar to the
// wolf itself. Each is refused with the reference's answer and the collar
// stays in the owner's inventory; once the wolf is returned, the same collar
// drops.
func TestSummonedPetCollarStaysWithItsOwner(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(discardableCollarItems())})
	buyer := h.joinSecondPlayer(t, "Buyer")
	wolf, _ := h.spawnWolf(t)

	openTrade(t, h, buyer)
	h.client.Send(encodeAddTradeItem(0, h.collarID, 1))
	assertStaticSystemMessage(t, mustRead(t, h.client, "NOTHING_HAPPENED"), serverpackets.SystemMessageNothingHappened)
	requireNoFrame(t, h.client, "trade add of the summoned pet's collar")
	requireNoFrame(t, buyer.client, "partner after a refused collar add")
	h.client.Send(encodeTradeDone(0))
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer.client)

	h.requireCollarRefusals(t)

	h.client.Send(encodeRequestGiveItemToPet(h.collarID, 1))
	h.syncOnSkillList(t)
	h.srv.InventoryUpdates.Tick()
	drainUntilQuiet(t, h.client)
	if got := petItemCount(wolf, wolfCollarID); got != 0 {
		t.Fatalf("pet holds %d of its own collar, want none", got)
	}
	if h.ownerInventory(t).ItemByObjectID(h.collarID) == nil {
		t.Fatal("collar left the owner's inventory on a hand-over to its own pet")
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("owner collar rows = %d, want the collar kept", got)
	}

	h.returnPet(t)
	x, y, z := h.character(t).Position()
	h.client.Send(encodeRequestDropItem(h.collarID, 1, int32(x), int32(y), int32(z)))
	readUntilOpcode(t, h.client, serverpackets.OpcodeDropItem, "DropItem of the returned pet's collar")
}

// TestCollarOfferedBeforeItsPetLandsCancelsTrade starts the collar's summon
// cast, opens a trade and offers the collar while the summon is still
// reading its pets row (the reference's own slot is empty across that read,
// SummonCreature.java:58-64), then lets the wolf land before both sides
// confirm. Settlement
// re-checks every offered item, so the trade is cancelled for both players
// and the collar stays with its owner.
func TestCollarOfferedBeforeItsPetLandsCancelsTrade(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	buyer := h.joinSecondPlayer(t, "Buyer")

	// The summon holds on its pets-row read, so the pet is not out yet.
	release := h.useCollarRestoreHeld(t)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer.client)
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("pet landed before the trade opened")
	}

	openTrade(t, h, buyer)
	h.client.Send(encodeAddTradeItem(0, h.collarID, 1))
	readUntilOpcode(t, h.client, serverpackets.OpcodeTradeOwnAdd, "TradeOwnAdd of the collar")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer.client)

	release()
	h.awaitPet(t)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer.client)

	h.client.Send(encodeTradeDone(1))
	drainUntilQuiet(t, buyer.client)
	drainUntilQuiet(t, h.client)
	buyer.client.Send(encodeTradeDone(1))
	for _, c := range []*testsupport.ScriptedClient{h.client, buyer.client} {
		frames := readUntilOpcode(t, c, serverpackets.OpcodeSendTradeDone, "trade cancel")
		if done := frames[len(frames)-1]; wire.NewReader(done[1:]).ReadInt32() != 0 {
			t.Fatal("trade with the summoned pet's collar settled, want it cancelled")
		}
		drainUntilQuiet(t, c)
	}
	if h.ownerInventory(t).ItemByObjectID(h.collarID) == nil {
		t.Fatal("collar left its owner through a cancelled trade")
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("owner collar rows = %d, want the collar kept", got)
	}
}

// TestDeadPetCollarStaysWithReturningOwner logs the owner out with a dead
// wolf and back in before the corpse decays. The corpse is the new session's
// pet, so its collar is still bound: trade, drop, and destroy are refused.
func TestDeadPetCollarStaysWithReturningOwner(t *testing.T) {
	t.Parallel()
	decay := newCorpseDecay(t)
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithItemTemplates(discardableCollarItems()),
		gameservertest.WithDecay(decay.task), gameservertest.WithReuseDelays(0, 0),
	})
	decay.attach(h.srv.State)
	wolf, _ := h.spawnWolf(t)
	killPet(t, h, wolf)
	h.relogOwner(t)
	if obj, ok := h.srv.State.Summon(h.ownerID); !ok || obj.ObjectID() != wolf.ObjectID() {
		t.Fatal("returning owner's summon slot does not hold its pet's corpse")
	}

	buyer := h.joinSecondPlayer(t, "Buyer")
	openTrade(t, h, buyer)
	h.client.Send(encodeAddTradeItem(0, h.collarID, 1))
	assertStaticSystemMessage(t, mustRead(t, h.client, "NOTHING_HAPPENED"), serverpackets.SystemMessageNothingHappened)
	requireNoFrame(t, h.client, "trade add of the dead pet's collar")
	h.client.Send(encodeTradeDone(0))
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer.client)

	h.requireCollarRefusals(t)
	h.srv.FlushPersistence(t)
	if _, ok, err := h.srv.Pets.Get(petCtx(), h.collarID); err != nil || !ok {
		t.Fatalf("pets row for the dead pet's collar: ok=%v err=%v, want it kept", ok, err)
	}
	if !wolf.Dead() {
		t.Fatal("pet corpse revived during the refusals")
	}
}

// TestMountedCollarStaysWithItsRider mounts the wyvern and tries to drop and
// destroy its collar: the drop is refused as undiscardable and the destroy
// with PET_SUMMONED_MAY_NOT_DESTROYED, and the rider keeps it.
func TestMountedCollarStaysWithItsRider(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(discardableCollarItems())},
		seedItem{TemplateID: wyvernCollarID, Count: 1})
	wyvernCollar := h.seededItem(t, wyvernCollarID)
	h.client.Send(encodeUseItem(wyvernCollar, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")
	drainUntilQuiet(t, h.client)
	if got := h.character(t).MountObjectID(); got != wyvernCollar {
		t.Fatalf("MountObjectID() = %d, want %d", got, wyvernCollar)
	}

	x, y, z := h.character(t).Position()
	h.client.Send(encodeRequestDropItem(wyvernCollar, 1, int32(x), int32(y), int32(z)))
	assertStaticSystemMessage(t, mustRead(t, h.client, "CANNOT_DISCARD_THIS_ITEM"), serverpackets.SystemMessageCannotDiscardThisItem)
	requireNoFrame(t, h.client, "drop of the mounted collar")

	h.client.Send(encodeRequestDestroyItem(wyvernCollar, 1))
	assertStaticSystemMessage(t, mustRead(t, h.client, "PET_SUMMONED_MAY_NOT_DESTROYED"), petSummonedMayNotDestroyed)
	requireNoFrame(t, h.client, "destroy of the mounted collar")

	if h.ownerInventory(t).ItemByObjectID(wyvernCollar) == nil {
		t.Fatal("mounted collar left the rider's inventory")
	}
	if got := h.ownerItemCount(t, wyvernCollarID); got != 1 {
		t.Fatalf("rider wyvern collar rows = %d, want the collar kept", got)
	}
}

// requireCollarRefusals drops the bound collar in reach and out of it and
// destroys it: every attempt is refused with the reference's answer, no
// ground item appears, and the collar stays.
func (h *petWorld) requireCollarRefusals(t *testing.T) {
	t.Helper()
	x, y, z := h.character(t).Position()
	for _, dx := range []int32{0, 1000} {
		h.client.Send(encodeRequestDropItem(h.collarID, 1, int32(x)+dx, int32(y), int32(z)))
		assertStaticSystemMessage(t, mustRead(t, h.client, "CANNOT_DISCARD_THIS_ITEM"), serverpackets.SystemMessageCannotDiscardThisItem)
		requireNoFrame(t, h.client, "drop of the bound collar")
	}

	h.client.Send(encodeRequestDestroyItem(h.collarID, 1))
	assertStaticSystemMessage(t, mustRead(t, h.client, "PET_SUMMONED_MAY_NOT_DESTROYED"), petSummonedMayNotDestroyed)
	requireNoFrame(t, h.client, "destroy of the bound collar")

	if h.ownerInventory(t).ItemByObjectID(h.collarID) == nil {
		t.Fatal("bound collar left the owner's inventory")
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 1 {
		t.Fatalf("owner collar rows = %d, want the collar kept", got)
	}
}

// openTrade has the owner request a trade with buyer and buyer accept it.
func openTrade(t *testing.T, h *petWorld, buyer secondPlayer) {
	t.Helper()
	h.client.Send(encodeTradeRequest(buyer.id))
	drainUntilQuiet(t, buyer.client)
	buyer.client.Send(encodeAnswerTradeRequest(1))
	readUntilOpcode(t, h.client, serverpackets.OpcodeTradeStart, "owner TradeStart")
	readUntilOpcode(t, buyer.client, serverpackets.OpcodeTradeStart, "buyer TradeStart")
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, buyer.client)
}

// requireNoFrame fails if c receives anything more after a refusal.
func requireNoFrame(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("%s sent opcode %#x after its refusal, want nothing", what, frame[0])
	}
}
