package inventory

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Operator edits of a character's inventory and paperdoll, made from outside
// the game client.

const (
	// MaxEditInstances caps the instances one add of a non-stackable item
	// creates.
	MaxEditInstances = 100
	// MaxEditEnchant is the highest enchant level an edit sets.
	MaxEditEnchant = 65535
)

// EditOp is what an ItemEdit does.
type EditOp uint8

const (
	// EditAdd creates Count units of ItemID at Enchant.
	EditAdd EditOp = iota + 1
	// EditSet changes the count and/or the enchant level of ObjectID.
	EditSet
	// EditDelete removes ObjectID whole, taking it off first when worn.
	EditDelete
	// EditEquip puts ObjectID on.
	EditEquip
	// EditUnequip takes ObjectID off.
	EditUnequip
)

// String names op in logs.
func (op EditOp) String() string {
	switch op {
	case EditAdd:
		return "add"
	case EditSet:
		return "edit"
	case EditDelete:
		return "delete"
	case EditEquip:
		return "equip"
	case EditUnequip:
		return "unequip"
	}
	return fmt.Sprintf("EditOp(%d)", uint8(op))
}

// ItemExpect is what the caller last saw of the item it edits: an edit
// whose item no longer matches it is stale and changes nothing.
type ItemExpect struct {
	ItemID   int32
	Count    int
	Enchant  int
	Location item.Location
}

// ItemEdit is one edit of a character's items. EditAdd reads ItemID, Count
// and Enchant; every other op names ObjectID and Expect, and EditSet also
// the new Count when SetCount and the new Enchant when SetEnchant.
type ItemEdit struct {
	Op          EditOp
	CharacterID int32
	ItemID      int32
	Count       int
	Enchant     int
	ObjectID    int32
	Expect      ItemExpect
	SetCount    bool
	SetEnchant  bool
}

// CharacterItems is a character's inventory and paperdoll items, ordered by
// object id, and whether the character is in the world.
type CharacterItems struct {
	CharacterID int32
	Name        string
	Online      bool
	Items       []item.InstanceState
}

// EditFault classifies why an edit or listing failed.
type EditFault uint8

const (
	// EditInvalid is a request that can never succeed as given.
	EditInvalid EditFault = iota + 1
	// EditNoCharacter names no stored character.
	EditNoCharacter
	// EditNoItem names an object id the inventory does not hold.
	EditNoItem
	// EditStale is an item that no longer matches the caller's Expect.
	EditStale
	// EditRefused is a change the paperdoll or the character refuses.
	EditRefused
	// EditBusy is a character a login, a logout or another edit is
	// handling: retrying later may succeed.
	EditBusy
	// EditFailed is a change applied in memory whose write did not land;
	// it stays queued for the item persistence task.
	EditFailed
)

// EditError is an edit or listing that failed for Fault.
type EditError struct {
	Fault  EditFault
	Reason string
}

func (e *EditError) Error() string { return e.Reason }

func editErr(fault EditFault, format string, args ...any) *EditError {
	return &EditError{Fault: fault, Reason: fmt.Sprintf(format, args...)}
}

// CheckAdd returns the template of an add of count units of itemID at
// enchant to inv, or why the add cannot go in: an unknown item or a herb,
// which is used on pickup and never carried; a count below one, above
// MaxEditInstances for an item that does not stack, or taking the held stack
// past the int32 limit; an enchant out of [0, MaxEditEnchant], or other than
// zero for a stackable item.
func CheckAdd(inv *itemcontainer.Inventory, itemID int32, count, enchant int) (*item.Template, error) {
	tmpl, ok := inv.Templates().Get(itemID)
	if !ok {
		return nil, editErr(EditInvalid, "item %d does not exist", itemID)
	}
	if tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemHerb {
		return nil, editErr(EditInvalid, "item %d is a herb, which is never carried", itemID)
	}
	if count < 1 || count > math.MaxInt32 {
		return nil, editErr(EditInvalid, "count must be 1 to %d", math.MaxInt32)
	}
	if enchant < 0 || enchant > MaxEditEnchant {
		return nil, editErr(EditInvalid, "enchant must be 0 to %d", MaxEditEnchant)
	}
	if !tmpl.Stackable {
		if count > MaxEditInstances {
			return nil, editErr(EditInvalid, "item %d does not stack: at most %d per request", itemID, MaxEditInstances)
		}
		return tmpl, nil
	}
	if enchant != 0 {
		return nil, editErr(EditInvalid, "item %d stacks and takes no enchant", itemID)
	}
	if held := inv.ItemByTemplateID(itemID); held != nil && held.CountValue() > math.MaxInt32-count {
		return nil, editErr(EditInvalid, "the held stack of item %d would pass %d", itemID, math.MaxInt32)
	}
	return tmpl, nil
}

