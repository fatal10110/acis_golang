package trade

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// RequestTimeout is how long a pending direct-trade request remains usable.
const RequestTimeout = 15 * time.Second

// RequestKind is what a pending request asks its target for. Every kind
// shares one slot per player: a player answers one request at a time, and
// waits on one it sent.
type RequestKind uint8

// Request kinds.
const (
	KindTrade RequestKind = iota
	KindParty
	KindCommandChannel
	KindPartyRoom
)

// pendingRequest is a request as its target holds it.
type pendingRequest struct {
	kind        RequestKind
	requesterID int32
	expiresAt   time.Time
	// requesterLeft marks a requester that left the world after asking. The
	// target stays held until the request expires, but the login that asked
	// is gone: a later login under the same id never asked, so an answer
	// reaches nothing of it.
	requesterLeft bool
}

// outgoingRequest is a request as its requester holds it. It expires on its
// own, so a requester stays busy for the whole timeout even when its target
// left the world before answering.
type outgoingRequest struct {
	targetID  int32
	expiresAt time.Time
}

// Book owns pending and active direct-trade sessions. mu guards every map.
type Book struct {
	mu                 sync.Mutex
	now                func() time.Time
	pendingByTarget    map[int32]pendingRequest
	pendingByRequester map[int32]outgoingRequest
	active             map[int32]*session
}

type session struct {
	firstID   int32
	secondID  int32
	offers    map[int32]*offer
	confirmed map[int32]bool
	locked    bool
	// leftID is the participant who left the world with the window still
	// open, or 0. The session stays reachable from the one who remains;
	// a second departure drops it.
	leftID int32
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
	// LeftID is the participant who left the world with the window still
	// open, or 0.
	LeftID int32
}

// NewBook returns an empty direct-trade book.
func NewBook(now func() time.Time) *Book {
	if now == nil {
		now = time.Now
	}
	return &Book{
		now:                now,
		pendingByTarget:    make(map[int32]pendingRequest),
		pendingByRequester: make(map[int32]outgoingRequest),
		active:             make(map[int32]*session),
	}
}

// Request records a pending direct-trade request.
func (b *Book) Request(requesterID, targetID int32) RequestResult {
	return b.RequestUnless(requesterID, targetID, false)
}

// RequestUnless is Request for a request a check outside the book may
// refuse, such as the target's block list. refused is that check's verdict;
// it counts only once neither side is busy, so a busy side is reported
// first, and a refused request records nothing.
func (b *Book) RequestUnless(requesterID, targetID int32, refused bool) RequestResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.purgeExpiredLocked(b.now())
	switch {
	case b.processingTransactionLocked(requesterID):
		return RequestResult{Status: RequestRequesterBusy}
	case b.processingTransactionLocked(targetID):
		return RequestResult{Status: RequestTargetBusy}
	case refused:
		return RequestResult{Status: RequestRefused}
	}

	b.recordLocked(KindTrade, requesterID, targetID)
	return RequestResult{Status: RequestStarted}
}

func (b *Book) recordLocked(kind RequestKind, requesterID, targetID int32) {
	expiresAt := b.now().Add(RequestTimeout)
	b.pendingByTarget[targetID] = pendingRequest{kind: kind, requesterID: requesterID, expiresAt: expiresAt}
	b.pendingByRequester[requesterID] = outgoingRequest{targetID: targetID, expiresAt: expiresAt}
}

// Invite records a pending request of a kind other than a trade. Unlike a
// trade request it leaves open trade sessions out: it is refused only while
// targetID, or requesterID when checkRequester is set, is processing a
// request (ProcessingRequest).
func (b *Book) Invite(kind RequestKind, requesterID, targetID int32, checkRequester bool) RequestResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.purgeExpiredLocked(b.now())
	switch {
	case checkRequester && b.processingRequestLocked(requesterID):
		return RequestResult{Status: RequestRequesterBusy}
	case b.processingRequestLocked(targetID):
		return RequestResult{Status: RequestTargetBusy}
	}
	b.recordLocked(kind, requesterID, targetID)
	return RequestResult{Status: RequestStarted}
}

// TakeInvite consumes the request of kind targetID holds and returns its
// requester. A request of another kind stays pending and reports false, as
// does one whose requester left the world after asking.
func (b *Book) TakeInvite(kind RequestKind, targetID int32) (int32, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.purgeExpiredLocked(b.now())
	pending, ok := b.pendingByTarget[targetID]
	if !ok || pending.kind != kind {
		return 0, false
	}
	delete(b.pendingByTarget, targetID)
	if out, ok := b.pendingByRequester[pending.requesterID]; ok && !pending.requesterLeft && out.targetID == targetID {
		delete(b.pendingByRequester, pending.requesterID)
	}
	if pending.requesterLeft {
		return 0, false
	}
	return pending.requesterID, true
}

