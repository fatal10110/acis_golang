package network

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
)

// Operator item edits: the debug listener's item API lists and changes a
// character's inventory and paperdoll through the owners the game itself
// uses, whether the character is in the world or not.
//
// A character in the world is changed on its own queue, through its live
// inventory, as a game master's commands change it: the client hears of it
// and the lazy item persistence writes it.
//
// A character out of the world is changed the way a login would restore it,
// without the session. The edit takes the same per-character claim a
// selection takes (selectingOwners), so no selection can restore the
// character while it runs (a selection meanwhile aborts silently, and an edit
// meeting a selection under way is refused as busy), and neither can an
// offline pet settle write its rows. Under the claim it waits for every save
// the character's last session queued, then runs as one job on the
// character's persistence lane: it reads the rows, resolves them against
// the held item writes as a login does (restoreRows), rebuilds an inventory,
// applies the edit, and writes the result the way a logout does
// (task.ItemInstances.FlushReleased). Every write of those rows is on that
// lane or is a held write the restore takes over, so nothing lands between
// the read and the write, and a write that fails stays held for the next
// login to restore, exactly as a failed logout flush does.

// itemEditWait bounds how long an edit waits to start on a character's queue
// or persistence lane. An edit that has not started by then never runs.
const itemEditWait = 10 * time.Second

// CharacterItems lists characterID's inventory and paperdoll items: a
// character in the world from its live inventory, any other from its rows
// resolved as a login would restore them. Failures are *inventory.EditError
// values but for storage errors.
func (l *GameClientLink) CharacterItems(ctx context.Context, characterID int32) (invops.CharacterItems, error) {
	return l.editItems(ctx, invops.ItemEdit{CharacterID: characterID})
}

// EditCharacterItems applies edit to its character's items and returns them
// as they stand after it; see CharacterItems. Every change is logged.
func (l *GameClientLink) EditCharacterItems(ctx context.Context, edit invops.ItemEdit) (invops.CharacterItems, error) {
	if edit.Op < invops.EditAdd || edit.Op > invops.EditUnequip {
		return invops.CharacterItems{}, &invops.EditError{Fault: invops.EditInvalid, Reason: "unknown edit"}
	}
	return l.editItems(ctx, edit)
}

// editItems runs edit, or lists the items when edit has no op, for the
// character in the world or out of it.
func (l *GameClientLink) editItems(ctx context.Context, edit invops.ItemEdit) (invops.CharacterItems, error) {
	id := edit.CharacterID
	if id <= 0 {
		return invops.CharacterItems{}, &invops.EditError{Fault: invops.EditInvalid, Reason: "character_id must be positive"}
	}
	ctx, cancel := context.WithTimeout(ctx, itemEditWait)
	defer cancel()
	if live, found, err := l.itemEditPlayer(id); found || err != nil {
		return l.editLiveItems(ctx, live, edit, err)
	}
	if !l.flow.selections.begin(id) {
		return invops.CharacterItems{}, busyEdit("character %d is logging in; retry", id)
	}
	// A selection that registered its player ended its claim after it; the
	// world is checked again under this claim, which no later selection can
	// pass until it ends.
	live, found, err := l.itemEditPlayer(id)
	if found || err != nil {
		l.flow.selections.end(id)
		return l.editLiveItems(ctx, live, edit, err)
	}
	defer l.flow.selections.end(id)
	return l.editStoredItems(ctx, edit)
}

// itemEditPlayer finds characterID in the world.
func (l *GameClientLink) itemEditPlayer(characterID int32) (*livePlayer, bool, error) {
	if l.world == nil {
		return nil, false, nil
	}
	obj, ok := l.world.Player(characterID)
	if !ok {
		return nil, false, nil
	}
	live, ok := obj.(*livePlayer)
	if !ok {
		return nil, true, busyEdit("character %d is in the world without a session", characterID)
	}
	return live, true, nil
}

