package network

import (
	"context"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	enchantflow "github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

func (l *GameClientLink) enchantStateStore() *enchantflow.State {
	if l.enchantState == nil {
		l.enchantState = enchantflow.NewState()
	}
	return l.enchantState
}

func (l *GameClientLink) enchantService() *enchantflow.Service {
	if l.enchant == nil {
		l.enchant = enchantflow.NewService(l.enchantStateStore(), l.ids, l.rollEnchant, enchantflow.DefaultConfig())
	}
	return l.enchant
}

func (l *GameClientLink) useEnchantScroll(live *livePlayer, scroll *item.Instance) bool {
	if live == nil || scroll == nil {
		return false
	}
	result, ok := l.enchantService().UseScroll(live.ObjectID(), live.Inventory(), scroll)
	if !ok {
		return false
	}
	if result.FirstSelect {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSelectItemToEnchant))
	}
	live.SendFrame(serverpackets.FrameChooseInventoryItem(result.ScrollItemID))
	return true
}

// enchantLiveItem answers RequestEnchantItem. A dead player may enchant too:
// the request is gated on the store/trade state only.
func (l *GameClientLink) enchantLiveItem(ctx context.Context, live *livePlayer, req clientpackets.RequestEnchantItem) {
	if live == nil || req.ObjectID == 0 {
		return
	}
	inv := live.Inventory()
	if inv == nil {
		return
	}

	playerID := live.ObjectID()
	end := l.itemInstances.BeginOperation()
	defer end()
	result, err := l.enchantService().EnchantItem(enchantflow.Request{
		PlayerID: playerID,
		Inv:      inv,
		ObjectID: req.ObjectID,
		Busy:     live.Operating() || (l.trades != nil && l.trades.ProcessingTransaction(playerID)),
		TradeActive: func() bool {
			return l.trades != nil && l.trades.HasActive(playerID)
		},
	})
	if err != nil {
		l.log.Error().Err(err).Msg("enchant item")
	}
	l.applyPersistActions(result.Persist)
	end()
	if len(result.Steps) == 0 {
		return
	}
	l.applyEnchantSteps(live, result.Steps)
}

// cancelActiveEnchant drops live's scroll selection, telling the client when
// there was one.
func (l *GameClientLink) cancelActiveEnchant(live *livePlayer) {
	if live == nil {
		return
	}
	result := l.enchantService().Cancel(live.ObjectID(), live.Inventory())
	l.applyEnchantSteps(live, result.Steps)
}

func (l *GameClientLink) applyEnchantSteps(live *livePlayer, steps []enchantflow.Step) {
	for _, step := range steps {
		switch step.Kind {
		case enchantflow.StepSystemMessage:
			l.sendEnchantMessage(live, step.Message)
		case enchantflow.StepEnchantResult:
			live.SendFrame(serverpackets.FrameEnchantResult(enchantResult(step.EnchantResult)))
		case enchantflow.StepBroadcastEquipment:
			l.broadcastEquipmentChange(live)
		case enchantflow.StepGrantEnchantSkill:
			l.applyEnchantSkillChange(live, step.Template, true)
		case enchantflow.StepRevokeEnchantSkill:
			l.applyEnchantSkillChange(live, step.Template, false)
		case enchantflow.StepGrantArmorSetSkill:
			l.applyArmorSetSkillChange(live, step.SkillID, true)
		case enchantflow.StepRevokeArmorSetSkill:
			l.applyArmorSetSkillChange(live, step.SkillID, false)
		case enchantflow.StepUnequipped:
			// The grade penalty is not refreshed for an item a failed
			// enchant destroys, so only the equip side effects are undone
			// here.
			l.applyEquipItemStats(live, live.Inventory(), invops.Result{EquipmentChanged: true, Changed: step.Unequipped})
		case enchantflow.StepCancelTrade:
			l.cancelActiveTrade(live)
		}
	}
}

// applyEnchantSkillChange adds or removes tmpl's +4 enchant skill after an
// enchant attempt on the equipped weapon, resending SkillList whenever the
// weapon has a loaded one.
func (l *GameClientLink) applyEnchantSkillChange(live *livePlayer, tmpl *item.Template, grant bool) {
	if l.skills == nil {
		return
	}
	var resend bool
	if grant {
		var err error
		resend, err = l.skills.GrantEnchant4Skill(live.Character, tmpl)
		if err != nil {
			l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("grant enchant skill")
		}
	} else {
		resend = l.skills.RevokeEnchant4Skill(live.Character, tmpl)
	}
	if resend {
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	}
}

