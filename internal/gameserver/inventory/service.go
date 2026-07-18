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
	if st.Location == item.LocationPaperdoll || st.Location == item.LocationPetEquip {
		if inv.UnequipSlot(st.LocationData) == nil {
			return Result{}, false
		}
		return Result{EquipmentChanged: true}, true
	}
	if len(inv.EquipItem(inst, tmpl)) == 0 {
		return Result{}, false
	}
	return Result{EquipmentChanged: true}, true
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
	if inv.UnequipSlot(paperdollSlot) == nil {
		return Result{}, false
	}
	return Result{EquipmentChanged: true}, true
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
	wasEquipped := (st.Location == item.LocationPaperdoll || st.Location == item.LocationPetEquip) && st.Count <= count
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

// PickupGround moves ground (with its loaded template) into inv, the same
// way any other incoming item would merge into an existing stack or take a
// free slot. pickerID is compared against ground.OwnerID to enforce a loot
// lock; an unowned ground item (OwnerID == 0) is free for anyone.
func (s *Service) PickupGround(inv *itemcontainer.Inventory, ground *item.Instance, tmpl *item.Template, pickerID int32) (Result, PickupFailure) {
	groundState := ground.Snapshot()
	if inv == nil || ground == nil || tmpl == nil || groundState.Count <= 0 {
		return Result{}, PickupNoop
	}
	if groundState.OwnerID != 0 && groundState.OwnerID != pickerID {
		return Result{}, PickupLootLocked
	}
	if !inv.ValidateCapacity(inv.SlotsNeededFor(ground, tmpl)) {
		return Result{}, PickupSlotsFull
	}

	picked := groundState.Instance()
	result, absorbed := inv.Add(picked)
	if result == nil {
		return Result{}, PickupNoop
	}
	if absorbed {
		return Result{Persist: []Persist{Update(result), Delete(ground.ObjectID)}}, PickupOK
	}
	return Result{Persist: []Persist{Save(result)}}, PickupOK
}

// DestroyItem consumes count units from inv.
func (s *Service) DestroyItem(inv *itemcontainer.Inventory, objectID int32, count int) (Result, bool) {
	if inv == nil || count <= 0 {
		return Result{}, false
	}
	inst := inv.ItemByObjectID(objectID)
	if inst == nil {
		return Result{}, false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	st := inst.Snapshot()
	if !ok || !inst.Destroyable(tmpl) || tmpl.HeroItem() || st.Count < count {
		return Result{}, false
	}
	if !tmpl.Stackable && count > 1 {
		return Result{}, false
	}
	wasEquipped := (st.Location == item.LocationPaperdoll || st.Location == item.LocationPetEquip) && st.Count <= count
	if inv.DestroyItem(inst, count) == nil {
		return Result{}, false
	}
	return Result{EquipmentChanged: wasEquipped}, true
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
	if st.Count > count && targetStack == nil {
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
		out.Persist = append(out.Persist, Delete(freedObjectID))
	}
	if result.ObjectID == objectID || receiver.ItemByObjectID(result.ObjectID) == result && newObjectID == 0 {
		out.Persist = append(out.Persist, Update(result))
	} else {
		out.Persist = append(out.Persist, Save(result))
	}
	return out, true, nil
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
	wasEquipped := (st.Location == item.LocationPaperdoll || st.Location == item.LocationPetEquip) && st.Count <= count
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
