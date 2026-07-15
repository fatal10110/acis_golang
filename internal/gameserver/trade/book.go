package trade

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// RequestTimeout is how long a pending direct-trade request remains usable.
const RequestTimeout = 15 * time.Second

type pendingRequest struct {
	requesterID int32
	expiresAt   time.Time
}

// Book owns pending and active direct-trade sessions. mu guards every map.
type Book struct {
	mu                 sync.Mutex
	now                func() time.Time
	pendingByTarget    map[int32]pendingRequest
	pendingByRequester map[int32]int32
	active             map[int32]*session
}

type session struct {
	firstID   int32
	secondID  int32
	offers    map[int32]*offer
	confirmed map[int32]bool
	locked    bool
}

type offer struct {
	ownerID int32
	items   map[int32]*offeredItem
	order   []int32
}

type offeredItem struct {
	snapshot ItemSnapshot
	count    int
}

// ItemSnapshot is one item row shown in a direct-trade offer.
type ItemSnapshot struct {
	ObjectID     int32
	TemplateID   int32
	Count        int
	EnchantLevel int
}

// ItemUpdateEntry is one row in an offer availability refresh.
type ItemUpdateEntry struct {
	Item           ItemSnapshot
	AvailableCount int
}

// Item is one offered trade item.
type Item struct {
	Snapshot ItemSnapshot
	Count    int
}

// Offer is a copy of one participant's offered items.
type Offer struct {
	OwnerID int32
	Items   []Item
}

// Session is a copy of an active direct-trade session.
type Session struct {
	FirstID     int32
	SecondID    int32
	FirstOffer  Offer
	SecondOffer Offer
}

// NewBook returns an empty direct-trade book.
func NewBook(now func() time.Time) *Book {
	if now == nil {
		now = time.Now
	}
	return &Book{
		now:                now,
		pendingByTarget:    make(map[int32]pendingRequest),
		pendingByRequester: make(map[int32]int32),
		active:             make(map[int32]*session),
	}
}

// Request records a pending direct-trade request.
func (b *Book) Request(requesterID, targetID int32) RequestResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.purgeExpiredLocked(b.now())
	switch {
	case b.processingTransactionLocked(requesterID):
		return RequestResult{Status: RequestRequesterBusy}
	case b.processingTransactionLocked(targetID):
		return RequestResult{Status: RequestTargetBusy}
	}

	b.pendingByTarget[targetID] = pendingRequest{requesterID: requesterID, expiresAt: b.now().Add(RequestTimeout)}
	b.pendingByRequester[requesterID] = targetID
	return RequestResult{Status: RequestStarted}
}

// Answer accepts or rejects a pending direct-trade request.
func (b *Book) Answer(targetID int32, accept bool) AnswerResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	pending, ok := b.pendingByTarget[targetID]
	if ok {
		delete(b.pendingByTarget, targetID)
		delete(b.pendingByRequester, pending.requesterID)
	}
	if !ok {
		return AnswerResult{Status: AnswerMissing, TargetID: targetID}
	}
	if !accept || !now.Before(pending.expiresAt) {
		return AnswerResult{Status: AnswerDenied, RequesterID: pending.requesterID, TargetID: targetID}
	}

	s := newSession(pending.requesterID, targetID)
	b.active[pending.requesterID] = s
	b.active[targetID] = s
	return AnswerResult{Status: AnswerAccepted, RequesterID: pending.requesterID, TargetID: targetID}
}

// Session returns a snapshot of the player's active direct-trade session.
func (b *Book) Session(playerID int32) (Session, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := b.active[playerID]
	if s == nil || s.locked {
		return Session{}, false
	}
	return s.snapshot(), true
}

// HasActive reports whether playerID is in an active direct-trade session.
func (b *Book) HasActive(playerID int32) bool {
	_, ok := b.Session(playerID)
	return ok
}

// AddItem adds an item to a player's active direct-trade offer.
func (b *Book) AddItem(playerID int32, inv *itemcontainer.Inventory, objectID int32, count int) AddResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := b.active[playerID]
	if s == nil || s.locked {
		return AddResult{Status: AddNoSession}
	}
	partnerID, _ := s.partnerID(playerID)
	if s.confirmed[playerID] {
		return AddResult{Status: AddSelfConfirmed, PartnerID: partnerID}
	}
	if s.confirmed[partnerID] {
		return AddResult{Status: AddPartnerConfirmed, PartnerID: partnerID}
	}

	inst, ok := itemForOffer(inv, playerID, objectID, count)
	if !ok {
		return AddResult{Status: AddInvalidItem, PartnerID: partnerID}
	}
	added, ok := s.offers[playerID].add(inst, count)
	if !ok {
		return AddResult{Status: AddNoSession, PartnerID: partnerID}
	}

	return AddResult{
		Status:         AddAccepted,
		PartnerID:      partnerID,
		Item:           added.snapshot,
		AddedCount:     count,
		AvailableCount: inst.Count - added.count,
		Entries:        s.offers[playerID].entries(inv),
	}
}

