package network

import (
	"fmt"
	"strconv"

	enchantflow "github.com/fatal10110/acis_golang/internal/gameserver/enchant"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
)

// adminEnchant answers //enchant <slot> <level>: the item the selected
// player (gm itself without one) wears in that slot takes that enchant
// level, with the skills the new level brings or takes, and gm is told the
// change.
//
// The item's InventoryUpdate goes to its owner, whose inventory changed.
func (l *GameClientLink) adminEnchant(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) != 2 {
		sendText(gm, "Usage: //enchant slot enchant")
		sendText(gm, "Slots: under|lear|rear|neck|lfinger|rfinger|head|rhand|lhand")
		sendText(gm, "Slots: gloves|chest|legs|feet|cloak|face|hair|hairall")
		return
	}
	slot, ok := handleradmin.PaperdollSlot(args[0])
	if !ok {
		sendText(gm, "Unknown paperdoll slot.")
		return
	}
	level, ok := parseJavaInt(args[1])
	if !ok {
		sendText(gm, "Please specify a new enchant value.")
		return
	}
	if level < 0 || level > enchantflow.MaxAdminLevel {
		sendText(gm, "You must set the enchant level between 0 - 65535.")
		return
	}
	target := adminTargetPlayer(gm, true)
	onPlayer(gm, target, func() { l.setEnchantLevel(gm, target, slot, int(level)) })
}

// setEnchantLevel runs //enchant's change on target's queue.
func (l *GameClientLink) setEnchantLevel(gm, target *livePlayer, slot, level int) {
	inv := target.Inventory()
	if inv == nil {
		return
	}
	end := l.itemInstances.BeginOperation(inv.OwnerID())
	result := l.enchantService().SetLevel(inv, slot, level)
	l.applyPersistActions(result.Persist)
	end()
	if result.Item == nil {
		sendText(gm, fmt.Sprintf("%s doesn't wear any item in %s slot.", target.Name, handleradmin.PaperdollSlotName(slot)))
		return
	}
	name := ""
	if result.Template != nil {
		name = result.Template.Name
	}
	if result.Unchanged {
		sendText(gm, target.Name+"'s "+name+" enchant is already set to "+strconv.Itoa(level)+".")
		return
	}
	l.applyEnchantSteps(target, result.Steps)
	sendText(gm, fmt.Sprintf("%s's %s enchant was modified from %d to %d.", target.Name, name, result.OldLevel, level))
}
