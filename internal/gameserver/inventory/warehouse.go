package inventory

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Store is a warehouse or freight container items move into and out of.
type Store interface {
	itemcontainer.Receiver
	OwnerID() int32
	ItemByObjectID(objectID int32) *item.Instance
	Transfer(objectID int32, count int, target itemcontainer.Receiver, newObjectID int32) (result *item.Instance, freedObjectID int32, freed bool)
}

// Holder is what the player holding an inventory is using its items for
// right now, which ties those items where they are.
type Holder struct {
	// PetCollar reports whether objectID is the control item of the
	// holder's pet in the world, alive or dead.
	PetCollar func(objectID int32) bool
	// MountCollar is the control item of the mount the holder rides, or 0.
	MountCollar int32
	// EnchantScroll is the enchant scroll the holder has selected, or 0.
	EnchantScroll int32
	// Casting reports a cast in flight; CastConsumeID is the item id that
	// cast consumes, or 0.
	Casting       bool
	CastConsumeID int32
}

func (h Holder) petCollar(objectID int32) bool {
	return h.PetCollar != nil && h.PetCollar(objectID)
}

// manipulable reports whether count units of inst may leave the holder's
// inventory: a positive count, at most one of an item that does not stack
// and at most the held count, and not an item the holder has in use. An
// augmented item stays put while a cast is in flight.
func (h Holder) manipulable(inst *item.Instance, tmpl *item.Template, count int) bool {
	if count < 1 || (count > 1 && !tmpl.Stackable) || count > inst.CountValue() {
		return false
	}
	if h.petCollar(inst.ObjectID) || (h.MountCollar != 0 && inst.ObjectID == h.MountCollar) {
		return false
	}
	if h.EnchantScroll != 0 && inst.ObjectID == h.EnchantScroll {
		return false
	}
	return !inst.Augmented() || !h.Casting
}

// available reports whether inst is free to be offered: not worn, not a
// quest item, not the pet's collar, the selected enchant scroll or what the
// cast in flight consumes, and tradable unless allowNonTradable.
func (h Holder) available(inst *item.Instance, tmpl *item.Template, allowNonTradable bool) bool {
	if inst.Equipped() || inst.QuestItem(tmpl) || h.petCollar(inst.ObjectID) {
		return false
	}
	if h.EnchantScroll != 0 && inst.ObjectID == h.EnchantScroll {
		return false
	}
	if h.Casting && h.CastConsumeID != 0 && inst.TemplateID == h.CastConsumeID {
		return false
	}
	return allowNonTradable || inst.Tradable(tmpl)
}

// DepositableItems lists the items of inv a warehouse deposit window offers,
// in inventory order: every available item, non-tradable ones only for a
// private warehouse, that the warehouse accepts.
func (s *Service) DepositableItems(inv *itemcontainer.Inventory, h Holder, private bool) []*item.Instance {
	if inv == nil {
		return nil
	}
	var out []*item.Instance
	for _, inst := range inv.Items() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if ok && h.available(inst, tmpl, private) && inst.Depositable(tmpl, private) {
			out = append(out, inst)
		}
	}
	return out
}

// SendableItems lists the items of inv a package can carry, in inventory
// order: every available tradable item.
func (s *Service) SendableItems(inv *itemcontainer.Inventory, h Holder) []*item.Instance {
	if inv == nil {
		return nil
	}
	var out []*item.Instance
	for _, inst := range inv.Items() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if ok && h.available(inst, tmpl, false) {
			out = append(out, inst)
		}
	}
	return out
}

// StoreOutcome is how a warehouse or freight transfer ended.
type StoreOutcome int

const (
	// StoreDropped refused the request with no answer, or stopped it part
	// way through the moves with no answer.
	StoreDropped StoreOutcome = iota
	// StoreOverCapacity would overfill the receiving warehouse or freight.
	StoreOverCapacity
	// StoreNotEnoughAdena could not pay the fee.
	StoreNotEnoughAdena
	// StoreSlotsFull would overfill the receiving inventory.
	StoreSlotsFull
	// StoreWeightExceeded would overload the receiving inventory.
	StoreWeightExceeded
	// StoreDone ran every move.
	StoreDone
)