// applyArmorSetSkillChange adds or removes the worn armor set's +6 skill
// after an enchant attempt on a worn armor piece. A grant resends SkillList
// when the skill is loaded; a removal always does.
func (l *GameClientLink) applyArmorSetSkillChange(live *livePlayer, skillID int32, grant bool) {
	if l.skills == nil {
		return
	}
	resend := true
	if grant {
		var err error
		resend, err = l.skills.GrantArmorSetSkill(live.Character, skillID)
		if err != nil {
			l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("grant armor set skill")
		}
	} else {
		l.skills.RevokeArmorSetSkill(live.Character, skillID)
	}
	if resend {
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	}
}

func (l *GameClientLink) sendEnchantMessage(live *livePlayer, message enchantflow.Message) {
	switch message.Code {
	case enchantflow.MessageSelectItemToEnchant:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSelectItemToEnchant))
	case enchantflow.MessageEnchantScrollCancelled:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageEnchantScrollCancelled))
	case enchantflow.MessageInappropriateEnchantCondition:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInappropriateEnchantCondition))
	case enchantflow.MessageNotEnoughItems:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
	case enchantflow.MessageS1SuccessfullyEnchanted:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageS1SuccessfullyEnchanted, message.ItemID))
	case enchantflow.MessageS1S2SuccessfullyEnchanted:
		live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageS1S2SuccessfullyEnchanted, message.Number, message.ItemID))
	case enchantflow.MessageBlessedEnchantFailed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageBlessedEnchantFailed))
	case enchantflow.MessageEarnedS2S1S:
		live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageEarnedS2S1S, message.ItemID, message.Number))
	case enchantflow.MessageEnchantmentFailedS1S2Evaporated:
		live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageEnchantmentFailedS1S2Evaporated, message.Number, message.ItemID))
	case enchantflow.MessageEnchantmentFailedS1Evaporated:
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageEnchantmentFailedS1Evaporated, message.ItemID))
	case enchantflow.MessageCannotEnchantWhileStore:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotEnchantWhileStore))
	case enchantflow.MessageTradeAttemptFailed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTradeAttemptFailed))
	}
}

func enchantResult(result enchantflow.ResultCode) serverpackets.EnchantResult {
	switch result {
	case enchantflow.ResultSuccess:
		return serverpackets.EnchantResultSuccess
	case enchantflow.ResultUnsuccess:
		return serverpackets.EnchantResultUnsuccess
	case enchantflow.ResultBrokenNoCrystals:
		return serverpackets.EnchantResultBrokenNoCrystals
	case enchantflow.ResultBrokenWithCrystals:
		return serverpackets.EnchantResultBrokenWithCrystals
	default:
		return serverpackets.EnchantResultCancelled
	}
}

