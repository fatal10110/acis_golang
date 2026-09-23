package inventory

import (
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
	SourceItemID  int32
	CrystalItemID int32
	CrystalCount  int
}

// ToggleEquipItem equips objectID, or unequips it when it is already worn.
func (s *Service) ToggleEquipItem(inv *itemcontainer.Inventory, objectID int32) (Result, bool) {
	if inv == nil {
		return Result{}, false
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return Result{}, false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.Slot == item.SlotNone {
		return Result{}, false
	}

	st := inst.Snapshot()
	if st.Equipped() {
		unequipped := inv.UnequipSlot(st.LocationData)
		if unequipped == nil {
			return Result{}, false
		}
		return Result{EquipmentChanged: true, Changed: []*item.Instance{unequipped}}, true
	}
	changed := inv.EquipItem(inst, tmpl)
	if len(changed) == 0 {
		return Result{}, false
	}
	return Result{EquipmentChanged: true, Changed: changed}, true
}

// UnequipBodySlot clears the paperdoll position represented by bodySlot.
func (s *Service) UnequipBodySlot(inv *itemcontainer.Inventory, bodySlot int32) (Result, bool) {
	if inv == nil {
		return Result{}, false
	}
	paperdollSlot, ok := item.Slot(bodySlot).PaperdollIndex()
	if !ok {
		return Result{}, false
	}
	unequipped := inv.UnequipSlot(paperdollSlot)
	if unequipped == nil {
		return Result{}, false
	}
	return Result{EquipmentChanged: true, Changed: []*item.Instance{unequipped}}, true
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
	dropped := inv.DropItem(objectID, count, newObjectID)
	if dropped == nil {
		return DropResult{}, false, nil
	}
	return DropResult{
		Result:   Result{EquipmentChanged: wasEquipped},
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
// The reference also admits the owner's looting party (isLooterOrInLooterParty);
// this narrower owner comparison is the repo's existing simplification, now
// stated in one place instead of two.
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
	if inv.DestroyItem(inst, count) == nil {
		return Result{}, DestroyNoop
	}
	res := Result{EquipmentChanged: wasEquipped}
	if wasEquipped {
		res.Changed = []*item.Instance{inst}
	}
	return res, DestroyOK
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
	// Ids come from outside the inventory locks, so every row gets one up
	// front; a row that needs none hands it back below.
	ids := make([]int32, len(aOut)+len(bOut))
	for i := range ids {
		id, _, err := s.nextID()
		if err != nil {
			s.releaseIDs(ids)
			return Result{}, false, err
		}
		ids[i] = id
	}
	defer s.releaseIDs(ids)

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

// moveAll transfers every row from source to receiver, taking ids[i] for row
// i and zeroing it once the move keeps it.
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
			ids[i] = 0
			res.Persist = append(res.Persist, Save(m.Item))
		} else {
			res.Persist = append(res.Persist, Update(m.Item))
		}
	}
	return true
}

// releaseIDs hands unused ids back when the allocator takes them back.
func (s *Service) releaseIDs(ids []int32) {
	if s == nil {
		return
	}
	releaser, ok := s.ids.(interface{ ReleaseID(int32) })
	if !ok {
		return
	}
	for _, id := range ids {
		if id != 0 {
			releaser.ReleaseID(id)
		}
	}
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
	sourceItemID := st.TemplateID
	if inv.DestroyItem(inst, count) == nil {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}
	if inv.AddNew(crystalItemID, int(crystalCount), crystalObjectID) == nil {
		return CrystallizeResult{}, CrystallizeNoop, nil
	}

	return CrystallizeResult{
		Result:        Result{EquipmentChanged: wasEquipped},
		SourceItemID:  sourceItemID,
		CrystalItemID: crystalItemID,
		CrystalCount:  int(crystalCount),
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