// Deposit moves rows from inv into store for fee adena: a private
// warehouse when private, otherwise a public one (freight). Every row must
// name an item the holder may let go of, or nothing happens. fits reports
// whether the slots the rows need fit in store. The fee is taken once the
// rows fit, and only then are the items moved: a row the warehouse does not
// accept is skipped, its fee kept, and a row no longer held as checked
// stops the moves there.
//
// The adena left after the rows is counted in 32 bits and wraps on
// overflow.
func (s *Service) Deposit(inv *itemcontainer.Inventory, store Store, rows []Move, h Holder, private bool, fee int, fits func(slots int) bool) (Result, StoreOutcome, error) {
	if inv == nil || store == nil {
		return Result{}, StoreDropped, nil
	}
	adena := int32(inv.Adena())
	slots := 0
	for _, row := range rows {
		inst, tmpl := s.heldItem(inv, row, h)
		if inst == nil {
			return Result{}, StoreDropped, nil
		}
		slots += storeSlots(store, inst, tmpl, row.Count)
		if inst.TemplateID == item.AdenaID {
			adena -= int32(row.Count)
		}
	}
	if !fits(slots) {
		return Result{}, StoreOverCapacity, nil
	}
	if int64(adena) < int64(fee) || !reduceAdena(inv, fee) {
		return Result{}, StoreNotEnoughAdena, nil
	}
	var res Result
	for _, row := range rows {
		inst, tmpl := s.heldItem(inv, row, h)
		if inst == nil {
			return res, StoreDropped, nil
		}
		if !inst.Depositable(tmpl, private) || !h.available(inst, tmpl, private) {
			continue
		}
		if err := s.storeMove(&res, inv, store, inst, tmpl, row.Count); err != nil {
			return res, StoreDone, err
		}
	}
	return res, StoreDone, nil
}

// SendPackage sends rows from inv to the freight store of another of the
// holder's characters for fee adena. A row naming an item the holder may
// not let go of is left out, its fee still charged; a row naming a
// non-tradable or quest item refuses the whole package. fits reports
// whether the slots the rows need fit in store. A hero item is never sent,
// its fee kept, and a worn item is taken off first.
func (s *Service) SendPackage(inv *itemcontainer.Inventory, store Store, rows []Move, h Holder, fee int, fits func(slots int) bool) (Result, StoreOutcome, error) {
	if inv == nil || store == nil {
		return Result{}, StoreDropped, nil
	}
	adena := int32(inv.Adena())
	slots := 0
	valid := make([]bool, len(rows))
	for i, row := range rows {
		inst, tmpl := s.heldItem(inv, row, h)
		if inst == nil {
			continue
		}
		if !inst.Tradable(tmpl) || inst.QuestItem(tmpl) {
			return Result{}, StoreDropped, nil
		}
		valid[i] = true
		if inst.TemplateID == item.AdenaID {
			adena -= int32(row.Count)
		}
		slots += storeSlots(store, inst, tmpl, row.Count)
	}
	if !fits(slots) {
		return Result{}, StoreOverCapacity, nil
	}
	if int64(adena) < int64(fee) || !reduceAdena(inv, fee) {
		return Result{}, StoreNotEnoughAdena, nil
	}
	var res Result
	for i, row := range rows {
		if !valid[i] {
			continue
		}
		inst := inv.ItemByObjectID(row.ObjectID)
		if inst == nil {
			continue
		}
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok || tmpl.HeroItem() {
			continue
		}
		st := inst.Snapshot()
		if st.Equipped() && row.Count >= st.Count {
			changed := inv.UnequipItem(inst)
			res.Changed = append(res.Changed, changed...)
			res.EquipmentChanged = res.EquipmentChanged || len(changed) > 0
		}
		if err := s.storeMove(&res, inv, store, inst, tmpl, row.Count); err != nil {
			return res, StoreDone, err
		}
	}
	return res, StoreDone, nil
}