// applyPersistActions queues actions' item-row writes on the persistence
// worker instead of writing them here: this runs on an actor queue, where a
// slow database would hold a pool worker and stall unrelated actors.
//
// The rows one operation changed are written as one transaction
// (itemStore.WriteBatch), so they land together or not at all. A trade moves
// items between two inventories: written row by row, a failure or crash
// between the giver's rows and the receiver's leaves the stack in both
// inventories or in neither, and the next login restores that. Rows the write
// skips because a later write of theirs already landed are no exception — the
// database holds their newer state already.
//
// Each row's state is the one this queue produced, and takes its place in
// that row's write order before it is queued (persist.Order). An item that
// changes hands has its next write queued on the new owner's lane, a
// destroyed item's delete comes from the persistence tick's own lane, and a
// dropped item is picked up as a new instance carrying the same object id —
// in each case a second lane writes the row this one is about to, and only
// the reservation decides which of them the row keeps. The write runs on one
// owner's lane but is owed on every owner's whose rows it carries, so a login
// waiting on any of those lanes waits for it (queueItemWrite).
//
// Both a new row and a changed one are written as the same upsert, which is
// what the persistence tick's own batch does for every live item
// (task.ItemInstances.addToBatch). The distinction cannot survive ordering:
// the write that creates a row may be the one a later write supersedes, and a
// plain UPDATE from that later write would then touch nothing and lose the
// row altogether.
//
// A count-zero pet collar is the one row whose pets-row delete needs the
// collar's own lane (task.ItemInstances.laneKey); the item-row delete queued
// here touches no pets row, and both deletes are idempotent, so it stays on
// the owner's lane with everything else.
//
// A failed write is logged and left to the persistence tick, which still has
// every mutated instance pending and writes their current state. The tick
// writes per owner lane, and the next handler write of one of these rows may
// carry that row alone, so the rows are bound first (task.ItemInstances.Bind):
// until one write lands them all, any write of one of them takes the rest
// along. This write widens the same way when one of its rows is still bound to
// an earlier operation's that never landed — the receiver spending traded
// adena before the trade's own rows are in the database. A widened row's
// current state decides between its save and its delete, as the tick's does.
//
// The binding comes after the mutation, so every caller opens the operation
// (task.ItemInstances.BeginOperation) before it mutates and ends it once this
// returns: until then the tick and a container's last flush read none of its
// rows, and so never land one of them changed but not yet bound.
func (l *GameClientLink) applyPersistActions(actions []invops.Persist) {
	if l.items == nil {
		return
	}
	// A row can be named twice — a stack that gives on one leg of a trade and
	// receives on the other — and a write may reserve each row once. Its last
	// action decides whether it is saved or deleted.
	type rowAction struct {
		inst    *item.Instance
		ownerID int32
		remove  bool
		// widened marks a row bound to this write's rows by an earlier
		// operation, which this one did not touch.
		widened bool
	}
	rows := make(map[int32]rowAction, len(actions))
	order := make([]int32, 0, len(actions))
	for _, action := range actions {
		var row rowAction
		var objectID int32
		switch action.Action {
		case invops.PersistSave, invops.PersistUpdate:
			if action.Item == nil {
				continue
			}
			row, objectID = rowAction{inst: action.Item}, action.Item.ObjectID
		case invops.PersistDelete:
			row, objectID = rowAction{ownerID: action.OwnerID, remove: true}, action.ObjectID
		default:
			continue
		}
		if _, seen := rows[objectID]; !seen {
			order = append(order, objectID)
		}
		rows[objectID] = row
	}
	if len(order) == 0 {
		return
	}
	// carried is the group this write lands whole, if it binds one. It
	// holds every row of the groups the write widened to, so it alone
	// stands for them.
	var carried task.BoundGroups
	if l.itemInstances != nil {
		widened, _ := l.itemInstances.Widen(order)
		for _, bound := range widened {
			row := rowAction{inst: bound.Inst, ownerID: bound.OwnerID, widened: true}
			if bound.Inst == nil {
				row.remove = true
			}
			rows[bound.ObjectID] = row
			order = append(order, bound.ObjectID)
		}
		if len(order) > 1 {
			bound := make([]task.BoundRow, 0, len(order))
			for _, objectID := range order {
				row := rows[objectID]
				ownerID := row.ownerID
				if row.inst != nil && !row.widened {
					ownerID = row.inst.Snapshot().OwnerID
				}
				bound = append(bound, task.BoundRow{ObjectID: objectID, OwnerID: ownerID, Inst: row.inst})
			}
			carried = l.itemInstances.Bind(bound)
		}
	}

	var batch item.FlushBatch
	owners := make([]int32, 0, 2)
	reserved := l.itemWrites.Begin()
	for _, objectID := range order {
		row := rows[objectID]
		if row.remove {
			reserved.Add(objectID)
			batch.Deletes = append(batch.Deletes, objectID)
		} else {
			// The state and its place are taken together, under the
			// instance, so no mutation can land between them and leave this
			// write holding a place that does not match what it will write.
			row.inst.WithState(func(s item.InstanceState) {
				reserved.Add(s.ObjectID)
				if row.widened && (s.Count <= 0 || s.Location == item.LocationVoid) {
					batch.Deletes = append(batch.Deletes, s.ObjectID)
					return
				}
				batch.Saves = append(batch.Saves, s)
				row.ownerID = s.OwnerID
				l.addAugmentationRow(&batch, s)
			})
		}
		if !slices.Contains(owners, row.ownerID) {
			owners = append(owners, row.ownerID)
		}
	}
	instances := l.itemInstances
	l.queueItemWrite(reserved, func(keep []int32) {
		batch := keptRows(batch, keep)
		ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
		defer cancel()
		if err := l.items.WriteBatch(ctx, batch); err != nil {
			l.log.Error().Err(err).Int("saves", len(batch.Saves)).Int("deletes", len(batch.Deletes)).
				Msg("write item rows")
			return
		}
		if instances != nil {
			instances.Landed(carried, keep)
		}
	}, owners[0], owners[1:]...)
}

