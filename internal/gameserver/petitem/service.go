package petitem

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// GiveInteractionDistance is the maximum owner-pet distance for item transfers.
const GiveInteractionDistance = 150

type positioned interface {
	Position() (int, int, int)
}

// GiveFailure is the non-mutating reason a give-to-pet request failed.
type GiveFailure uint8

const (
	// GiveOK means the item was transferred.
	GiveOK GiveFailure = iota
	// GiveNoop means the request is invalid and should be ignored.
	GiveNoop
	// GiveItemNotForPets means the item is forbidden for pets.
	GiveItemNotForPets
	// GiveDeadPet means dead pets cannot receive items.
	GiveDeadPet
	// GiveTooFar means the owner is too far from the pet.
	GiveTooFar
	// GivePetCannotCarryMore means the pet lacks free inventory slots.
	GivePetCannotCarryMore
	// GivePetTooEncumbered means the pet lacks remaining weight capacity.
	GivePetTooEncumbered
)

// UseFailure is the non-mutating reason a pet-use-item request failed.
type UseFailure uint8

const (
	// UseOK means the item was equipped or unequipped.
	UseOK UseFailure = iota
	// UseNoop means the request is invalid and should be ignored.
	UseNoop
	// UseCannotBeUsed means the item cannot be used in the pet's current state.
	UseCannotBeUsed
	// UsePetCannotUseItem means the item is not valid pet equipment.
	UsePetCannotUseItem
)

// UseOutcome identifies the pet equipment mutation.
type UseOutcome uint8

const (
	// Equipped means the pet put the item on.
	Equipped UseOutcome = iota
	// Unequipped means the pet took the item off.
	Unequipped
)

// TransferResult carries item-store operations for a pet inventory transfer.
type TransferResult struct {
	Persist []inventory.Persist
}

// UseResult carries item-store operations and owner-visible item data for pet equipment changes.
type UseResult struct {
	Outcome UseOutcome
	ItemID  int32
	Persist []inventory.Persist
}

// Service performs pet inventory mutations without knowing how clients are notified.
type Service struct {
	inventory *inventory.Service
}

// NewService returns a pet item mutation service.
func NewService(ids inventory.IDAllocator) *Service {
	return &Service{inventory: inventory.NewService(ids)}
}

// GiveToPet validates and transfers one item from an owner inventory to a pet inventory.
func (s *Service) GiveToPet(playerInv, petInv *itemcontainer.Inventory, pet *summon.Actor, owner positioned, objectID int32, count int) (TransferResult, GiveFailure, error) {
	if playerInv == nil || petInv == nil || pet == nil || count <= 0 {
		return TransferResult{}, GiveNoop, nil
	}
	inst := playerInv.ItemByObjectID(objectID)
	if inst == nil || inst.Augmented() {
		return TransferResult{}, GiveNoop, nil
	}
	tmpl, ok := playerInv.Templates().Get(inst.TemplateID)
	if !ok {
		return TransferResult{}, GiveNoop, nil
	}
	if ForbiddenForPet(inst, tmpl) {
		return TransferResult{}, GiveItemNotForPets, nil
	}
	if pet.Dead() {
		return TransferResult{}, GiveDeadPet, nil
	}
	if !withinGiveRange(owner, pet) {
		return TransferResult{}, GiveTooFar, nil
	}
	if !petInv.ValidateCapacity(petInv.SlotsNeededFor(inst, tmpl)) {
		return TransferResult{}, GivePetCannotCarryMore, nil
	}
	if !petInv.ValidateWeight(int(tmpl.Weight) * count) {
		return TransferResult{}, GivePetTooEncumbered, nil
	}
	return s.transfer(playerInv, petInv, objectID, count)
}

func withinGiveRange(owner positioned, pet *summon.Actor) bool {
	if owner == nil || pet == nil {
		return false
	}
	ax, ay, az := owner.Position()
	bx, by, bz := pet.Position()
	return location.In3DRange(ax, ay, az, bx, by, bz, GiveInteractionDistance)
}

// GetFromPet transfers one item from a pet inventory to its owner's inventory.
func (s *Service) GetFromPet(petInv, playerInv *itemcontainer.Inventory, objectID int32, count int) (TransferResult, bool, error) {
	result, failure, err := s.transfer(petInv, playerInv, objectID, count)
	return result, failure == GiveOK, err
}

// UseItem equips or unequips one pet equipment item.
func UseItem(pet *summon.Actor, petInv *itemcontainer.Inventory, objectID int32, ownerDead bool) (UseResult, UseFailure) {
	if pet == nil || petInv == nil {
		return UseResult{}, UseNoop
	}
	inst := petInv.ItemByObjectID(objectID)
	if inst == nil {
		return UseResult{}, UseNoop
	}
	tmpl, ok := petInv.Templates().Get(inst.TemplateID)
	if !ok {
		return UseResult{}, UseNoop
	}
	if ownerDead || pet.Dead() {
		return UseResult{ItemID: inst.TemplateID}, UseCannotBeUsed
	}
	if !Equippable(tmpl) || !pet.CanWearPetItem(tmpl) {
		return UseResult{}, UsePetCannotUseItem
	}

	if inst.Equipped() {
		old := petInv.UnequipSlot(inst.LocationData)
		if old == nil {
			return UseResult{}, UseNoop
		}
		return UseResult{Outcome: Unequipped, ItemID: old.TemplateID, Persist: []inventory.Persist{inventory.Update(old)}}, UseOK
	}

	slot := itemcontainer.Chest
	if tmpl.Weapon != nil && tmpl.Weapon.Type == item.WeaponPet {
		slot = itemcontainer.RHand
	}
	old := petInv.SetPaperdollItem(slot, inst, tmpl)
	actions := []inventory.Persist{inventory.Update(inst)}
	if old != nil {
		actions = append([]inventory.Persist{inventory.Update(old)}, actions...)
	}
	return UseResult{Outcome: Equipped, ItemID: inst.TemplateID, Persist: actions}, UseOK
}

func (s *Service) transfer(from, to *itemcontainer.Inventory, objectID int32, count int) (TransferResult, GiveFailure, error) {
	if s == nil {
		s = NewService(nil)
	}
	res, ok, err := s.inventory.TransferItem(from, to, objectID, count)
	if err != nil {
		return TransferResult{}, GiveNoop, err
	}
	if !ok {
		return TransferResult{}, GiveNoop, nil
	}
	return TransferResult{Persist: res.Persist}, GiveOK, nil
}
