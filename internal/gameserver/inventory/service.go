package inventory

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// IDAllocator supplies object ids when a mutation needs to split a stack.
type IDAllocator interface {
	NextID() (int32, error)
}

// Service performs inventory mutations without knowing how clients are notified.
type Service struct {
	ids IDAllocator
}

// NewService returns an inventory mutation service.
func NewService(ids IDAllocator) *Service {
	return &Service{ids: ids}
}

// DropResult is the domain result of dropping an item to the world.
type DropResult struct {
	Result
	Dropped  *item.Instance
	Template *item.Template
}

// TransferResult is the domain result of moving an item between inventories.
type TransferResult struct {
	Result
	Item *item.Instance
}

// CrystallizeFailure is the non-mutating reason a crystallize request failed.
type CrystallizeFailure uint8

const (
	// CrystallizeOK means the source item was crystallized.
	CrystallizeOK CrystallizeFailure = iota
	// CrystallizeNoop means the request is invalid and should be ignored.
	CrystallizeNoop
	// CrystallizeNoSkill means the character has no crystallize skill.
	CrystallizeNoSkill
	// CrystallizeGradeTooHigh means the skill level cannot crystallize the item grade.
	CrystallizeGradeTooHigh
)

// DestroyFailure is the non-mutating reason a destroy request failed.
type DestroyFailure uint8

const (
	// DestroyOK means the item was destroyed.
	DestroyOK DestroyFailure = iota
	// DestroyNoop means the request should be ignored.
	DestroyNoop
	// DestroyInvalidCount means the requested count is invalid.
	DestroyInvalidCount
	// DestroyNotDestroyable means the item cannot be destroyed.
	DestroyNotDestroyable
	// DestroyHeroItem means the item is a hero item.
	DestroyHeroItem
)

// CrystallizeResult is the domain result of crystallizing one item.
type CrystallizeResult struct {
	Result
	SourceItemID int32
	// SourceEnchantLevel is the crystallized item's enchant level, which a
	// worn item's removal message names.
	SourceEnchantLevel int
	CrystalItemID      int32
	CrystalCount       int
}

// EquipFailure is the non-mutating reason an equip toggle failed.
type EquipFailure uint8

const (
	// EquipOK means the item was equipped or unequipped.
	EquipOK EquipFailure = iota
	// EquipNoop means the request is invalid and changed nothing.
	EquipNoop
	// EquipBadCondition means the paperdoll refused the item: legs, feet,
	// gloves or a helmet while formal wear is worn.
	EquipBadCondition
)