// HoldsRequest reports whether playerID holds a request it has not
// answered.
func (b *Book) HoldsRequest(playerID int32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.purgeExpiredLocked(b.now())
	_, ok := b.pendingByTarget[playerID]
	return ok
}

// Answer accepts or rejects a pending direct-trade request.
//
// A request past its timeout is no request at all: accepted or refused, the
// answer finds nothing (AnswerMissing), so the requester hears no denial and
// the answering side is told its target is gone.
//
// Accepting a request whose requester has left opens the session on the
// target's side only, with the requester already marked as left: the target
// trades against the login that asked, which is gone, and a later login under
// the same id can neither reach the session nor be reached by it.
func (b *Book) Answer(targetID int32, accept bool) AnswerResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	pending, ok := b.pendingByTarget[targetID]
	if ok && pending.kind != KindTrade {
		// Another kind of request is not this answer's to consume.
		return AnswerResult{Status: AnswerMissing, TargetID: targetID}
	}
	delete(b.pendingByTarget, targetID)
	if !ok || !now.Before(pending.expiresAt) {
		return AnswerResult{Status: AnswerMissing, TargetID: targetID}
	}
	if out, ok := b.pendingByRequester[pending.requesterID]; ok && !pending.requesterLeft && out.targetID == targetID {
		delete(b.pendingByRequester, pending.requesterID)
	}
	result := AnswerResult{RequesterID: pending.requesterID, TargetID: targetID, RequesterLeft: pending.requesterLeft}
	if !accept {
		result.Status = AnswerDenied
		return result
	}

	s := newSession(pending.requesterID, targetID)
	if pending.requesterLeft {
		s.leftID = pending.requesterID
	} else {
		b.active[pending.requesterID] = s
	}
	b.active[targetID] = s
	result.Status = AnswerAccepted
	return result
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

// ProcessingTransaction reports whether playerID is tied up in a direct
// trade: an open session, or an unexpired request it sent or received.
func (b *Book) ProcessingTransaction(playerID int32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.purgeExpiredLocked(b.now())
	return b.processingTransactionLocked(playerID)
}

// ProcessingRequest reports whether playerID has an unexpired trade
// request it sent or received, leaving an open session out.
func (b *Book) ProcessingRequest(playerID int32) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.purgeExpiredLocked(b.now())
	return b.processingRequestLocked(playerID)
}

func (b *Book) processingRequestLocked(playerID int32) bool {
	if _, ok := b.pendingByTarget[playerID]; ok {
		return true
	}
	_, ok := b.pendingByRequester[playerID]
	return ok
}

// BoundItems tells which of a participant's items are bound where they are
// and may not change hands whatever their own state: the control item of a
// pet that is out, or the enchant scroll the participant has selected. It is
// asked again at settlement. A nil BoundItems binds nothing.
type BoundItems func(objectID int32) bool

// AddItem adds an item to a player's active direct-trade offer. bound is
// that player's.
func (b *Book) AddItem(playerID int32, inv *itemcontainer.Inventory, bound BoundItems, objectID int32, count int) AddResult {
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

	if inv == nil {
		return AddResult{Status: AddInvalidItem, PartnerID: partnerID}
	}
	inst, ok := itemForOffer(inv, bound, playerID, objectID, count)
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
		AvailableCount: inst.Snapshot().Count - added.count,
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
	if s.leftID == partnerID {
		return DoneResult{Status: DonePartnerLeft, PartnerID: partnerID}
	}
	s.confirmed[playerID] = true
	if !s.confirmed[partnerID] {
		return DoneResult{Status: DoneConfirmed, PartnerID: partnerID}
	}

	b.closeLocked(s)
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
	b.closeLocked(s)
	return CancelResult{Status: CancelDone, Session: s.snapshot()}
}

// Leave takes playerID, who is leaving the world, out of its open session
// without closing it: the partner's window stays open and learns of the
// departure only on its own next trade action. The departed side can no
// longer reach the session, so a later login under the same id starts free
// of it. When the partner has already left too, the session is dropped.
//
// A request still waiting for an answer goes the same way. One playerID
// received is dropped, so a later login has nothing to answer, while its
// requester stays busy until the request would have expired. One playerID
// sent no longer holds playerID, so a later login is free to trade at once,
// while its target stays held until expiry and an answer opens nothing on
// that later login (Answer).
func (b *Book) Leave(playerID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.pendingByTarget, playerID)
	if out, ok := b.pendingByRequester[playerID]; ok {
		delete(b.pendingByRequester, playerID)
		if pending, ok := b.pendingByTarget[out.targetID]; ok && pending.requesterID == playerID {
			pending.requesterLeft = true
			b.pendingByTarget[out.targetID] = pending
		}
	}

	s := b.active[playerID]
	if s == nil || s.locked {
		return
	}
	delete(b.active, playerID)
	if s.leftID != 0 {
		s.locked = true
		return
	}
	s.leftID = playerID
}

