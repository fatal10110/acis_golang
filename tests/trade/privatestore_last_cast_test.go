package trade

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// castPotionOut gives the first trader potions, a sword and the potion
// cast, enters both traders and runs the cast to its end. It returns the
// sword's object id.
func castPotionOut(t *testing.T, h *traders) int32 {
	t.Helper()
	potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
	sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
	if err := h.srv.KnownSkills.SetKnownSkill(context.Background(), h.firstID, 0, potionCastSkillID, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	h.enterAll(t)
	obj, ok := h.srv.State.Player(h.firstID)
	if !ok {
		t.Fatal("first trader missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("first trader %T is not an online character", obj)
	}
	h.first.Send(encodeMagicSkillUse(potionCastSkillID))
	h.srv.AdvanceUntil(t, "potion cast finished", func() bool {
		inst := pc.Inventory().ItemByObjectID(potions)
		return inst != nil && inst.CountValue() == 9 && !pc.CastingNow()
	})
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
	return sword
}

// TestSellManageListHidesLastCastConsumeItem pins the available-item
// filter's cast term on the sell store manage window
// (PcInventory.getItemsToSell through ItemInstance.isAvailable, which reads
// the last started skill after its cast ends): once the potion-consuming
// cast is over, the window offers the sword and leaves the potions out.
func TestSellManageListHidesLastCastConsumeItem(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, potionCastSkills(t))
	sword := castPotionOut(t, h)

	h.first.Send(encodeRequestActionUse(actionStoreSell))
	frame := h.first.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodePrivateStoreManageListSell, "PrivateStoreManageListSell")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // owner
	r.ReadInt32() // package flag
	r.ReadInt32() // adena
	offered := r.ReadInt32()
	r.ReadInt32() // type2
	object := r.ReadInt32()
	if err := r.Err(); err != nil || offered != 1 || object != sword {
		t.Fatalf("sell manage window offers %d items, first object %d (err %v), want only the sword %d", offered, object, err, sword)
	}
}

// TestBuyManageListHidesLastCastConsumeItem pins the same cast term on the
// buy store manage window (PcInventory.getUniqueItems through
// ItemInstance.isAvailable): after the potion-consuming cast the window
// offers the sword to want and leaves the potions out.
func TestBuyManageListHidesLastCastConsumeItem(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, potionCastSkills(t))
	castPotionOut(t, h)

	h.first.Send(encodeRequestActionUse(actionStoreBuy))
	frame := h.first.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodePrivateStoreManageListBuy, "PrivateStoreManageListBuy")
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // owner
	r.ReadInt32() // adena
	offered := r.ReadInt32()
	itemID := r.ReadInt32()
	if err := r.Err(); err != nil || offered != 1 || itemID != swordID {
		t.Fatalf("buy manage window offers %d items, first item %d (err %v), want only the sword %d", offered, itemID, err, swordID)
	}
}