// Confirm records a player's confirmation and returns a session snapshot when both sides confirmed.
func (b *Book) Confirm(playerID int32) DoneResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := b.active[playerID]
	if s == nil || s.locked {
		return DoneResult{Status: DoneNoSession}
	}
	partnerID, ok := s.partnerID(playerID)
	if !ok {
		return DoneResult{Status: DoneNoSession}
	}
	if s.confirmed[playerID] {
		return DoneResult{Status: DoneAlreadyConfirmed, PartnerID: partnerID}
	}
	s.confirmed[playerID] = true
	if !s.confirmed[partnerID] {
		return DoneResult{Status: DoneConfirmed, PartnerID: partnerID}
	}

	s.locked = true
	delete(b.active, s.firstID)
	delete(b.active, s.secondID)
	return DoneResult{Status: DoneReady, PartnerID: partnerID, Session: s.snapshot()}
}

// Cancel removes a player's active direct-trade session.
func (b *Book) Cancel(playerID int32) CancelResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	s := b.active[playerID]
	if s == nil || s.locked {
		return CancelResult{Status: CancelMissing}
	}
	s.locked = true
	delete(b.active, s.firstID)
	delete(b.active, s.secondID)
	return CancelResult{Status: CancelDone, Session: s.snapshot()}
}

// PartnerID returns the active direct-trade partner for playerID.
func (s Session) PartnerID(playerID int32) (int32, bool) {
	switch playerID {
	case s.FirstID:
		return s.SecondID, true
	case s.SecondID:
		return s.FirstID, true
	default:
		return 0, false
	}
}

// Offer returns playerID's offer from the session.
func (s Session) Offer(playerID int32) Offer {
	switch playerID {
	case s.FirstID:
		return s.FirstOffer
	case s.SecondID:
		return s.SecondOffer
	default:
		return Offer{}
	}
}

// Empty reports whether neither participant offered an item.
func (s Session) Empty() bool {
	return s.FirstOffer.Empty() && s.SecondOffer.Empty()
}

// ValidItems reports whether all offered items are still tradeable in their source inventories.
func (s Session) ValidItems(firstInv, secondInv *itemcontainer.Inventory) bool {
	return ValidOfferItems(firstInv, s.FirstID, s.FirstOffer) && ValidOfferItems(secondInv, s.SecondID, s.SecondOffer)
}

// ReceiverStatus reports whether both participants can receive their partner's offer.
func (s Session) ReceiverStatus(firstInv, secondInv *itemcontainer.Inventory) ReceiverStatus {
	if !ReceiverWeightFits(firstInv, s.SecondOffer) || !ReceiverWeightFits(secondInv, s.FirstOffer) {
		return ReceiverWeightExceeded
	}
	if !ReceiverFits(firstInv, s.SecondOffer) || !ReceiverFits(secondInv, s.FirstOffer) {
		return ReceiverSlotsFull
	}
	return ReceiverOK
}

// Empty reports whether the offer has no items.
func (o Offer) Empty() bool {
	return len(o.Items) == 0
}

// Entries returns availability rows for the offer against its source inventory.
func (o Offer) Entries(inv *itemcontainer.Inventory) []ItemUpdateEntry {
	entries := make([]ItemUpdateEntry, 0, len(o.Items))
	for _, row := range o.Items {
		available := 0
		if inv != nil {
			if inst := inv.ItemByObjectID(row.Snapshot.ObjectID); inst != nil {
				available = inst.Count - row.Count
			}
		}
		entries = append(entries, ItemUpdateEntry{Item: row.Snapshot, AvailableCount: available})
	}
	return entries
}

// ValidOfferItems reports whether every item in offer is still tradeable in inv.
func ValidOfferItems(inv *itemcontainer.Inventory, ownerID int32, offer Offer) bool {
	for _, row := range offer.Items {
		if _, ok := itemForOffer(inv, ownerID, row.Snapshot.ObjectID, row.Count); !ok {
			return false
		}
	}
	return true
}

