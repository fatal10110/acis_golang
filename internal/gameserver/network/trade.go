package network

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

const (
	tradeRequestTimeout      = 15 * time.Second
	tradeInteractionDistance = 150
)

type tradeCoordinator struct {
	mu                 sync.Mutex
	now                func() time.Time
	pendingByTarget    map[int32]pendingTradeRequest
	pendingByRequester map[int32]int32
	active             map[int32]*tradeSession
}

type pendingTradeRequest struct {
	requesterID int32
	expiresAt   time.Time
}

type tradeSession struct {
	first     *livePlayer
	second    *livePlayer
	offers    map[int32]*tradeOffer
	confirmed map[int32]bool
	locked    bool
}

type tradeOffer struct {
	owner *livePlayer
	items map[int32]*tradeItem
	order []int32
}

type tradeItem struct {
	snapshot serverpackets.TradeItemSnapshot
	count    int
}

func newTradeCoordinator(now func() time.Time) *tradeCoordinator {
	if now == nil {
		now = time.Now
	}
	return &tradeCoordinator{
		now:                now,
		pendingByTarget:    make(map[int32]pendingTradeRequest),
		pendingByRequester: make(map[int32]int32),
		active:             make(map[int32]*tradeSession),
	}
}

func (l *GameClientLink) tradeCoordinator() *tradeCoordinator {
	if l.trades == nil {
		l.trades = newTradeCoordinator(time.Now)
	}
	return l.trades
}

func (l *GameClientLink) handleTradeRequest(live *livePlayer, req clientpackets.TradeRequest) {
	if live == nil || l.world == nil {
		return
	}
	target, ok := l.livePlayerByID(req.ObjectID)
	if !ok {
		return
	}
	if target.ObjectID() == live.ObjectID() || !world.Knows(live, target) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
		return
	}

	trades := l.tradeCoordinator()
	trades.mu.Lock()
	trades.purgeExpiredLocked(trades.now())
	switch {
	case trades.processingTransactionLocked(live.ObjectID()):
		trades.mu.Unlock()
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAlreadyTrading))
		return
	case trades.processingTransactionLocked(target.ObjectID()):
		trades.mu.Unlock()
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1IsBusyTryLater, target.Name))
		return
	}
	trades.pendingByTarget[target.ObjectID()] = pendingTradeRequest{
		requesterID: live.ObjectID(),
		expiresAt:   trades.now().Add(tradeRequestTimeout),
	}
	trades.pendingByRequester[live.ObjectID()] = target.ObjectID()
	trades.mu.Unlock()

	target.SendFrame(serverpackets.FrameSendTradeRequest(live.ObjectID()))
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageRequestS1ForTrade, target.Name))
}

func (l *GameClientLink) handleAnswerTradeRequest(live *livePlayer, req clientpackets.AnswerTradeRequest) {
	if live == nil {
		return
	}
	trades := l.tradeCoordinator()
	trades.mu.Lock()
	now := trades.now()
	pending, ok := trades.pendingByTarget[live.ObjectID()]
	if ok {
		delete(trades.pendingByTarget, live.ObjectID())
		delete(trades.pendingByRequester, pending.requesterID)
	}
	trades.mu.Unlock()

	requester, requesterOnline := l.livePlayerByID(pending.requesterID)
	if !ok || !requesterOnline {
		live.SendFrame(serverpackets.FrameSendTradeDone(false))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	if req.Response != 1 || !now.Before(pending.expiresAt) {
		requester.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1DeniedTradeRequest, live.Name))
		return
	}
	if !l.validTradeParticipants(requester, live) {
		live.SendFrame(serverpackets.FrameSendTradeDone(false))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}

	session := newTradeSession(requester, live)
	trades.mu.Lock()
	trades.active[requester.ObjectID()] = session
	trades.active[live.ObjectID()] = session
	trades.mu.Unlock()

	if !l.sendTradeStart(requester, live) || !l.sendTradeStart(live, requester) {
		l.cancelTrade(session, requester)
	}
}