func busyEdit(format string, args ...any) *invops.EditError {
	return &invops.EditError{Fault: invops.EditBusy, Reason: fmt.Sprintf(format, args...)}
}

// editLiveItems runs edit on live's queue through its live inventory.
func (l *GameClientLink) editLiveItems(ctx context.Context, live *livePlayer, edit invops.ItemEdit, found error) (invops.CharacterItems, error) {
	if found != nil {
		return invops.CharacterItems{}, found
	}
	var out invops.CharacterItems
	err := errors.New("item edit did not complete")
	ran, waitErr := runUnlessAbandoned(ctx, func(fn func()) bool { return postLive(live, fn) }, func() {
		inv := live.Inventory()
		if live.detached() || inv == nil {
			err = busyEdit("character %d is leaving the world; retry", live.ObjectID())
			return
		}
		if edit.Op != 0 {
			if err = l.applyLiveEdit(live, inv, edit); err != nil {
				return
			}
		}
		out, err = characterItems(live.ObjectID(), live.CharacterName(), true, inv), nil
	})
	if !ran {
		if waitErr != nil {
			return invops.CharacterItems{}, busyEdit("character %d did not answer in time; nothing changed", live.ObjectID())
		}
		return invops.CharacterItems{}, busyEdit("character %d is leaving the world; retry", live.ObjectID())
	}
	return out, err
}

// applyLiveEdit applies edit to live's inventory on live's queue, the way
// the game master commands and the player's own requests change it.
func (l *GameClientLink) applyLiveEdit(live *livePlayer, inv *itemcontainer.Inventory, edit invops.ItemEdit) error {
	name := live.CharacterName()
	if edit.Op == invops.EditAdd {
		tmpl, err := l.checkItemAdd(inv, edit)
		if err != nil {
			return err
		}
		if !live.RewardItemFits(edit.ItemID, edit.Count) {
			return &invops.EditError{Fault: invops.EditRefused, Reason: "the inventory is full"}
		}
		added, err := l.addEditItems(inv, tmpl, edit, name, true)
		if len(added) > 0 {
			units := edit.Count
			if !tmpl.Stackable {
				units = len(added)
			}
			live.ItemAdded(event.ItemObtained{ItemID: edit.ItemID, Count: units, Notice: event.ObtainCreated})
		}
		return err
	}
	inst, tmpl, err := invops.CheckExpected(inv, edit)
	if err != nil {
		return err
	}
	if err := l.checkItemChange(inv, inst, tmpl, edit, live.CursedWeaponEquipped()); err != nil {
		return err
	}
	// A summoned pet's collar is not destroyed under it.
	if edit.Op == invops.EditDelete && live.ControlItemInUse(inst.ObjectID) {
		return &invops.EditError{Fault: invops.EditRefused, Reason: fmt.Sprintf("item %d controls the summoned pet", inst.ObjectID)}
	}
	before := inst.Snapshot()
	switch edit.Op {
	case invops.EditSet:
		if edit.SetCount {
			inv.SetCount(inst, edit.Count)
		}
		if edit.SetEnchant && edit.Enchant != before.EnchantLevel {
			l.setLiveEnchant(live, inv, inst, before, edit.Enchant)
		}
	case invops.EditDelete:
		res, ok := invops.RemoveItem(inv, inst)
		if !ok {
			return &invops.EditError{Fault: invops.EditStale, Reason: fmt.Sprintf("item %d changed; nothing removed", before.ObjectID)}
		}
		if res.EquipmentChanged {
			l.applyEquipStatChanges(live, inv, res)
			l.broadcastEquipmentChange(live)
		}
	case invops.EditEquip, invops.EditUnequip:
		if edit.Op == invops.EditEquip && tmpl.HeroItem() && (l.heroes == nil || !l.heroes.IsActive(live.ObjectID())) {
			return &invops.EditError{Fault: invops.EditRefused, Reason: "a hero item goes on an active hero only"}
		}
		l.toggleEquipItem(live, inv, inst, tmpl, false)
		if inst.Equipped() != (edit.Op == invops.EditEquip) {
			return &invops.EditError{Fault: invops.EditRefused, Reason: fmt.Sprintf("the paperdoll refused item %d", before.ObjectID)}
		}
	}
	l.logItemEdit(edit, name, true, before, inst.Snapshot())
	return nil
}

