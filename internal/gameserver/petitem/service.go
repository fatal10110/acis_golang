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
	// GivePetCannotWear means the pet cannot wear the equipment item its
	// owner uses on it.
	GivePetCannotWear
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
	// UsePetCannotUseItem means the item is neither valid pet equipment nor
	// an eligible consumable (food the pet's template accepts, or a potion).
	UsePetCannotUseItem
	// UseConsumable means the item is not pet equipment but is an eligible
	// consumable (food or potion): the caller dispatches it to the item's
	// etc-item handler (network.usePetConsumable) rather than mutating the
	// paperdoll itself.
	UseConsumable
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
	// WasWorn reports whether the transferred item was equipped on the pet
	// before the transfer (set only by GetFromPet).
	WasWorn bool
	// ItemID is the transferred item's template id, valid whenever WasWorn is true.
	ItemID int32
	// Unequipped lists the instances whose paperdoll slots changed as the
	// transferred item came off its giver (set only by Transfer).
	Unequipped []*item.Instance
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
	if failure := s.CheckGive(playerInv, petInv, pet, owner, objectID, count); failure != GiveOK {
		return TransferResult{}, failure, nil
	}
	return s.transfer(playerInv, petInv, objectID, count)
}

// CheckGive runs the non-mutating precondition checks for a give-to-pet
// request without performing the transfer. Split out from GiveToPet so
// callers can cancel an active enchant selection between validation and
// mutation: the selection is cancelled after the preconditions pass but
// before the item is transferred.
func (s *Service) CheckGive(playerInv, petInv *itemcontainer.Inventory, pet *summon.Actor, owner positioned, objectID int32, count int) GiveFailure {
	if playerInv == nil || petInv == nil || pet == nil || count <= 0 {
		return GiveNoop
	}
	inst := playerInv.ItemByObjectID(objectID)
	if inst == nil || inst.Augmented() {
		return GiveNoop
	}
	tmpl, ok := playerInv.Templates().Get(inst.TemplateID)
	if !ok {
		return GiveNoop
	}
	if ForbiddenForPet(inst, tmpl) {
		return GiveItemNotForPets
	}
	if pet.Dead() {
		return GiveDeadPet
	}
	if !withinGiveRange(owner, pet) {
		return GiveTooFar
	}
	if !petInv.ValidateCapacity(petInv.SlotsNeededFor(inst, tmpl)) {
		return GivePetCannotCarryMore
	}
	if !petInv.ValidateWeight(int(tmpl.Weight) * count) {
		return GivePetTooEncumbered
	}
	return GiveOK
}

// CheckWear runs the checks an owner's use of a pet equipment item from
// their own inventory goes through before the item moves to the pet, in
// their order: the pet must be able to wear it, be alive, and have a free
// slot and the weight to spare for one unit. Unlike a give, nothing checks
// the distance to the pet or whether the item may be handed to a pet.
func CheckWear(petInv *itemcontainer.Inventory, pet *summon.Actor, inst *item.Instance, tmpl *item.Template) GiveFailure {
	if petInv == nil || pet == nil || inst == nil || tmpl == nil {
		return GiveNoop
	}
	if !pet.CanWearPetItem(tmpl) {
		return GivePetCannotWear
	}
	if pet.Dead() {
		return GiveDeadPet
	}
	if !petInv.ValidateCapacity(petInv.SlotsNeededFor(inst, tmpl)) {
		return GivePetCannotCarryMore
	}
	if !petInv.ValidateWeight(int(tmpl.Weight)) {
		return GivePetTooEncumbered
	}
	return GiveOK
}

// Transfer moves one item between inventories, exported so handlers can
// perform the mutation after their own between-validation-and-mutation
// side effects (e.g. cancelling an active enchant selection).
//
// A worn item comes off first when count moves the whole of it and is one
// the transfer can move; part of a worn stack leaves the rest worn.
// Unequipped then lists every instance whose slots changed, for the caller
// to take their equipment effects off the giver.
func (s *Service) Transfer(from, to *itemcontainer.Inventory, objectID int32, count int) (TransferResult, bool, error) {
	var unequipped []*item.Instance
	if validTransferCount(from, objectID, count) {
		if inst := from.ItemByObjectID(objectID); inst.Equipped() && count >= inst.Snapshot().Count {
			unequipped = from.UnequipItem(inst)
		}
	}
	result, failure, err := s.transfer(from, to, objectID, count)
	result.Unequipped = unequipped
	return result, failure == GiveOK, err
}

func withinGiveRange(owner positioned, pet *summon.Actor) bool {
	if owner == nil || pet == nil {
		return false
	}
	ax, ay, az := owner.Position()
	bx, by, bz := pet.Position()
	return location.In3DRadius(ax, ay, az, bx, by, bz, GiveInteractionDistance)
}

