package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// The player standing, mount, equipment and status changes the freeze of
// the script-facing API adds (#3748). Each panics on a handle on nothing.

// ClanID returns the id of the player's clan, 0 for none.
func (p *Player) ClanID() int32 { return p.character().ClanID() }

// IsHero reports whether the player is a hero.
func (p *Player) IsHero() bool { return p.character().IsHero() }

// MountType returns what the player rides: 0 nothing, 1 a strider, 2 a
// wyvern.
func (p *Player) MountType() int32 { return p.character().MountType() }

// Flying reports whether the player rides a wyvern.
func (p *Player) Flying() bool { return p.character().MountType() == player.MountTypeWyvern }

// Overweight reports whether the player carries too much: a load in the
// third weight band or above, or an inventory at least 80% full.
func (p *Player) Overweight() bool { return p.character().Overweight() }

// RightHandItemID returns the item id of what the player holds in its right
// hand, 0 when the hand is empty.
func (p *Player) RightHandItemID() int32 { return p.character().RightHandItemID() }

// SetIn7sDungeon marks the player as inside a Seven Signs dungeon, or no
// longer inside one.
func (p *Player) SetIn7sDungeon(in bool) { p.character().SetIn7sDungeon(in) }

// SetVarkaKetraAlliance sets the player's standing with the two factions:
// 1 to 5 allied with the Ketra Orcs, -1 to -5 with the Varka Silenos, 0
// neither. It is stored with the character's next save.
func (p *Player) SetVarkaKetraAlliance(level int32) { p.character().SetVarkaKetraAlliance(int(level)) }

// RefreshHennaList recomputes what the player's dyes add to its stats and
// shows the player its dye list.
func (p *Player) RefreshHennaList() { p.character().RefreshScriptHennaList() }

// MakeNoble makes the player a noblesse: it gets the noble skills, then its
// skill list and its UserInfo, and the status is stored at once.
func (p *Player) MakeNoble() { p.character().MakeScriptNoble() }

// StartedQuests returns how many real quests the player has started.
func (p *Player) StartedQuests() int { return p.character().Quests().Started() }

// AttackType returns the weapon type c attacks with: a player's right-hand
// weapon, its class's fists when that hand is empty; an NPC's template
// weapon; WeaponNone for a creature that has no attack type.
func AttackType(c Creature) item.WeaponType {
	if a, ok := tracked(c).(interface{ AttackType() item.WeaponType }); ok {
		return a.AttackType()
	}
	if h, ok := tracked(c).(player.CharacterHolder); ok {
		return h.PlayerCharacter().AttackType()
	}
	return item.WeaponNone
}

// Riding reports whether c is a player riding a strider.
func Riding(c Creature) bool {
	if h, ok := tracked(c).(player.CharacterHolder); ok {
		return h.PlayerCharacter().Riding()
	}
	return false
}

// PetControlItemID returns the object id of the item that summons c, a
// pet; 0 for a servitor and any other creature.
func PetControlItemID(c Creature) int32 {
	if s, ok := tracked(c).(interface{ ControlItemID() int32 }); ok {
		return s.ControlItemID()
	}
	return 0
}