// Withdraw moves rows from store into inv. Every row must name an item
// store holds at least the row's count of, or nothing happens; the rows
// must fit in inv's free slots and weight limit. A row store no longer
// holds as checked is skipped.
//
// The weight is summed in 32 bits and wraps on overflow.
func (s *Service) Withdraw(store Store, inv *itemcontainer.Inventory, rows []Move) (Result, StoreOutcome, error) {
	if inv == nil || store == nil {
		return Result{}, StoreDropped, nil
	}
	var weight int32
	slots := 0
	for _, row := range rows {
		inst := store.ItemByObjectID(row.ObjectID)
		if inst == nil || inst.CountValue() < row.Count {
			return Result{}, StoreDropped, nil
		}
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			return Result{}, StoreDropped, nil
		}
		weight += int32(row.Count) * tmpl.Weight
		switch {
		case !tmpl.Stackable:
			slots += row.Count
		case inv.ItemByTemplateID(inst.TemplateID) == nil:
			slots++
		}
	}
	if !inv.ValidateCapacity(slots) {
		return Result{}, StoreSlotsFull, nil
	}
	if !inv.ValidateWeight(int(weight)) {
		return Result{}, StoreWeightExceeded, nil
	}
	var res Result
	for _, row := range rows {
		inst := store.ItemByObjectID(row.ObjectID)
		// A row of no units moves nothing.
		if inst == nil || inst.CountValue() < row.Count || row.Count < 1 {
			continue
		}
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			continue
		}
		var target *item.Instance
		if tmpl.Stackable {
			target = inv.ItemByTemplateID(inst.TemplateID)
		}
		if err := s.move(&res, store.OwnerID(), inst, row.Count, target, func(newID int32) (*item.Instance, int32, bool) {
			return store.Transfer(row.ObjectID, row.Count, inv, newID)
		}); err != nil {
			return res, StoreDone, err
		}
	}
	return res, StoreDone, nil
}

// heldItem returns the item of inv a deposit row names, with its template,
// when the holder may let row.Count of it go.
func (s *Service) heldItem(inv *itemcontainer.Inventory, row Move, h Holder) (*item.Instance, *item.Template) {
	inst := inv.ItemByObjectID(row.ObjectID)
	if inst == nil {
		return nil, nil
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || !h.manipulable(inst, tmpl, row.Count) {
		return nil, nil
	}
	return inst, tmpl
}

// storeSlots is how many slots count units of inst take in store: one per
// unit of an item that does not stack, one for a stack store does not hold
// yet, none for a stack it does.
func storeSlots(store Store, inst *item.Instance, tmpl *item.Template, count int) int {
	switch {
	case !tmpl.Stackable:
		return count
	case store.ItemByTemplateID(inst.TemplateID) == nil:
		return 1
	default:
		return 0
	}
}

// storeMove moves count units of inst from inv into store.
func (s *Service) storeMove(res *Result, inv *itemcontainer.Inventory, store Store, inst *item.Instance, tmpl *item.Template, count int) error {
	var target *item.Instance
	if tmpl.Stackable {
		target = store.ItemByTemplateID(inst.TemplateID)
	}
	return s.move(res, inv.OwnerID(), inst, count, target, func(newID int32) (*item.Instance, int32, bool) {
		return inv.TransferItem(inst.ObjectID, count, store, newID)
	})
}

// move runs transfer, which moves count units of inst out of the container
// sourceOwnerID owns, and records the rows it changed. target is the stack
// the units merge into, or nil. A move that splits inst or merges into
// target gets a new object id first.
func (s *Service) move(res *Result, sourceOwnerID int32, inst *item.Instance, count int, target *item.Instance, transfer func(newID int32) (*item.Instance, int32, bool)) error {
	newID := int32(0)
	if inst.CountValue() > count || target != nil {
		id, ok, err := s.nextID()
		if err != nil {
			return err
		}
		if !ok {
			return errNoIDAllocator
		}
		newID = id
	}
	moved, freedID, freed := transfer(newID)
	if moved == nil {
		return nil
	}
	if moved != inst && inst.CountValue() > 0 {
		res.Persist = append(res.Persist, Update(inst))
	}
	if freed {
		res.Persist = append(res.Persist, Delete(sourceOwnerID, freedID))
	}
	if newID != 0 && moved.ObjectID == newID {
		res.Persist = append(res.Persist, Save(moved))
	} else {
		res.Persist = append(res.Persist, Update(moved))
	}
	return nil
}

// reduceAdena takes count adena from inv, reporting false when inv holds
// less. Nothing is taken for a count below 1.
func reduceAdena(inv *itemcontainer.Inventory, count int) bool {
	if count > inv.Adena() {
		return false
	}
	if count > 0 {
		return inv.DestroyByTemplateID(item.AdenaID, count) != nil
	}
	return true
}
