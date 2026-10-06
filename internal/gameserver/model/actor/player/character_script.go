package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// GiveScriptItems adds count units of itemID to c's inventory the way a
// script hands items over, with no slot or weight check: a stackable joins
// the held stack or starts one, and a non-stackable arrives as count
// instances, or as one unless multiple is set. A positive enchant is set on
// the instance added last. The chat line names count as asked for. nextID
// allocates each new instance's object id. A count below one, an unknown
// item and a character leaving the world get nothing.
func (c *Character) GiveScriptItems(itemID int32, count, enchant int, multiple bool, nextID func() (int32, error)) {
	c.addScriptItems(itemID, count, enchant, multiple, nextID, event.ObtainGiven)
}

// AddScriptItems is GiveScriptItems with no enchant, whose chat line reads
// as items picked up rather than earned.
func (c *Character) AddScriptItems(itemID int32, count int, multiple bool, nextID func() (int32, error)) {
	c.addScriptItems(itemID, count, 0, multiple, nextID, event.ObtainCreated)
}

func (c *Character) addScriptItems(itemID int32, count, enchant int, multiple bool, nextID func() (int32, error), notice event.ObtainNotice) {
	if count <= 0 || c.Detaching() || c.inventory == nil {
		return
	}
	tmpl, ok := c.inventory.Templates().Get(itemID)
	if !ok {
		return
	}
	instances := 1
	if !tmpl.Stackable && multiple {
		instances = count
	}
	var last *item.Instance
	for range instances {
		id, err := nextID()
		if err != nil {
			c.log.Error().Err(err).Int32("item_id", itemID).Msg("allocate script item id")
			break
		}
		inst := c.inventory.AddNew(itemID, count, id)
		if inst == nil {
			break
		}
		last = inst
	}
	if last == nil {
		return
	}
	if enchant > 0 {
		c.inventory.SetEnchantLevel(last, enchant)
	}
	c.emit(event.ItemObtained{ItemID: itemID, Count: count, Notice: notice})
}

// TakeScriptItems removes count units of itemID from c's inventory the way
// a script takes items, each removal named in chat. A negative count, or
// one above what c holds of a stackable, takes the whole stack; a
// non-stackable loses count instances, all of them for a negative count. A
// worn instance is taken off first. An unknown item, one c does not hold
// and a character leaving the world lose nothing.
func (c *Character) TakeScriptItems(itemID int32, count int) {
	if c.Detaching() || c.inventory == nil {
		return
	}
	tmpl, ok := c.inventory.Templates().Get(itemID)
	if !ok {
		return
	}
	if !tmpl.Stackable {
		removed := 0
		for _, inst := range c.inventory.ItemsByTemplateID(itemID) {
			if count >= 0 && removed == count {
				break
			}
			c.unequipForTake(inst)
			held := inst.CountValue()
			taken := c.inventory.DestroyItem(inst, held) != nil
			c.emit(event.ItemsTaken{ItemID: itemID, Count: held, Short: !taken})
			removed++
		}
		return
	}
	inst := c.inventory.ItemByTemplateID(itemID)
	if inst == nil {
		return
	}
	if held := inst.CountValue(); count < 0 || count > held {
		count = held
	}
	c.unequipForTake(inst)
	if count == 0 {
		// Nothing goes, yet any item but adena is still named.
		if itemID != item.AdenaID {
			c.emit(event.ItemsTaken{ItemID: itemID})
		}
		return
	}
	taken := c.inventory.DestroyItem(inst, count) != nil
	c.emit(event.ItemsTaken{ItemID: itemID, Count: count, Short: !taken})
}

// unequipForTake takes inst off when c wears it.
func (c *Character) unequipForTake(inst *item.Instance) {
	if inst.Equipped() {
		c.emit(event.UnequipRequested{ObjectID: inst.ObjectID})
	}
}

// DestroyScriptItems destroys count units of itemID, from the first
// instance of it c holds, with no chat line, and reports whether it did. An
// instance holding fewer units and a character leaving the world lose
// nothing.
func (c *Character) DestroyScriptItems(itemID int32, count int) bool {
	if c.Detaching() || c.inventory == nil {
		return false
	}
	inst := c.inventory.ItemByTemplateID(itemID)
	if inst == nil || inst.CountValue() < count {
		return false
	}
	return c.inventory.DestroyItem(inst, count) != nil
}

// HoldsItem reports whether c's inventory holds the item instance
// objectID.
func (c *Character) HoldsItem(objectID int32) bool {
	return c.inventory != nil && c.inventory.ItemByObjectID(objectID) != nil
}

// NotifySystemMessage shows c the system message id, which takes no
// parameter.
func (c *Character) NotifySystemMessage(id int) {
	c.emit(event.SystemMessageShown{ID: id})
}

// NotifySound plays the sound file for c alone.
func (c *Character) NotifySound(file string) {
	c.emit(event.SoundPlayed{File: file})
}

// PartyMembers returns the characters of c's party, in party order, nil
// when c is in none.
func (c *Character) PartyMembers() []*Character {
	if c.social == nil {
		return nil
	}
	return c.social.PartyMembers(c.ObjectID())
}

// OnlineClanLeader returns the leader of c's clan when that leader is in
// the world.
func (c *Character) OnlineClanLeader() (*Character, bool) {
	clanID := c.ClanID()
	if clanID == 0 || c.social == nil || c.world == nil {
		return nil, false
	}
	obj, ok := c.world.Player(c.social.ClanLeaderID(clanID))
	if !ok {
		return nil, false
	}
	h, ok := obj.(CharacterHolder)
	if !ok {
		return nil, false
	}
	return h.PlayerCharacter(), true
}