func (l *GameClientLink) handleAddTradeItem(live *livePlayer, req clientpackets.AddTradeItem) {
	if live == nil || req.Count <= 0 {
		return
	}
	trades := l.tradeCoordinator()
	trades.mu.Lock()
	session := trades.active[live.ObjectID()]
	if session == nil || session.locked {
		trades.mu.Unlock()
		return
	}
	partner := session.partner(live)
	offer := session.offers[live.ObjectID()]
	switch {
	case partner == nil || session.offers[partner.ObjectID()] == nil || !l.validTradeParticipants(live, partner):
		trades.mu.Unlock()
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		l.cancelTrade(session, live)
		return
	case session.confirmed[live.ObjectID()]:
		trades.mu.Unlock()
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnceTradeConfirmedCannotMove))
		return
	case session.confirmed[partner.ObjectID()]:
		trades.mu.Unlock()
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotAdjustItemsAfterConfirm))
		return
	}

	inst, _, ok := tradeItemForOffer(live, req.ObjectID, int(req.Count))
	if !ok {
		trades.mu.Unlock()
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNothingHappened))
		return
	}
	tradeItem, ok := offer.add(inst, int(req.Count))
	if !ok {
		trades.mu.Unlock()
		return
	}
	available := inst.Count - tradeItem.count
	entries := offer.entries(live.Inventory())
	trades.mu.Unlock()

	if frame, err := serverpackets.FrameTradeOwnAdd(tradeItem.snapshot, int(req.Count), live.Inventory().Templates()); err == nil {
		live.SendFrame(frame)
	} else {
		l.log.Error().Err(err).Msg("build TradeOwnAdd")
		return
	}
	if frame, err := serverpackets.FrameTradeUpdate(tradeItem.snapshot, available, live.Inventory().Templates()); err == nil {
		live.SendFrame(frame)
	} else {
		l.log.Error().Err(err).Msg("build TradeUpdate")
		return
	}
	if frame, err := serverpackets.FrameTradeItemUpdate(entries, live.Inventory().Templates()); err == nil {
		live.SendFrame(frame)
	} else {
		l.log.Error().Err(err).Msg("build TradeItemUpdate")
		return
	}
	if frame, err := serverpackets.FrameTradeOtherAdd(tradeItem.snapshot, int(req.Count), partner.Inventory().Templates()); err == nil {
		partner.SendFrame(frame)
	} else {
		l.log.Error().Err(err).Msg("build TradeOtherAdd")
	}
}

func (l *GameClientLink) handleTradeDone(ctx context.Context, live *livePlayer, req clientpackets.TradeDone) {
	if live == nil {
		return
	}
	trades := l.tradeCoordinator()
	trades.mu.Lock()
	session := trades.active[live.ObjectID()]
	if session == nil || session.locked {
		trades.mu.Unlock()
		return
	}
	if req.Response != 1 {
		trades.mu.Unlock()
		l.cancelTrade(session, live)
		return
	}
	partner := session.partner(live)
	if partner == nil || !l.validTradeParticipants(live, partner) {
		trades.mu.Unlock()
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetNotFound))
		return
	}
	if session.confirmed[live.ObjectID()] {
		trades.mu.Unlock()
		return
	}

	if !l.validateTradeSession(session, false) {
		trades.mu.Unlock()
		l.cancelTrade(session, live)
		return
	}
	session.confirmed[live.ObjectID()] = true
	if !session.confirmed[partner.ObjectID()] {
		trades.mu.Unlock()
		l.sendTradeConfirmed(live, partner)
		return
	}

	session.locked = true
	delete(trades.active, session.first.ObjectID())
	delete(trades.active, session.second.ObjectID())
	trades.mu.Unlock()

	if !l.validateTradeSession(session, true) {
		l.finishTrade(session, false)
		return
	}
	ok, failMessage := l.exchangeTradeItems(ctx, session)
	if failMessage != 0 {
		session.first.SendFrame(serverpackets.FrameSystemMessage(failMessage))
		session.second.SendFrame(serverpackets.FrameSystemMessage(failMessage))
	}
	if ok {
		l.sendInventoryUpdate(session.first, session.first.Inventory())
		l.sendInventoryUpdate(session.second, session.second.Inventory())
	}
	l.finishTrade(session, ok)
}