// closeLocked locks s and drops the book entries that still point at it. A
// participant who left may already hold a newer session under the same id,
// which must survive.
func (b *Book) closeLocked(s *session) {
	s.locked = true
	for _, id := range []int32{s.firstID, s.secondID} {
		if b.active[id] == s {
			delete(b.active, id)
		}
	}
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

// PartnerLeft reports whether playerID's partner left the world with the
// window still open.
func (s Session) PartnerLeft(playerID int32) bool {
	partnerID, ok := s.PartnerID(playerID)
	return ok && s.LeftID == partnerID
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

// Holdings is what settling a trade reads from one participant's inventory:
// an *itemcontainer.Inventory, or the itemcontainer.Held view of one the
// settling exchange has locked.
type Holdings interface {
	ItemByObjectID(objectID int32) *item.Instance
	ItemByTemplateID(templateID int32) *item.Instance
	Templates() *item.Table
	ValidateCapacity(slotCount int) bool
	ValidateWeight(weight int) bool
}

// Check re-validates a ready session against both participants' inventories
// and bound items as they stand when it settles: every offered item still
// tradeable, then the partner's offer within each receiver's weight and
// slots.
func (s Session) Check(first, second Holdings, firstBound, secondBound BoundItems) SettlementStatus {
	if !ValidOfferItems(first, firstBound, s.FirstID, s.FirstOffer) || !ValidOfferItems(second, secondBound, s.SecondID, s.SecondOffer) {
		return SettlementInvalidItems
	}
	switch s.ReceiverStatus(first, second) {
	case ReceiverWeightExceeded:
		return SettlementWeightExceeded
	case ReceiverSlotsFull:
		return SettlementSlotsFull
	}
	return SettlementOK
}

// ReceiverStatus reports whether both participants can receive their partner's offer.
func (s Session) ReceiverStatus(firstInv, secondInv Holdings) ReceiverStatus {
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
				available = inst.Snapshot().Count - row.Count
			}
		}
		entries = append(entries, ItemUpdateEntry{Item: row.Snapshot, AvailableCount: available})
	}
	return entries
}

// ValidOfferItems reports whether every item in offer is still tradeable in
// inv and none of them is bound.
func ValidOfferItems(inv Holdings, bound BoundItems, ownerID int32, offer Offer) bool {
	for _, row := range offer.Items {
		if _, ok := itemForOffer(inv, bound, ownerID, row.Snapshot.ObjectID, row.Count); !ok {
			return false
		}
	}
	return true
}

// ReceiverFits reports whether receiver has enough slots for offer.
func ReceiverFits(receiver Holdings, offer Offer) bool {
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
func ReceiverWeightFits(receiver Holdings, offer Offer) bool {
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
		LeftID:      s.leftID,
	}
}

func (o *offer) add(inst *item.Instance, count int) (*offeredItem, bool) {
	if inst == nil || count <= 0 {
		return nil, false
	}
	if existing := o.items[inst.ObjectID]; existing != nil {
		if existing.count+count > inst.Snapshot().Count {
			return nil, false
		}
		existing.count += count
		existing.snapshot.Count = existing.count
		return existing, true
	}
	st := inst.Snapshot()
	if count > st.Count {
		return nil, false
	}
	row := &offeredItem{
		snapshot: ItemSnapshot{
			ObjectID:     st.ObjectID,
			TemplateID:   st.TemplateID,
			Count:        count,
			EnchantLevel: st.EnchantLevel,
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

// purgeExpiredLocked drops expired requests from both sides. Each side
// expires on its own: a request either participant left behind is held on
// one side only.
//
// A request held by a target with a trade window open outlives its expiry
// on the target's side until that window closes: the target can still
// answer it, and is still processing it.
func (b *Book) purgeExpiredLocked(now time.Time) {
	for targetID, pending := range b.pendingByTarget {
		if s := b.active[targetID]; s != nil && !s.locked {
			continue
		}
		if !now.Before(pending.expiresAt) {
			delete(b.pendingByTarget, targetID)
		}
	}
	for requesterID, out := range b.pendingByRequester {
		if !now.Before(out.expiresAt) {
			delete(b.pendingByRequester, requesterID)
		}
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

func itemForOffer(inv Holdings, bound BoundItems, ownerID, objectID int32, count int) (*item.Instance, bool) {
	if count <= 0 {
		return nil, false
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return nil, false
	}
	st := inst.Snapshot()
	if st.OwnerID != ownerID || st.Equipped() || st.Count < count {
		return nil, false
	}
	if bound != nil && bound(objectID) {
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
