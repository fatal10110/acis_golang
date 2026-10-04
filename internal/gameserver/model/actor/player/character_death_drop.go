package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// deathDropMinMonsterLevel is the lowest level at which a monster kill can
// cost a player its items.
const deathDropMinMonsterLevel = 5

// DeathDropRates are the percent chances one kind of death rolls: Chance
// that the death drops anything at all, then per item Equip for a worn
// armor or jewel, EquipWeapon for a worn weapon and Item for anything not
// worn, until Limit items have dropped.
type DeathDropRates struct {
	Chance      int
	Equip       int
	EquipWeapon int
	Item        int
	Limit       int
}

// DeathDropRules is what a player's death may cost it in items. A player
// killer (PK) holding at least KarmaPKLimit PK kills and some karma rolls
// the Karma rates whoever kills it; any other player of level 5 or more
// rolls the Monster rates when an NPC kills it. A game master drops nothing
// unless GMDrops is set. Kept lists the item templates no death drops.
// The zero value drops nothing.
type DeathDropRules struct {
	Karma        DeathDropRates
	Monster      DeathDropRates
	KarmaPKLimit int
	GMDrops      bool
	Kept         []int32
}

// reportDeathDrop reports the rates c's death by killer rolls its items at.
// Nothing drops for a death no one caused, nor where a cursed weapon is
// held by c or the player behind killer, nor for a player kill inside a PvP
// zone. A karma-free player killed by another player never drops: neither
// rate set applies to it, so a clan war needs no exemption of its own. The
// game-master exemption is the session's to apply, since only the session
// knows the access level.
func (c *Character) reportDeathDrop(killer attackable.Combatant) {
	if killer == nil {
		return
	}
	pk := actingCharacter(killer)
	if c.CursedWeaponEquipped() || (pk != nil && pk.CursedWeaponEquipped()) {
		return
	}
	if pk != nil && c.InPvPZone() {
		return
	}
	c.stateMu.RLock()
	rules := c.deathDrop
	c.stateMu.RUnlock()
	c.progressionMu.RLock()
	karma, pkKills := c.KarmaPoints, c.PKKills
	c.progressionMu.RUnlock()

	var rates DeathDropRates
	switch {
	case karma > 0 && pkKills >= rules.KarmaPKLimit:
		rates = rules.Karma
	case killer.Kind() == actor.KindNPC && c.Level() >= deathDropMinMonsterLevel && !c.FestivalParticipant():
		rates = rules.Monster
	default:
		return
	}
	if rates.Chance <= 0 {
		return
	}
	c.emit(event.DeathItemDrop{
		Chance:       rates.Chance,
		EquipChance:  rates.Equip,
		WeaponChance: rates.EquipWeapon,
		ItemChance:   rates.Item,
		Limit:        rates.Limit,
	})
}