// setLiveEnchant gives inst, held by live, the enchant level level: a worn
// item as //enchant sets it, with the skills the level crossing grants or
// takes and the equipment broadcast, any other through the inventory.
func (l *GameClientLink) setLiveEnchant(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance, before item.InstanceState, level int) {
	if !before.Equipped() || l.belongings.itemInstances == nil {
		inv.SetEnchantLevel(inst, level)
		return
	}
	end := l.belongings.itemInstances.BeginOperation(inv.OwnerID())
	result := l.enchanting.items.SetLevel(inv, before.LocationData, level)
	l.applyPersistActions(result.Persist)
	end()
	l.applyEnchantSteps(live, result.Steps)
}

// checkItemAdd checks an add, refusing a cursed weapon too: one comes into
// being only through its own lifecycle.
func (l *GameClientLink) checkItemAdd(inv *itemcontainer.Inventory, edit invops.ItemEdit) (*item.Template, error) {
	if l.cursed != nil && l.cursed.IsCursed(edit.ItemID) {
		return nil, &invops.EditError{Fault: invops.EditInvalid, Reason: fmt.Sprintf("item %d is a cursed weapon", edit.ItemID)}
	}
	return invops.CheckAdd(inv, edit.ItemID, edit.Count, edit.Enchant)
}

// checkItemChange refuses a change the game itself never makes: a cursed
// weapon follows its own lifecycle and is not set, taken off, put on or
// destroyed by request, and a holder of one (holder) keeps it in hand, so no
// other weapon or shield, nor formal wear, goes on.
func (l *GameClientLink) checkItemChange(inv *itemcontainer.Inventory, inst *item.Instance, tmpl *item.Template, edit invops.ItemEdit, holder bool) error {
	if l.cursedItem(inv, inst.ObjectID) {
		return &invops.EditError{Fault: invops.EditRefused, Reason: fmt.Sprintf("item %d is a cursed weapon", inst.ObjectID)}
	}
	if edit.Op != invops.EditEquip || !holder || inst.Equipped() {
		return nil
	}
	switch tmpl.Slot {
	case item.SlotLRHand, item.SlotLHand, item.SlotRHand:
	default:
		if inst.TemplateID != formalWearID {
			return nil
		}
	}
	return &invops.EditError{Fault: invops.EditRefused, Reason: "a cursed weapon's holder keeps it in hand"}
}

// holdsCursedWeapon reports whether inv, an offline character's inventory,
// wears a cursed weapon.
func (l *GameClientLink) holdsCursedWeapon(inv *itemcontainer.Inventory) bool {
	for _, inst := range inv.Items() {
		if inst.Equipped() && l.cursedItem(inv, inst.ObjectID) {
			return true
		}
	}
	return false
}

// addEditItems adds an edit's units to inv, logging each instance added to
// or created.
func (l *GameClientLink) addEditItems(inv *itemcontainer.Inventory, tmpl *item.Template, edit invops.ItemEdit, name string, online bool) ([]*item.Instance, error) {
	var held item.InstanceState
	if tmpl.Stackable {
		if inst := inv.ItemByTemplateID(tmpl.ID); inst != nil {
			held = inst.Snapshot()
		}
	}
	added, err := invops.AddItems(inv, tmpl, edit.Count, edit.Enchant, l.nextObjectID)
	for _, inst := range added {
		before := held
		if before.ObjectID != inst.ObjectID {
			before = item.InstanceState{}
		}
		l.logItemEdit(edit, name, online, before, inst.Snapshot())
	}
	if err != nil {
		l.log.Error().Err(err).Int32("character_id", edit.CharacterID).Int32("item_id", edit.ItemID).Msg("item api: allocate item id")
		return added, fmt.Errorf("allocate item id: %w", err)
	}
	return added, nil
}

