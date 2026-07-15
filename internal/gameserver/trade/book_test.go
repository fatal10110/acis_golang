package trade

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

func TestBookRequestAnswerCreatesActiveSession(t *testing.T) {
	now := time.Unix(10, 0)
	book := NewBook(func() time.Time { return now })

	if res := book.Request(1, 2); res.Status != RequestStarted {
		t.Fatalf("Request status = %v, want started", res.Status)
	}
	answer := book.Answer(2, true)
	if answer.Status != AnswerAccepted || answer.RequesterID != 1 || answer.TargetID != 2 {
		t.Fatalf("Answer = %+v, want accepted session for 1 and 2", answer)
	}
	if _, ok := book.Session(1); !ok {
		t.Fatal("first participant has no active session")
	}
	if _, ok := book.Session(2); !ok {
		t.Fatal("second participant has no active session")
	}
}

func TestBookRejectsBusyParticipants(t *testing.T) {
	book := NewBook(time.Now)
	book.Request(1, 2)

	if res := book.Request(1, 3); res.Status != RequestRequesterBusy {
		t.Fatalf("requester busy status = %v, want requester busy", res.Status)
	}
	if res := book.Request(3, 2); res.Status != RequestTargetBusy {
		t.Fatalf("target busy status = %v, want target busy", res.Status)
	}
}

func TestBookRejectsAddAfterSelfConfirm(t *testing.T) {
	book := NewBook(time.Now)
	book.Request(1, 2)
	book.Answer(2, true)
	if res := book.Confirm(1); res.Status != DoneConfirmed {
		t.Fatalf("Confirm status = %v, want confirmed", res.Status)
	}
	inv := newTradeInventory(1)
	inst := inv.AddNew(item.AdenaID, 100, 500)

	res := book.AddItem(1, inv, inst.ObjectID, 10)
	if res.Status != AddSelfConfirmed {
		t.Fatalf("Add status = %v, want self confirmed", res.Status)
	}
}

func TestBookReceiverChecksCapacityAndWeight(t *testing.T) {
	templates := tradeTemplates()
	receiver := itemcontainer.NewPlayerInventory(2, templates)
	receiver.SlotLimit = 1
	receiver.AddNew(20, 1, 700)
	weaponOffer := Offer{OwnerID: 1, Items: []Item{{Snapshot: ItemSnapshot{ObjectID: 500, TemplateID: 20, Count: 1}, Count: 1}}}

	if ReceiverFits(receiver, weaponOffer) {
		t.Fatal("ReceiverFits returned true with no free slots")
	}

	receiver.SlotLimit = 10
	receiver.WeightLimit = 10
	heavyOffer := Offer{OwnerID: 1, Items: []Item{{Snapshot: ItemSnapshot{ObjectID: 501, TemplateID: 30, Count: 2}, Count: 2}}}
	if ReceiverWeightFits(receiver, heavyOffer) {
		t.Fatal("ReceiverWeightFits returned true when added weight exceeds limit")
	}
}

func TestBookConfirmReturnsCommitSnapshot(t *testing.T) {
	book := NewBook(time.Now)
	first := newTradeInventory(1)
	second := newTradeInventory(2)
	stack := first.AddNew(item.AdenaID, 100, 500)
	first.DrainUpdates()
	second.DrainUpdates()

	book.Request(1, 2)
	book.Answer(2, true)
	if res := book.AddItem(1, first, stack.ObjectID, 40); res.Status != AddAccepted {
		t.Fatalf("Add status = %v, want accepted", res.Status)
	}
	if res := book.Confirm(1); res.Status != DoneConfirmed {
		t.Fatalf("first Confirm status = %v, want confirmed", res.Status)
	}
	res := book.Confirm(2)
	if res.Status != DoneReady {
		t.Fatalf("second Confirm status = %v, want ready", res.Status)
	}
	if res.Session.FirstID != 1 || res.Session.SecondID != 2 {
		t.Fatalf("session ids = %+v, want 1 and 2", res.Session)
	}
	offer := res.Session.Offer(1)
	if len(offer.Items) != 1 || offer.Items[0].Snapshot.ObjectID != stack.ObjectID || offer.Items[0].Count != 40 {
		t.Fatalf("offer = %+v, want 40 adena from first player", offer)
	}
	if _, ok := book.Session(1); ok {
		t.Fatal("ready session still active for first participant")
	}
}

func newTradeInventory(ownerID int32) *itemcontainer.Inventory {
	return itemcontainer.NewPlayerInventory(ownerID, tradeTemplates())
}

func tradeTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: item.AdenaID, Kind: item.KindEtcItem, Stackable: true, Tradable: true, Duration: -1, EtcItem: &item.EtcItemDetail{}},
		{ID: 20, Kind: item.KindWeapon, Slot: item.SlotRHand, Tradable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 30, Kind: item.KindWeapon, Slot: item.SlotRHand, Weight: 20, Tradable: true, Duration: -1, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
	})
}