// addAugmentationRow adds the augmentations row a weapon's saved state
// carries to batch: written when it is augmented, deleted when it is not,
// as the persistence tick does. The row lands in the same transaction as
// the item's own, so an augmentation and the life stone it consumed cannot
// land apart.
func (l *GameClientLink) addAugmentationRow(batch *item.FlushBatch, s item.InstanceState) {
	if l.itemTemplates == nil {
		return
	}
	if tmpl, ok := l.itemTemplates.Get(s.TemplateID); !ok || tmpl.Kind != item.KindWeapon {
		return
	}
	if s.Augmentation == nil {
		batch.AugmentationDeletes = append(batch.AugmentationDeletes, s.ObjectID)
		return
	}
	batch.AugmentationSaves = append(batch.AugmentationSaves, item.FlushAugmentationSave{ObjectID: s.ObjectID, Augmentation: *s.Augmentation})
}

// keptRows narrows batch to the rows in keep, which is sorted ascending: the
// rows no later write has landed on yet (persist.Write.Run).
func keptRows(batch item.FlushBatch, keep []int32) item.FlushBatch {
	kept := func(id int32) bool {
		_, found := slices.BinarySearch(keep, id)
		return found
	}
	return item.FlushBatch{
		Saves:   slices.DeleteFunc(slices.Clone(batch.Saves), func(s item.InstanceState) bool { return !kept(s.ObjectID) }),
		Deletes: slices.DeleteFunc(slices.Clone(batch.Deletes), func(id int32) bool { return !kept(id) }),
		AugmentationSaves: slices.DeleteFunc(slices.Clone(batch.AugmentationSaves), func(a item.FlushAugmentationSave) bool {
			return !kept(a.ObjectID)
		}),
		AugmentationDeletes: slices.DeleteFunc(slices.Clone(batch.AugmentationDeletes), func(id int32) bool { return !kept(id) }),
	}
}

// itemWriteRowWait is how long a queued item write waits for its row before
// giving its lane back. It only has to be long enough that an uncontended row
// is taken on the first try; the wait for a row another write is holding is
// paid by re-queueing, not by sitting on the lane.
const itemWriteRowWait = 20 * time.Millisecond

// queueItemWrite runs write on laneOwner's persistence lane once every row
// reserved holds is free, handing it the rows still worth writing
// (persist.Write.Run). The write is owed on laneOwner's lane and on the lane
// of every owner in alsoOwed, so waiting on any of them waits for it.
//
// A lane serves every owner that maps to it (persist.Lanes of them for the
// whole world), so a write that simply waited for its rows would stall the
// character, shortcut, skill and pet writes queued behind it — and a row can
// be held by the persistence tick's chunk for its whole transaction.
// This comes back to the lane instead, which keeps the lane draining and
// still lands the write in its reserved place.
//
// Coming back puts the write behind whatever the lane has taken meanwhile,
// including a flush marker pushed after it was first queued, so the write is
// also booked as work the lane owes: awaitPersistence waits for a lane to
// report that what it queued has run, and a restart reads the items table
// straight after. The debt is booked here, before the first Enqueue and on
// the actor queue — booking it inside attempt instead would let the write
// run, fail and hand itself forward after a flush had already read what the
// lane owes, which is the hole the booking exists to close.
//
// The debt is settled however the write ends — landed, dropped as superseded,
// cancelled, or panicking — on every exit but the one that hands it to
// another attempt. A lane owing work that nothing will ever settle wedges
// every later flush of it, so this follows the row release in persist.Order:
// the worker recovers a panicking job, so a write that panics has to leave
// the bookkeeping as it found it.
func (l *GameClientLink) queueItemWrite(reserved *persist.Write, write func(keep []int32), laneOwner int32, alsoOwed ...int32) {
	owed := []persist.Owed{l.persist.Owe(laneOwner)}
	for _, ownerID := range alsoOwed {
		owed = append(owed, l.persist.Owe(ownerID))
	}
	settleAll := func() {
		for _, o := range owed {
			o.Settle()
		}
	}
	var attempt func()
	attempt = func() {
		settle := true
		defer func() {
			if settle {
				settleAll()
			}
		}()
		if reserved.TryRun(itemWriteRowWait, write) {
			return
		}
		if l.persist.Enqueue(laneOwner, attempt) {
			settle = false // Still owed: whichever attempt ends it settles.
			return
		}
		// Shutdown: nothing will come back for this, so take the wait here
		// rather than dropping a write the row is still owed.
		reserved.Run(write)
	}
	if !l.persist.Enqueue(laneOwner, attempt) {
		reserved.Cancel()
		settleAll()
	}
}

func (l *GameClientLink) rollEnchant() float64 {
	if l.enchantRoll != nil {
		return l.enchantRoll()
	}
	return rnd.GetFloat(1)
}