// editStoredItems runs edit on the stored items of a character out of the
// world. The caller holds the character's selection claim.
func (l *GameClientLink) editStoredItems(ctx context.Context, edit invops.ItemEdit) (invops.CharacterItems, error) {
	id := edit.CharacterID
	if l.roster == nil || l.belongings.items == nil || l.belongings.itemInstances == nil {
		return invops.CharacterItems{}, errors.New("item api: no character or item persistence")
	}
	// Every save the last session queued, and every write those lanes owe,
	// lands before the rows are read.
	if err := l.persist.Flush(ctx, id); err != nil {
		return invops.CharacterItems{}, busyEdit("character %d still has saves queued; retry", id)
	}
	var out invops.CharacterItems
	err := errors.New("item edit did not complete")
	ran, _ := runUnlessAbandoned(ctx, func(fn func()) bool { return l.persist.Enqueue(id, fn) }, func() {
		out, err = l.applyStoredEdit(edit)
	})
	if !ran {
		return invops.CharacterItems{}, busyEdit("character %d's saves did not finish in time; nothing changed", id)
	}
	return out, err
}

// applyStoredEdit is editStoredItems' job on the character's persistence
// lane: read, resolve, rebuild, apply, write. The character row is read on
// the lane too, so a deletion purging the character, which waits for the
// lanes, lands wholly before the job or wholly after its write.
func (l *GameClientLink) applyStoredEdit(edit invops.ItemEdit) (invops.CharacterItems, error) {
	id := edit.CharacterID
	ctx, cancel := context.WithTimeout(context.Background(), livePlayerDetachSaveTimeout)
	defer cancel()
	c, err := l.roster.Load(ctx, id)
	if errors.Is(err, gamesql.ErrCharacterNotFound) {
		return invops.CharacterItems{}, &invops.EditError{Fault: invops.EditNoCharacter, Reason: fmt.Sprintf("no character %d", id)}
	}
	if err != nil {
		return invops.CharacterItems{}, fmt.Errorf("load character %d: %w", id, err)
	}
	name := c.CharacterName()
	rows, err := l.belongings.items.ListByOwner(ctx, id)
	if err != nil {
		return invops.CharacterItems{}, fmt.Errorf("list items of character %d: %w", id, err)
	}
	carried := func(loc item.Location) bool {
		return loc == item.LocationInventory || loc == item.LocationPaperdoll
	}
	rows = slices.DeleteFunc(rows, func(inst *item.Instance) bool { return inst == nil || !carried(inst.Location) })
	inv := itemcontainer.NewPlayerInventoryWithDelivery(id, l.itemTemplates, nil, l.itemPersister(id))
	inv.Restore(l.restoreRows(id, rows, carried))
	removed, changeErr := l.applyStoredChange(inv, name, edit)
	// What the restore merged, or the edit changed and did not write, stays
	// held for the item persistence task, as a logout leaves it.
	inv.ReleasePersistence()
	if edit.Op != 0 && changeErr == nil {
		if err := l.belongings.itemInstances.FlushReleased(ctx, append(inv.Items(), removed...)); err != nil {
			l.log.Error().Err(err).Int32("character_id", id).Msg("item api: write items")
			return invops.CharacterItems{}, &invops.EditError{
				Fault:  invops.EditFailed,
				Reason: "the item write failed; the change stays queued for the item persistence task",
			}
		}
	}
	if changeErr != nil {
		return invops.CharacterItems{}, changeErr
	}
	return characterItems(id, name, false, inv), nil
}