// AddItems adds count units of tmpl to inv, as CheckAdd passed them: a
// stackable item joins the held stack or starts one, any other item arrives
// as count instances at enchant. A shadow item starts with its full
// duration. nextID allocates the id of each instance created. It returns the
// instances added to or created, and nextID's error when an id could not be
// taken, with the instances created before it.
func AddItems(inv *itemcontainer.Inventory, tmpl *item.Template, count, enchant int, nextID func() (int32, error)) ([]*item.Instance, error) {
	instances := count
	if tmpl.Stackable {
		instances = 1
	}
	// CheckAdd bounds count; the capacity is bounded again here so no
	// caller sizes an allocation by an unchecked count.
	capacity := instances
	if capacity > MaxEditInstances {
		capacity = MaxEditInstances
	}
	added := make([]*item.Instance, 0, capacity)
	for range instances {
		inst, err := inv.AddCreated(tmpl.ID, count, nextID)
		if err != nil {
			return added, err
		}
		if inst == nil {
			return added, fmt.Errorf("add item %d: not created", tmpl.ID)
		}
		if enchant > 0 {
			inv.SetEnchantLevel(inst, enchant)
		}
		added = append(added, inst)
	}
	return added, nil
}

// CheckExpected returns the item edit names in inv, with its template, when
// it is still what edit.Expect describes, and checks what edit asks of it.
func CheckExpected(inv *itemcontainer.Inventory, edit ItemEdit) (*item.Instance, *item.Template, error) {
	inst := inv.ItemByObjectID(edit.ObjectID)
	if inst == nil {
		return nil, nil, editErr(EditNoItem, "no item %d in the inventory or paperdoll", edit.ObjectID)
	}
	st := inst.Snapshot()
	want := edit.Expect
	if st.TemplateID != want.ItemID || st.Count != want.Count || st.EnchantLevel != want.Enchant || st.Location != want.Location {
		return nil, nil, editErr(EditStale, "item %d is item %d x%d +%d at %s, not item %d x%d +%d at %s",
			st.ObjectID, st.TemplateID, st.Count, st.EnchantLevel, st.Location, want.ItemID, want.Count, want.Enchant, want.Location)
	}
	tmpl, ok := inv.Templates().Get(st.TemplateID)
	if !ok {
		return nil, nil, editErr(EditInvalid, "item %d has no loaded template", st.TemplateID)
	}
	switch edit.Op {
	case EditSet:
		if !edit.SetCount && !edit.SetEnchant {
			return nil, nil, editErr(EditInvalid, "give count, enchant or both")
		}
		if edit.SetCount {
			if edit.Count < 1 || edit.Count > math.MaxInt32 {
				return nil, nil, editErr(EditInvalid, "count must be 1 to %d; delete the item instead", math.MaxInt32)
			}
			if !tmpl.Stackable && edit.Count != 1 {
				return nil, nil, editErr(EditInvalid, "item %d does not stack: count stays 1", st.TemplateID)
			}
		}
		if edit.SetEnchant {
			if edit.Enchant < 0 || edit.Enchant > MaxEditEnchant {
				return nil, nil, editErr(EditInvalid, "enchant must be 0 to %d", MaxEditEnchant)
			}
			if tmpl.Stackable && edit.Enchant != 0 {
				return nil, nil, editErr(EditInvalid, "item %d stacks and takes no enchant", st.TemplateID)
			}
		}
	case EditEquip:
		if st.Equipped() {
			return nil, nil, editErr(EditInvalid, "item %d is already worn", st.ObjectID)
		}
		if tmpl.Slot == item.SlotNone {
			return nil, nil, editErr(EditInvalid, "item %d is not equipment", st.TemplateID)
		}
	case EditUnequip:
		if !st.Equipped() {
			return nil, nil, editErr(EditInvalid, "item %d is not worn", st.ObjectID)
		}
	}
	return inst, tmpl, nil
}

// RemoveItem removes inst from inv whole, taking it off first when worn, so
// the result lists everything that came off the paperdoll with it: a bow's
// arrows or a rod's lure too.
func RemoveItem(inv *itemcontainer.Inventory, inst *item.Instance) (Result, bool) {
	st := inst.Snapshot()
	changed := unequipConsumed(inv, inst, st.Equipped())
	if inv.DestroyItem(inst, st.Count) == nil {
		return Result{}, false
	}
	return Result{EquipmentChanged: len(changed) > 0, Changed: changed}, true
}
