package player

import (
	"sync/atomic"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// cursedWeaponHold is the cursed weapon a character holds and the stage it
// is at; zero for none. The cursed weapon lifecycle sets it from whichever
// queue the change happens on, and packet encoders read it.
type cursedWeaponHold struct {
	itemID atomic.Int32
	stage  atomic.Int32
}

// CursedWeaponEquipped reports whether c holds a cursed weapon.
func (c *Character) CursedWeaponEquipped() bool { return c.cursedWeapon.itemID.Load() != 0 }

// CursedWeaponID returns the item id of the cursed weapon c holds, 0 for
// none.
func (c *Character) CursedWeaponID() int32 { return c.cursedWeapon.itemID.Load() }

// CursedWeaponStage returns the stage of the cursed weapon c holds, 0 for
// none.
func (c *Character) CursedWeaponStage() int32 {
	if !c.CursedWeaponEquipped() {
		return 0
	}
	return c.cursedWeapon.stage.Load()
}

// SetCursedWeapon records that c holds the cursed weapon itemID at stage;
// itemID 0 records none.
func (c *Character) SetCursedWeapon(itemID, stage int32) {
	if itemID == 0 {
		stage = 0
	}
	c.cursedWeapon.stage.Store(stage)
	c.cursedWeapon.itemID.Store(itemID)
}

// SetPKKills sets c's PK kill count. Nothing is announced: the caller
// refreshes what shows it.
func (c *Character) SetPKKills(n int) {
	c.progressionMu.Lock()
	c.PKKills = n
	c.progressionMu.Unlock()
}

// creditCursedWeaponKill reports c's death to the player behind killer
// when that player holds a cursed weapon: the kill feeds the weapon
// (event.CursedWeaponKill) and earns neither karma nor a PvP point. It
// reports whether it did.
func (c *Character) creditCursedWeaponKill(killer attackable.Combatant) bool {
	pk := actingCharacter(killer)
	if pk == nil || pk == c || !pk.CursedWeaponEquipped() {
		return false
	}
	pk.emit(event.CursedWeaponKill{})
	return true
}

// loseCursedWeapon has the cursed weapon c held when killer killed it drop
// or end (event.CursedWeaponLost), and reports whether it did. Such a death
// costs nothing else: no items, no clan reputation, no experience; only the
// snapshot a resurrection restores from is cleared.
func (c *Character) loseCursedWeapon(killer attackable.Combatant) bool {
	if killer == nil || !c.CursedWeaponEquipped() {
		return false
	}
	c.progressionMu.Lock()
	c.ExpBeforeDeath = 0
	c.progressionMu.Unlock()
	c.emit(event.CursedWeaponLost{})
	return true
}
