package script

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// HasItems reports whether the player's inventory holds an instance of
// every one of itemIDs (ItemContainer.hasItems). An empty inventory holds
// none of them; with no ids, the answer is whether it holds anything.
func (p *Player) HasItems(itemIDs ...int32) bool {
	inv := p.character().Inventory()
	if inv == nil || inv.Size() == 0 {
		return false
	}
	return inv.HasItems(itemIDs...)
}

// ItemEnchantLevel returns the enchant level of the player's first
// instance of itemID, in inventory order
// (ItemContainer.getItemByItemId(itemID).getEnchantLevel()). A player who
// holds none panics, so the invocation aborts there.
func (p *Player) ItemEnchantLevel(itemID int32) int32 {
	if inv := p.character().Inventory(); inv != nil {
		if inst := inv.ItemByTemplateID(itemID); inst != nil {
			return int32(inst.Snapshot().EnchantLevel)
		}
	}
	panic(fmt.Sprintf("script: the enchant level of item %d the player does not hold", itemID))
}

// ActiveWeaponItemID returns the item id of the weapon the player attacks
// with (Player.getActiveWeaponItem().getItemId()): the one in its right
// hand, or its class's fists when that hand holds no weapon.
func (p *Player) ActiveWeaponItemID() int32 { return p.character().ActiveWeaponItemID() }

// PkKills returns the player's PK count (Player.getPkKills).
func (p *Player) PkKills() int32 { return int32(p.character().ProgressionValues().PKKills) }

// SetPkKills sets the player's PK count to n (Player.setPkKills). Nothing
// is shown: SendUserInfo shows the player its new count. It is saved with
// the character.
func (p *Player) SetPkKills(n int32) { p.character().SetPKKills(int(n)) }

// SendUserInfo sends the player its own full view, and no one else
// anything (Player.sendPacket of a UserInfo).
func (p *Player) SendUserInfo() { p.character().UpdateUserInfo() }

// ActiveSummon returns the player's summon in the world, a pet or a
// servitor, dead or alive; nil when it has none (Player.getSummon).
func (p *Player) ActiveSummon() Creature {
	s, ok := p.character().Summon()
	if !ok {
		return nil
	}
	c, ok := s.(attackable.Combatant)
	if !ok {
		return nil
	}
	return creatureOf(c)
}

// summonTemplate is a summon that tells its NPC template id.
type summonTemplate interface {
	NPCID() int
}

// SummonNpcID returns the NPC template id of the summon c
// (Summon.getNpcId). A handle on anything but a summon panics, so the
// invocation aborts there.
func SummonNpcID(c Creature) int32 {
	if s, ok := combatantOf(c).(summonTemplate); ok && c.Summon() {
		return int32(s.NPCID())
	}
	panic("script: the summon npc id of a creature that is no summon")
}
