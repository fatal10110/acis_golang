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
	c.emit(event.ItemObtained{ItemID: itemID, Count: count, Notice: event.ObtainGiven})
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