// applyStoredChange applies edit to inv, an offline character's rebuilt
// inventory, under the paperdoll's own equip rules, and returns the items it
// removed.
func (l *GameClientLink) applyStoredChange(inv *itemcontainer.Inventory, name string, edit invops.ItemEdit) ([]*item.Instance, error) {
	switch edit.Op {
	case 0:
		return nil, nil
	case invops.EditAdd:
		tmpl, err := l.checkItemAdd(inv, edit)
		if err != nil {
			return nil, err
		}
		_, err = l.addEditItems(inv, tmpl, edit, name, false)
		return nil, err
	}
	inst, tmpl, err := invops.CheckExpected(inv, edit)
	if err != nil {
		return nil, err
	}
	if err := l.checkItemChange(inv, inst, tmpl, edit, l.holdsCursedWeapon(inv)); err != nil {
		return nil, err
	}
	before := inst.Snapshot()
	var removed []*item.Instance
	switch edit.Op {
	case invops.EditSet:
		if edit.SetCount {
			inv.SetCount(inst, edit.Count)
		}
		if edit.SetEnchant {
			inv.SetEnchantLevel(inst, edit.Enchant)
		}
	case invops.EditDelete:
		if _, ok := invops.RemoveItem(inv, inst); !ok {
			return nil, &invops.EditError{Fault: invops.EditStale, Reason: fmt.Sprintf("item %d changed; nothing removed", before.ObjectID)}
		}
		removed = append(removed, inst)
	case invops.EditEquip:
		_, failure := l.inventory.ToggleEquipItem(inv, inst.ObjectID, l.heroes != nil && l.heroes.IsActive(edit.CharacterID))
		if failure != invops.EquipOK {
			return nil, &invops.EditError{Fault: invops.EditRefused, Reason: fmt.Sprintf("the paperdoll refused item %d", before.ObjectID)}
		}
	case invops.EditUnequip:
		inv.UnequipItem(inst)
	}
	if tmpl != nil {
		l.logItemEdit(edit, name, false, before, inst.Snapshot())
	}
	return removed, nil
}

// logItemEdit logs one item's change by an edit.
func (l *GameClientLink) logItemEdit(edit invops.ItemEdit, name string, online bool, before, after item.InstanceState) {
	l.log.Info().Int32("character_id", edit.CharacterID).Str("character", name).Bool("online", online).
		Stringer("op", edit.Op).Int32("object_id", cmp.Or(after.ObjectID, before.ObjectID)).
		Int32("item_id", cmp.Or(after.TemplateID, before.TemplateID)).
		Int("before_count", before.Count).Int("before_enchant", before.EnchantLevel).Stringer("before_location", before.Location).
		Int("after_count", after.Count).Int("after_enchant", after.EnchantLevel).Stringer("after_location", after.Location).
		Msg("item api: item changed")
}

// characterItems lists inv's items by object id.
func characterItems(id int32, name string, online bool, inv *itemcontainer.Inventory) invops.CharacterItems {
	held := inv.Items()
	states := make([]item.InstanceState, 0, len(held))
	for _, inst := range held {
		states = append(states, inst.Snapshot())
	}
	slices.SortFunc(states, func(a, b item.InstanceState) int { return cmp.Compare(a.ObjectID, b.ObjectID) })
	return invops.CharacterItems{CharacterID: id, Name: name, Online: online, Items: states}
}

// runUnlessAbandoned hands fn to post and waits until it has run or ctx
// ends. A fn that has not started when ctx ends never runs, so a caller that
// gave up changed nothing; one already running is waited out. ran reports
// whether fn ran; err is ctx's error when the wait gave up first.
func runUnlessAbandoned(ctx context.Context, post func(func()) bool, fn func()) (ran bool, err error) {
	const waiting, started, abandoned = 0, 1, 2
	var state atomic.Int32
	done := make(chan struct{})
	if !post(func() {
		if !state.CompareAndSwap(waiting, started) {
			return
		}
		defer close(done)
		fn()
	}) {
		return false, nil
	}
	select {
	case <-done:
		return true, nil
	case <-ctx.Done():
		if state.CompareAndSwap(waiting, abandoned) {
			return false, ctx.Err()
		}
		<-done
		return true, nil
	}
}