func (l *GameClientLink) cancelActiveTrade(live *livePlayer) {
	if live == nil || l.trades == nil {
		return
	}
	l.trades.mu.Lock()
	session := l.trades.active[live.ObjectID()]
	l.trades.mu.Unlock()
	if session != nil {
		l.cancelTrade(session, live)
	}
}

func (l *GameClientLink) cancelTrade(session *tradeSession, _ *livePlayer) {
	if session == nil {
		return
	}
	trades := l.tradeCoordinator()
	trades.mu.Lock()
	if session.locked {
		trades.mu.Unlock()
		return
	}
	session.locked = true
	delete(trades.active, session.first.ObjectID())
	delete(trades.active, session.second.ObjectID())
	trades.mu.Unlock()

	firstName, secondName := session.first.Name, session.second.Name
	session.first.SendFrame(serverpackets.FrameSendTradeDone(false))
	session.first.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1CanceledTrade, secondName))
	session.second.SendFrame(serverpackets.FrameSendTradeDone(false))
	session.second.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1CanceledTrade, firstName))
}

func (l *GameClientLink) finishTrade(session *tradeSession, success bool) {
	for _, live := range []*livePlayer{session.first, session.second} {
		live.SendFrame(serverpackets.FrameSendTradeDone(success))
		if success {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTradeSuccessful))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExchangeHasEnded))
		}
	}
}

func (l *GameClientLink) sendTradeStart(live, partner *livePlayer) bool {
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageBeginTradeWithS1, partner.Name))
	frame, err := serverpackets.FrameTradeStart(partner.ObjectID(), live.Inventory().Items(), live.Inventory().Templates())
	if err != nil {
		l.log.Error().Err(err).Msg("build TradeStart")
		return false
	}
	live.SendFrame(frame)
	return true
}

func (l *GameClientLink) sendTradeConfirmed(confirmer, partner *livePlayer) {
	partner.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1ConfirmedTrade, confirmer.Name))
	confirmer.SendFrame(serverpackets.FrameTradePressOwnOk())
	partner.SendFrame(serverpackets.FrameTradePressOtherOk())
}

func newTradeSession(first, second *livePlayer) *tradeSession {
	return &tradeSession{
		first:     first,
		second:    second,
		offers:    map[int32]*tradeOffer{first.ObjectID(): newTradeOffer(first), second.ObjectID(): newTradeOffer(second)},
		confirmed: make(map[int32]bool, 2),
	}
}

func newTradeOffer(owner *livePlayer) *tradeOffer {
	return &tradeOffer{owner: owner, items: make(map[int32]*tradeItem)}
}

func (s *tradeSession) partner(live *livePlayer) *livePlayer {
	if live == nil {
		return nil
	}
	switch live.ObjectID() {
	case s.first.ObjectID():
		return s.second
	case s.second.ObjectID():
		return s.first
	default:
		return nil
	}
}