// ReceiverFits reports whether receiver has enough slots for offer.
func ReceiverFits(receiver *itemcontainer.Inventory, offer Offer) bool {
	if receiver == nil {
		return false
	}
	slots := 0
	seenStack := make(map[int32]bool)
	for _, row := range offer.Items {
		tmpl, ok := receiver.Templates().Get(row.Snapshot.TemplateID)
		if !ok {
			return false
		}
		if tmpl.Stackable {
			if receiver.ItemByTemplateID(row.Snapshot.TemplateID) != nil || seenStack[row.Snapshot.TemplateID] {
				continue
			}
			seenStack[row.Snapshot.TemplateID] = true
		}
		slots++
	}
	return receiver.ValidateCapacity(slots)
}

// ReceiverWeightFits reports whether receiver can carry offer's weight.
func ReceiverWeightFits(receiver *itemcontainer.Inventory, offer Offer) bool {
	if receiver == nil {
		return false
	}
	weight := 0
	for _, row := range offer.Items {
		tmpl, ok := receiver.Templates().Get(row.Snapshot.TemplateID)
		if !ok {
			return false
		}
		weight += int(tmpl.Weight) * row.Count
	}
	return receiver.ValidateWeight(weight)
}

func newSession(firstID, secondID int32) *session {
	return &session{
		firstID:   firstID,
		secondID:  secondID,
		offers:    map[int32]*offer{firstID: newOffer(firstID), secondID: newOffer(secondID)},
		confirmed: make(map[int32]bool, 2),
	}
}

func newOffer(ownerID int32) *offer {
	return &offer{ownerID: ownerID, items: make(map[int32]*offeredItem)}
}

func (s *session) partnerID(playerID int32) (int32, bool) {
	switch playerID {
	case s.firstID:
		return s.secondID, true
	case s.secondID:
		return s.firstID, true
	default:
		return 0, false
	}
}

func (s *session) snapshot() Session {
	return Session{
		FirstID:     s.firstID,
		SecondID:    s.secondID,
		FirstOffer:  s.offers[s.firstID].snapshot(),
		SecondOffer: s.offers[s.secondID].snapshot(),
	}
}

func (o *offer) add(inst *item.Instance, count int) (*offeredItem, bool) {
	if inst == nil || count <= 0 {
		return nil, false
	}
	if existing := o.items[inst.ObjectID]; existing != nil {
		if existing.count+count > inst.Count {
			return nil, false
		}
		existing.count += count
		existing.snapshot.Count = existing.count
		return existing, true
	}
	if count > inst.Count {
		return nil, false
	}
	row := &offeredItem{
		snapshot: ItemSnapshot{
			ObjectID:     inst.ObjectID,
			TemplateID:   inst.TemplateID,
			Count:        count,
			EnchantLevel: inst.EnchantLevel,
		},
		count: count,
	}
	o.items[inst.ObjectID] = row
	o.order = append(o.order, inst.ObjectID)
	return row, true
}

func (o *offer) entries(inv *itemcontainer.Inventory) []ItemUpdateEntry {
	return o.snapshot().Entries(inv)
}

func (o *offer) snapshot() Offer {
	items := make([]Item, 0, len(o.order))
	for _, objectID := range o.order {
		row := o.items[objectID]
		if row == nil {
			continue
		}
		items = append(items, Item{Snapshot: row.snapshot, Count: row.count})
	}
	return Offer{OwnerID: o.ownerID, Items: items}
}

func (b *Book) purgeExpiredLocked(now time.Time) {
	for targetID, pending := range b.pendingByTarget {
		if now.Before(pending.expiresAt) {
			continue
		}
		delete(b.pendingByTarget, targetID)
		delete(b.pendingByRequester, pending.requesterID)
	}
}

func (b *Book) processingTransactionLocked(objectID int32) bool {
	if b.active[objectID] != nil {
		return true
	}
	if _, ok := b.pendingByTarget[objectID]; ok {
		return true
	}
	_, ok := b.pendingByRequester[objectID]
	return ok
}

func itemForOffer(inv *itemcontainer.Inventory, ownerID, objectID int32, count int) (*item.Instance, bool) {
	if inv == nil || count <= 0 {
		return nil, false
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil || inst.OwnerID != ownerID || inst.Equipped() || inst.Count < count {
		return nil, false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Tradable(tmpl) || inst.QuestItem(tmpl) {
		return nil, false
	}
	if !tmpl.Stackable && count > 1 {
		return nil, false
	}
	return inst, true
}
