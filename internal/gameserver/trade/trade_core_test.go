package trade

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// ---- from book_test.go ----
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

	res := book.AddItem(1, inv, nil, inst.ObjectID, 10)
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
	if res := book.AddItem(1, first, nil, stack.ObjectID, 40); res.Status != AddAccepted {
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

func openBookSession(t *testing.T, book *Book, requesterID, targetID int32) {
	t.Helper()
	if res := book.Request(requesterID, targetID); res.Status != RequestStarted {
		t.Fatalf("Request(%d, %d) status = %v, want started", requesterID, targetID, res.Status)
	}
	if res := book.Answer(targetID, true); res.Status != AnswerAccepted {
		t.Fatalf("Answer(%d) status = %v, want accepted", targetID, res.Status)
	}
}

func TestBookConfirmAfterOwnConfirmIgnoresPartnerLeave(t *testing.T) {
	book := NewBook(time.Now)
	openBookSession(t, book, 1, 2)
	if res := book.Confirm(1); res.Status != DoneConfirmed {
		t.Fatalf("first Confirm status = %v, want confirmed", res.Status)
	}
	book.Leave(2)

	// A repeat confirm is silent before any partner check runs.
	if res := book.Confirm(1); res.Status != DoneAlreadyConfirmed || res.PartnerID != 2 {
		t.Fatalf("repeat Confirm = %+v, want already confirmed with partner 2", res)
	}
	if !book.HasActive(1) {
		t.Fatal("remaining trader lost its session on a repeat confirm")
	}
}

func TestBookConfirmAfterPartnerLeaveReportsPartnerLeft(t *testing.T) {
	book := NewBook(time.Now)
	openBookSession(t, book, 1, 2)
	book.Leave(2)

	sess, ok := book.Session(1)
	if !ok || !sess.PartnerLeft(1) || sess.LeftID != 2 {
		t.Fatalf("Session(1) = %+v, %v; want open session with partner 2 gone", sess, ok)
	}
	if book.HasActive(2) || book.ProcessingTransaction(2) {
		t.Fatal("departed trader still reaches the session")
	}

	if res := book.Confirm(1); res.Status != DonePartnerLeft || res.PartnerID != 2 {
		t.Fatalf("Confirm = %+v, want partner left with partner 2", res)
	}
	// Confirm must not record a confirmation that could pair with the
	// departed side; the session stays for the caller to cancel.
	if res := book.Confirm(1); res.Status != DonePartnerLeft {
		t.Fatalf("second Confirm status = %v, want partner left", res.Status)
	}
	if res := book.Cancel(1); res.Status != CancelDone {
		t.Fatalf("Cancel status = %v, want done", res.Status)
	}
	if book.HasActive(1) || book.ProcessingTransaction(1) {
		t.Fatal("remaining trader still busy after cancel")
	}
}

func TestBookBothLeaveDropsSession(t *testing.T) {
	book := NewBook(time.Now)
	openBookSession(t, book, 1, 2)
	book.Leave(1)
	book.Leave(2)

	for _, id := range []int32{1, 2} {
		if book.HasActive(id) || book.ProcessingTransaction(id) {
			t.Fatalf("player %d still tied to a trade after both left", id)
		}
		if res := book.Confirm(id); res.Status != DoneNoSession {
			t.Fatalf("Confirm(%d) status = %v, want no session", id, res.Status)
		}
		if res := book.Cancel(id); res.Status != CancelMissing {
			t.Fatalf("Cancel(%d) status = %v, want missing", id, res.Status)
		}
	}
	if res := book.Request(1, 2); res.Status != RequestStarted {
		t.Fatalf("new Request status = %v, want started", res.Status)
	}
}

func TestBookCloseKeepsNewerSessionOfDepartedTrader(t *testing.T) {
	book := NewBook(time.Now)
	openBookSession(t, book, 1, 2)
	book.Leave(2)
	openBookSession(t, book, 2, 3)

	if res := book.Cancel(1); res.Status != CancelDone || res.Session.SecondID != 2 {
		t.Fatalf("Cancel(1) = %+v, want the old session with 2 closed", res)
	}
	sess, ok := book.Session(2)
	if !ok || sess.FirstID != 2 || sess.SecondID != 3 {
		t.Fatalf("Session(2) = %+v, %v; want the newer session with 3", sess, ok)
	}
	if !book.HasActive(3) {
		t.Fatal("newer session partner lost its session")
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
