package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// clanWarDeath reports whether c, killed by pk (nil for a death no player
// caused), dies between clans at war: both belong to a clan and either
// clan has declared war on the other.
func (c *Character) clanWarDeath(pk *Character) bool {
	if pk == nil || c.social == nil {
		return false
	}
	own, theirs := c.ClanID(), pk.ClanID()
	return own != 0 && theirs != 0 && (c.social.AtWar(own, theirs) || c.social.AtWar(theirs, own))
}

// reportClanKill reports c's death to a clan member, or its summon, for the
// clans to settle the reputation a kill between clans at war moves. A
// death inside an arena moves none, nor does one where the victim or the
// killer holds a cursed weapon.
func (c *Character) reportClanKill(killer attackable.Combatant) {
	pk := actingCharacter(killer)
	if pk == nil || attackable.InArena(c) || c.CursedWeaponEquipped() || pk.CursedWeaponEquipped() {
		return
	}
	if c.ClanID() == 0 || pk.ClanID() == 0 {
		return
	}
	c.emit(event.ClanKill{KillerID: pk.ID, KillerClanID: pk.ClanID()})
}