// GetFromPet transfers one item from a pet inventory to its owner's
// inventory. A worn item is taken off as it leaves the pet's inventory, inside
// the transfer, so a failed transfer leaves the pet's equip state untouched.
// The reported WasWorn, read before the transfer, drives the PET_TOOK_OFF_S1
// notification.
func (s *Service) GetFromPet(petInv, playerInv *itemcontainer.Inventory, objectID int32, count int) (TransferResult, bool, error) {
	wasWorn := false
	itemID := int32(0)
	if petInv != nil {
		if inst := petInv.ItemByObjectID(objectID); inst != nil {
			if st := inst.Snapshot(); st.Equipped() {
				wasWorn = true
				itemID = st.TemplateID
			}
		}
	}
	result, failure, err := s.transfer(petInv, playerInv, objectID, count)
	if failure != GiveOK {
		return result, false, err
	}
	result.WasWorn = wasWorn
	result.ItemID = itemID
	return result, true, err
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

	st := inst.Snapshot()
	// Both the equip and consumable-dispatch branches below are gated on
	// the item's use conditions, checked against the pet, for a
	// not-yet-equipped item. A failed condition sends PET_CANNOT_USE_ITEM
	// to the pet's owner rather than staying silent, so this reuses
	// UsePetCannotUseItem rather than a bare no-op. A test that aborts is
	// the one silent refusal: the check fails hard before anything is
	// sent, and a pet item use leaves the client waiting on nothing.
	if !st.Equipped() {
		held, aborted := checkUseConditions(pet, tmpl.UseConditions)
		if aborted {
			return UseResult{}, UseNoop
		}
		if !held {
			return UseResult{ItemID: inst.TemplateID}, UsePetCannotUseItem
		}
	}

	if !Equippable(tmpl) {
		if tmpl.EtcItem == nil || tmpl.EtcItem.Handler == "" {
			return UseResult{}, UsePetCannotUseItem
		}
		if !pet.CanEatFood(inst.TemplateID) && tmpl.EtcItem.Type != item.EtcItemPotion {
			return UseResult{}, UsePetCannotUseItem
		}
		return UseResult{ItemID: inst.TemplateID}, UseConsumable
	}
	if !pet.CanWearPetItem(tmpl) {
		return UseResult{}, UsePetCannotUseItem
	}
	res := wear(petInv, inst, tmpl)
	if res.ItemID == 0 {
		return UseResult{}, UseNoop
	}
	return res, UseOK
}

// Wear puts the pet equipment item objectID, already in petInv, on the pet,
// or takes it off when it is worn, and reports false when petInv holds no
// such item.
func Wear(petInv *itemcontainer.Inventory, objectID int32) (UseResult, bool) {
	if petInv == nil {
		return UseResult{}, false
	}
	inst := petInv.ItemByObjectID(objectID)
	if inst == nil {
		return UseResult{}, false
	}
	tmpl, ok := petInv.Templates().Get(inst.TemplateID)
	if !ok || !Equippable(tmpl) {
		return UseResult{}, false
	}
	res := wear(petInv, inst, tmpl)
	return res, res.ItemID != 0
}

// wear takes the worn inst off, or puts it on in its slot (a pet weapon in
// the right hand, pet armor on the chest), replacing what is worn there.
// A worn item whose slot is already empty yields a zero result.
func wear(petInv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template) UseResult {
	st := inst.Snapshot()
	if st.Equipped() {
		old := petInv.UnequipSlot(st.LocationData)
		if old == nil {
			return UseResult{}
		}
		return UseResult{Outcome: Unequipped, ItemID: old.TemplateID, Persist: []inventory.Persist{inventory.Update(old)}}
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
	return UseResult{Outcome: Equipped, ItemID: inst.TemplateID, Persist: actions}
}

// transfer moves count units of objectID from one inventory to the other.
// Unlike the shared inventory transfer, which moves at most what is there,
// a count the item cannot satisfy moves nothing: below one, several units
// of an unstackable item, or more than the stack holds.
func (s *Service) transfer(from, to *itemcontainer.Inventory, objectID int32, count int) (TransferResult, GiveFailure, error) {
	if s == nil {
		s = NewService(nil)
	}
	if !validTransferCount(from, objectID, count) {
		return TransferResult{}, GiveNoop, nil
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

func validTransferCount(from *itemcontainer.Inventory, objectID int32, count int) bool {
	if from == nil || count < 1 {
		return false
	}
	inst := from.ItemByObjectID(objectID)
	if inst == nil {
		return false
	}
	tmpl, ok := from.Templates().Get(inst.TemplateID)
	if !ok || (count > 1 && !tmpl.Stackable) {
		return false
	}
	return count <= inst.Snapshot().Count
}