func (o *tradeOffer) add(inst *item.Instance, count int) (*tradeItem, bool) {
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
	row := &tradeItem{
		snapshot: serverpackets.TradeItemSnapshot{
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

func (o *tradeOffer) entries(inv *itemcontainer.Inventory) []serverpackets.TradeItemUpdateEntry {
	entries := make([]serverpackets.TradeItemUpdateEntry, 0, len(o.order))
	for _, objectID := range o.order {
		row := o.items[objectID]
		if row == nil {
			continue
		}
		available := 0
		if inv != nil {
			if inst := inv.ItemByObjectID(objectID); inst != nil {
				available = inst.Count - row.count
			}
		}
		entries = append(entries, serverpackets.TradeItemUpdateEntry{Item: row.snapshot, AvailableCount: available})
	}
	return entries
}

func (o *tradeOffer) empty() bool {
	return len(o.items) == 0
}

func (t *tradeCoordinator) purgeExpiredLocked(now time.Time) {
	for targetID, pending := range t.pendingByTarget {
		if now.Before(pending.expiresAt) {
			continue
		}
		delete(t.pendingByTarget, targetID)
		delete(t.pendingByRequester, pending.requesterID)
	}
}

func (t *tradeCoordinator) processingTransactionLocked(objectID int32) bool {
	if t.active[objectID] != nil {
		return true
	}
	if _, ok := t.pendingByTarget[objectID]; ok {
		return true
	}
	_, ok := t.pendingByRequester[objectID]
	return ok
}

func (l *GameClientLink) livePlayerByID(objectID int32) (*livePlayer, bool) {
	if l.world == nil {
		return nil, false
	}
	obj, ok := l.world.Player(objectID)
	if !ok {
		return nil, false
	}
	live, ok := obj.(*livePlayer)
	return live, ok
}

func (l *GameClientLink) validTradeParticipants(first, second *livePlayer) bool {
	if first == nil || second == nil || first.Inventory() == nil || second.Inventory() == nil {
		return false
	}
	if current, ok := l.livePlayerByID(first.ObjectID()); !ok || current != first {
		return false
	}
	if current, ok := l.livePlayerByID(second.ObjectID()); !ok || current != second {
		return false
	}
	return livePlayersInRange(first, second, tradeInteractionDistance)
}

func livePlayersInRange(first, second *livePlayer, radius int) bool {
	ax, ay, az := first.Position()
	bx, by, bz := second.Position()
	dx := float64(ax - bx)
	dy := float64(ay - by)
	dz := float64(az - bz)
	return math.Sqrt(dx*dx+dy*dy+dz*dz) <= float64(radius)
}

func tradeItemForOffer(live *livePlayer, objectID int32, count int) (*item.Instance, *item.Template, bool) {
	if live == nil || count <= 0 {
		return nil, nil, false
	}
	inv := live.Inventory()
	if inv == nil {
		return nil, nil, false
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil || inst.OwnerID != live.ObjectID() || inst.Equipped() || inst.Count < count {
		return nil, nil, false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Tradable(tmpl) || inst.QuestItem(tmpl) {
		return nil, nil, false
	}
	if !tmpl.Stackable && count > 1 {
		return nil, nil, false
	}
	return inst, tmpl, true
}

func (l *GameClientLink) validateTradeSession(session *tradeSession, checkItems bool) bool {
	if session == nil || !l.validTradeParticipants(session.first, session.second) {
		return false
	}
	if !checkItems {
		return true
	}
	for _, offer := range session.offers {
		if !l.validateTradeOffer(offer) {
			return false
		}
	}
	return true
}

func (l *GameClientLink) validateTradeOffer(offer *tradeOffer) bool {
	if offer == nil || offer.owner == nil {
		return false
	}
	for _, row := range offer.items {
		if _, _, ok := tradeItemForOffer(offer.owner, row.snapshot.ObjectID, row.count); !ok {
			return false
		}
	}
	return true
}

func (l *GameClientLink) exchangeTradeItems(ctx context.Context, session *tradeSession) (bool, int) {
	if session.offers[session.first.ObjectID()].empty() && session.offers[session.second.ObjectID()].empty() {
		return false, 0
	}
	if !tradeReceiverWeightFits(session.first.Inventory(), session.offers[session.second.ObjectID()]) ||
		!tradeReceiverWeightFits(session.second.Inventory(), session.offers[session.first.ObjectID()]) {
		return false, serverpackets.SystemMessageWeightLimitExceeded
	}
	if !tradeReceiverFits(session.first.Inventory(), session.offers[session.second.ObjectID()]) ||
		!tradeReceiverFits(session.second.Inventory(), session.offers[session.first.ObjectID()]) {
		return false, serverpackets.SystemMessageSlotsFull
	}
	if !l.transferTradeOffer(ctx, session.offers[session.first.ObjectID()], session.second.Inventory()) {
		return false, 0
	}
	if !l.transferTradeOffer(ctx, session.offers[session.second.ObjectID()], session.first.Inventory()) {
		return false, 0
	}
	return true, 0
}

func tradeReceiverFits(receiver *itemcontainer.Inventory, offer *tradeOffer) bool {
	if receiver == nil || offer == nil {
		return false
	}
	slots := 0
	seenStack := make(map[int32]bool)
	for _, row := range offer.items {
		tmpl, ok := receiver.Templates().Get(row.snapshot.TemplateID)
		if !ok {
			return false
		}
		if tmpl.Stackable {
			if receiver.ItemByTemplateID(row.snapshot.TemplateID) != nil || seenStack[row.snapshot.TemplateID] {
				continue
			}
			seenStack[row.snapshot.TemplateID] = true
		}
		slots++
	}
	return receiver.ValidateCapacity(slots)
}

func tradeReceiverWeightFits(receiver *itemcontainer.Inventory, offer *tradeOffer) bool {
	if receiver == nil || offer == nil {
		return false
	}
	weight := 0
	for _, row := range offer.items {
		tmpl, ok := receiver.Templates().Get(row.snapshot.TemplateID)
		if !ok {
			return false
		}
		weight += int(tmpl.Weight) * row.count
	}
	return receiver.ValidateWeight(weight)
}

func (l *GameClientLink) transferTradeOffer(ctx context.Context, offer *tradeOffer, receiver *itemcontainer.Inventory) bool {
	if offer == nil || offer.owner == nil || receiver == nil {
		return false
	}
	source := offer.owner.Inventory()
	for _, objectID := range offer.order {
		row := offer.items[objectID]
		if row == nil || !l.transferTradeInventoryItem(ctx, source, receiver, objectID, row.count) {
			return false
		}
	}
	source.UpdateWeight()
	receiver.UpdateWeight()
	return true
}

func (l *GameClientLink) transferTradeInventoryItem(ctx context.Context, source, receiver *itemcontainer.Inventory, objectID int32, count int) bool {
	inst := source.ItemByObjectID(objectID)
	if inst == nil || count <= 0 || inst.Count < count {
		return false
	}
	tmpl, ok := source.Templates().Get(inst.TemplateID)
	if !ok {
		return false
	}
	targetStack := (*item.Instance)(nil)
	if tmpl.Stackable {
		targetStack = receiver.ItemByTemplateID(inst.TemplateID)
	}

	needsNewID := inst.Count > count && targetStack == nil
	if needsNewID && l.ids == nil {
		return false
	}
	newObjectID := int32(0)
	if needsNewID {
		var err error
		newObjectID, err = l.ids.NextID()
		if err != nil {
			l.log.Error().Err(err).Msg("allocate trade item id")
			return false
		}
	}

	if inst.Count == count && targetStack == nil {
		if !source.Remove(inst, false) {
			return false
		}
		result, _ := receiver.Add(inst)
		if result == nil {
			return false
		}
		persistUpdate(ctx, l, result)
		return true
	}

	templateID := inst.TemplateID
	if source.DestroyItem(inst, count) == nil {
		return false
	}
	persistDestroyedOrUpdated(ctx, l, inst)
	if targetStack != nil {
		result, _ := receiver.Add(&item.Instance{TemplateID: templateID, Count: count, ManaLeft: -1})
		if result == nil {
			return false
		}
		persistUpdate(ctx, l, result)
		return true
	}
	result := receiver.AddNew(templateID, count, newObjectID)
	if result == nil {
		return false
	}
	persistSave(ctx, l, result)
	return true
}