// ToggleEquipItem equips objectID into a player's paperdoll, or unequips it
// when it is already worn.
func (s *Service) ToggleEquipItem(inv *itemcontainer.Inventory, objectID int32) (Result, EquipFailure) {
	if inv == nil {
		return Result{}, EquipNoop
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return Result{}, EquipNoop
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.Slot == item.SlotNone {
		return Result{}, EquipNoop
	}

	st := inst.Snapshot()
	if st.Equipped() {
		changed := inv.UnequipItem(inst)
		if len(changed) == 0 {
			return Result{}, EquipNoop
		}
		return Result{EquipmentChanged: true, Changed: changed}, EquipOK
	}
	changed, refused := inv.EquipPlayerItem(inst, tmpl)
	if refused {
		return Result{}, EquipBadCondition
	}
	if len(changed) == 0 {
		return Result{}, EquipNoop
	}
	return Result{EquipmentChanged: true, Changed: changed}, EquipOK
}

// UnequipBodySlot clears the paperdoll position represented by bodySlot.
// Changed lists what the paperdoll changed in order: a bow's or rod's
// arrows or lure ahead of the bow or rod itself.
func (s *Service) UnequipBodySlot(inv *itemcontainer.Inventory, bodySlot int32) (Result, bool) {
	if inv == nil {
		return Result{}, false
	}
	paperdollSlot, ok := item.Slot(bodySlot).PaperdollIndex()
	if !ok {
		return Result{}, false
	}
	changed := inv.UnequipItem(inv.ItemAt(paperdollSlot))
	if len(changed) == 0 {
		return Result{}, false
	}
	return Result{EquipmentChanged: true, Changed: changed}, true
}

// DropFailure is the non-mutating reason a drop request fails.
type DropFailure uint8

const (
	// DropOK means the drop may go ahead.
	DropOK DropFailure = iota
	// DropCannotDiscard means the item cannot be discarded: it is not
	// held, the caller refuses it, the count is zero or above the stack,
	// or the item is not droppable.
	DropCannotDiscard
	// DropNoop means the request is ignored: a quest item, a negative
	// count, or several units of an unstackable item.
	DropNoop
)

// DropItemFailure classifies a drop request without mutating inv, in the
// order its checks answer: whether the item can be discarded at all comes
// before anything about where it would land. refused reports that the
// caller already refuses the item or the drop itself: an item bound to a pet
// that is out or selected as the enchant scroll, or a server that allows no
// discards.
func (s *Service) DropItemFailure(inv *itemcontainer.Inventory, objectID int32, count int, refused bool) DropFailure {
	if inv == nil {
		return DropNoop
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil || refused || count == 0 {
		return DropCannotDiscard
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !inst.Dropable(tmpl) {
		return DropCannotDiscard
	}
	if inst.QuestItem(tmpl) {
		return DropNoop
	}
	if count > inst.Snapshot().Count {
		return DropCannotDiscard
	}
	if count < 0 || (!tmpl.Stackable && count > 1) {
		return DropNoop
	}
	return DropOK
}

// DropItem removes count units from inv for a world drop.
func (s *Service) DropItem(inv *itemcontainer.Inventory, objectID int32, count int) (DropResult, bool, error) {
	if inv == nil || count <= 0 {
		return DropResult{}, false, nil
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return DropResult{}, false, nil
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	st := inst.Snapshot()
	if !ok || !inst.Dropable(tmpl) || inst.QuestItem(tmpl) || st.Count < count {
		return DropResult{}, false, nil
	}
	if !tmpl.Stackable && count > 1 {
		return DropResult{}, false, nil
	}
	newObjectID := int32(0)
	if st.Count > count {
		id, ok, err := s.nextID()
		if err != nil || !ok {
			return DropResult{}, false, err
		}
		newObjectID = id
	}
	wasEquipped := st.Equipped() && st.Count <= count
	changed := unequipConsumed(inv, inst, wasEquipped)
	dropped := inv.DropItem(objectID, count, newObjectID)
	if dropped == nil {
		return DropResult{}, false, nil
	}
	return DropResult{
		Result:   Result{EquipmentChanged: wasEquipped, Changed: changed},
		Dropped:  dropped,
		Template: tmpl,
	}, true, nil
}

// PickupFailure is the non-mutating reason a ground-item pickup failed.
type PickupFailure uint8

const (
	// PickupOK means the ground item was moved into inv.
	PickupOK PickupFailure = iota
	// PickupNoop means the request is invalid and should be ignored.
	PickupNoop
	// PickupLootLocked means ground is owned by someone other than picker.
	PickupLootLocked
	// PickupSlotsFull means inv lacks free inventory slots.
	PickupSlotsFull
)

// LootLocked reports whether a ground item owned by ownerID is reserved
// against pickerID: an unowned item (ownerID == 0) is free for anyone, an
// owned one only goes to its owner. Callers pass an owner id they already
// read, so one snapshot decides both the lock and the pickup.
//
// The full loot rule also admits members of the owner's looting party; this
// narrower owner comparison is the repo's existing simplification, now stated
// in one place instead of two.
func LootLocked(ownerID, pickerID int32) bool {
	return ownerID != 0 && ownerID != pickerID
}

// PickupGround moves ground (with its loaded template) into inv, the same
// way any other incoming item would merge into an existing stack or take a
// free slot. pickerID is compared against ground.OwnerID to enforce a loot
// lock; an unowned ground item (OwnerID == 0) is free for anyone.
func (s *Service) PickupGround(inv *itemcontainer.Inventory, ground *item.Instance, tmpl *item.Template, pickerID int32) (Result, PickupFailure) {
	groundState := ground.Snapshot()
	if inv == nil || ground == nil || tmpl == nil || groundState.Count <= 0 {
		return Result{}, PickupNoop
	}
	if !inv.ValidateCapacity(inv.SlotsNeededFor(ground, tmpl)) {
		return Result{}, PickupSlotsFull
	}
	if LootLocked(groundState.OwnerID, pickerID) {
		return Result{}, PickupLootLocked
	}

	picked := groundState.Instance()
	result, absorbed := inv.Add(picked)
	if result == nil {
		return Result{}, PickupNoop
	}
	if absorbed {
		return Result{Persist: []Persist{Update(result), Delete(groundState.OwnerID, groundState.ObjectID)}}, PickupOK
	}
	return Result{Persist: []Persist{Save(result)}}, PickupOK
}

// DestroyItemFailure classifies a destroy request without mutating inv.
func (s *Service) DestroyItemFailure(inv *itemcontainer.Inventory, objectID int32, count int) DestroyFailure {
	if inv == nil {
		return DestroyNoop
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return DestroyNoop
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	st := inst.Snapshot()
	if !ok {
		return DestroyNoop
	}
	if count <= 0 || st.Count < count {
		return DestroyInvalidCount
	}
	if !tmpl.Stackable && count > 1 {
		return DestroyNoop
	}
	if tmpl.HeroItem() {
		return DestroyHeroItem
	}
	if !inst.Destroyable(tmpl) {
		return DestroyNotDestroyable
	}
	return DestroyOK
}

// DestroyItemResult consumes count units from inv and classifies a rejection.
func (s *Service) DestroyItemResult(inv *itemcontainer.Inventory, objectID int32, count int) (Result, DestroyFailure) {
	if failure := s.DestroyItemFailure(inv, objectID, count); failure != DestroyOK {
		return Result{}, failure
	}
	inst := inv.ItemByObjectID(objectID)
	st := inst.Snapshot()
	wasEquipped := st.Equipped() && st.Count <= count
	changed := unequipConsumed(inv, inst, wasEquipped)
	if inv.DestroyItem(inst, count) == nil {
		return Result{}, DestroyNoop
	}
	return Result{EquipmentChanged: wasEquipped, Changed: changed}, DestroyOK
}

// unequipConsumed takes a worn item off the paperdoll ahead of the removal
// that consumes all of it, so the change list carries everything that
// removal took off: a bow's or rod's arrows or lure too, ahead of the bow or
// rod. The removal itself then finds nothing left to unequip. A removal
// that leaves part of a worn stack behind (consumed false) unequips
// nothing.
func unequipConsumed(inv *itemcontainer.Inventory, inst *item.Instance, consumed bool) []*item.Instance {
	if !consumed {
		return nil
	}
	return inv.UnequipItem(inst)
}

// DestroyItem consumes count units from inv.
func (s *Service) DestroyItem(inv *itemcontainer.Inventory, objectID int32, count int) (Result, bool) {
	res, failure := s.DestroyItemResult(inv, objectID, count)
	return res, failure == DestroyOK
}

// TransferItem moves count units from source to receiver and reports store writes.
func (s *Service) TransferItem(source, receiver *itemcontainer.Inventory, objectID int32, count int) (TransferResult, bool, error) {
	if source == nil || receiver == nil || count <= 0 {
		return TransferResult{}, false, nil
	}
	inst := source.ItemByObjectID(objectID)
	if inst == nil {
		return TransferResult{}, false, nil
	}
	st := inst.Snapshot()
	if count > st.Count {
		count = st.Count
	}
	tmpl, ok := source.Templates().Get(inst.TemplateID)
	if !ok {
		return TransferResult{}, false, nil
	}
	targetStack := (*item.Instance)(nil)
	if tmpl.Stackable {
		targetStack = receiver.ItemByTemplateID(st.TemplateID)
	}

	newObjectID := int32(0)
	if st.Count > count || targetStack != nil {
		id, ok, err := s.nextID()
		if err != nil || !ok {
			return TransferResult{}, false, err
		}
		newObjectID = id
	}

	result, freedObjectID, freed := source.TransferItem(objectID, count, receiver, newObjectID)
	if result == nil {
		return TransferResult{}, false, nil
	}

	out := TransferResult{Item: result}
	if remaining := source.ItemByObjectID(objectID); remaining != nil {
		out.Persist = append(out.Persist, Update(remaining))
	}
	if freed {
		out.Persist = append(out.Persist, Delete(source.OwnerID(), freedObjectID))
	}
	if newObjectID != 0 && result.ObjectID == newObjectID {
		out.Persist = append(out.Persist, Save(result))
	} else {
		out.Persist = append(out.Persist, Update(result))
	}
	return out, true, nil
}

var errNoIDAllocator = errors.New("inventory exchange: no object id allocator")

// Move is one item row an exchange hands from one inventory to the other.
type Move struct {
	ObjectID int32
	Count    int
}

// Exchange moves aOut from a to b and bOut from b to a as one step (see
// itemcontainer.Exchange): check sees both inventories exactly as the moves
// will find them and can veto the whole exchange, and nothing either owner
// does can land between the check and the last move. It reports false,
// having changed nothing, when check vetoes. A move failing after the check
// approved is a broken check; it reports false with the persistence of the
// moves already made, so the caller still writes what did happen.
func (s *Service) Exchange(a, b *itemcontainer.Inventory, aOut, bOut []Move, check func(a, b itemcontainer.Held) bool) (Result, bool, error) {
	if a == nil || b == nil || a == b {
		return Result{}, false, nil
	}
	// Ids come from outside the inventory locks, and whether a row splits is
	// only known under them, so every row gets one up front. A move that
	// could not get its id would fail after earlier rows had landed, so no
	// allocator means no exchange at all.
	// ponytail: a row that needs no id burns its id; the allocator never
	// reuses an id behind its cursor, so handing it back would buy nothing.
	ids := make([]int32, len(aOut)+len(bOut))
	for i := range ids {
		id, ok, err := s.nextID()
		if err != nil {
			return Result{}, false, err
		}
		if !ok {
			return Result{}, false, errNoIDAllocator
		}
		ids[i] = id
	}

	var res Result
	ok := false
	itemcontainer.Exchange(a, b, func(heldA, heldB itemcontainer.Held) {
		if check != nil && !check(heldA, heldB) {
			return
		}
		ok = moveAll(&res, heldA, heldB, a.OwnerID(), aOut, ids) &&
			moveAll(&res, heldB, heldA, b.OwnerID(), bOut, ids[len(aOut):])
	})
	return res, ok, nil
}

// moveAll transfers every row from source to receiver, row i splitting off
// under ids[i] when it has to.
func moveAll(res *Result, source, receiver itemcontainer.Held, sourceOwnerID int32, rows []Move, ids []int32) bool {
	for i, row := range rows {
		m, ok := source.Transfer(row.ObjectID, row.Count, receiver, ids[i])
		if !ok {
			return false
		}
		if m.Remaining != nil {
			res.Persist = append(res.Persist, Update(m.Remaining))
		}
		if m.FreedObjectID != 0 {
			res.Persist = append(res.Persist, Delete(sourceOwnerID, m.FreedObjectID))
		}
		if m.Created {
			res.Persist = append(res.Persist, Save(m.Item))
		} else {
			res.Persist = append(res.Persist, Update(m.Item))
		}
	}
	return true
}

// CrystallizeItem destroys up to count units of objectID and adds the crystal reward.
func (s *Service) CrystallizeItem(inv *itemcontainer.Inventory, objectID int32, count, skillLevel int) (CrystallizeResult, CrystallizeFailure, error) {
	if count <= 0 {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	if skillLevel <= 0 {
		return CrystallizeResult{}, CrystallizeNoSkill, nil
	}
	if inv == nil {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.HeroItem() || inst.ShadowItem(tmpl) {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	st := inst.Snapshot()
	crystalItemID, crystalCount, ok := tmpl.CrystalReward(st.EnchantLevel)
	if !ok {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	if !item.CanCrystallize(tmpl.Crystal, skillLevel) {
		return CrystallizeResult{}, CrystallizeGradeTooHigh, nil
	}
	if _, ok := inv.Templates().Get(crystalItemID); !ok {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	crystalObjectID, ok, err := s.nextID()
	if err != nil || !ok {
		return CrystallizeResult{}, CrystallizeNoop, err
	}

	if count > st.Count {
		count = st.Count
	}
	wasEquipped := st.Equipped() && st.Count <= count
	changed := unequipConsumed(inv, inst, wasEquipped)
	if inv.DestroyItem(inst, count) == nil {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	if inv.AddNew(crystalItemID, int(crystalCount), crystalObjectID) == nil {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	return CrystallizeResult{
		Result:             Result{EquipmentChanged: wasEquipped, Changed: changed},
		SourceItemID:       st.TemplateID,
		SourceEnchantLevel: st.EnchantLevel,
		CrystalItemID:      crystalItemID,
		CrystalCount:       int(crystalCount),
	}, CrystallizeOK, nil
}

func (s *Service) nextID() (int32, bool, error) {
	if s == nil || s.ids == nil {
		return 0, false, nil
	}
	id, err := s.ids.NextID()
	if err != nil {
		return 0, false, fmt.Errorf("allocate item id: %w", err)
	}
	return id, true, nil
}
