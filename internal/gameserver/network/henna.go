package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/symbolmaker"
)

// Symbol maker requests carry no NPC: each one is answered wherever the
// player stands, the draw and deletion windows only opening
// from a symbol maker's dialog. Each request runs on the player's queue, so
// one player's draws and deletions never interleave.

// sendHennaEquipList opens live's draw window: the symbols its class may
// use and whose dye it carries.
func (l *GameClientLink) sendHennaEquipList(live *livePlayer) {
	live.SendFrame(serverpackets.FrameHennaEquipList(liveAdena(live), live.HennaMaxSlots(), l.symbols.Drawable(live.Character)))
}

// openHennaRemoveList answers a symbol maker's RemoveList command: the
// deletion window, or SYMBOL_NOT_FOUND when live wears no symbol.
func (l *GameClientLink) openHennaRemoveList(live *livePlayer) {
	if len(live.HennaList().Hennas()) == 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSymbolNotFound))
		return
	}
	sendHennaUnequipList(live)
}

// sendHennaUnequipList opens live's deletion window on the symbols it
// wears, in slot order.
func sendHennaUnequipList(live *livePlayer) {
	live.SendFrame(serverpackets.FrameHennaUnequipList(liveAdena(live), live.HennaEmptySlots(), live.HennaList().Hennas()))
}

// sendHennaItemInfo answers RequestHennaItemInfo with a loaded symbol's
// draw details, whatever live's class. An unknown symbol is dropped without
// a word, as specified; the draw window asks again on the next
// click, so no client action waits on it.
func (l *GameClientLink) sendHennaItemInfo(live *livePlayer, req clientpackets.RequestHennaSymbol) {
	h, ok := l.symbols.Henna(int(req.SymbolID))
	if !ok {
		return
	}
	live.SendFrame(serverpackets.FrameHennaItemInfo(h, liveAdena(live), liveHennaStats(live)))
}

// sendHennaUnequipInfo answers RequestHennaUnequipInfo with a loaded
// symbol's deletion details, worn or not. An unknown symbol is dropped
// without a word, as specified.
func (l *GameClientLink) sendHennaUnequipInfo(live *livePlayer, req clientpackets.RequestHennaSymbol) {
	h, ok := l.symbols.Henna(int(req.SymbolID))
	if !ok {
		return
	}
	live.SendFrame(serverpackets.FrameHennaItemUnequipInfo(h, liveAdena(live), liveHennaStats(live)))
}

// drawHenna answers RequestHennaEquip. A drawn symbol is saved to its slot
// and live then sees its symbols, its refreshed stats and SYMBOL_ADDED. An
// unknown symbol is dropped without a word, as specified.
func (l *GameClientLink) drawHenna(live *livePlayer, req clientpackets.RequestHennaSymbol) {
	d := l.symbols.Draw(live.Character, int(req.SymbolID))
	for _, n := range d.Notices {
		switch n := n.(type) {
		case symbolmaker.CantDraw:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantDrawSymbol))
		case symbolmaker.SymbolsFull:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSymbolsFull))
		case symbolmaker.NotEnoughAdena:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		case symbolmaker.AdenaSpent:
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DisappearedAdena, int32(n.Count)))
		case symbolmaker.NotEnoughItems:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		case symbolmaker.DyeConsumed:
			live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageS2S1Disappeared, n.ItemID, int32(n.Count)))
		}
	}
	if !d.Added {
		return
	}
	l.saveHennaSlot(live, d.Symbol, d.DBSlot)
	l.sendHennaChange(live)
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSymbolAdded))
}

// deleteHenna answers RequestHennaUnequip. A deleted symbol's slot row is
// removed and live then sees its symbols, its refreshed stats, the dyes
// handed back and SYMBOL_DELETED. A symbol live does not wear is dropped
// without a word, as specified.
func (l *GameClientLink) deleteHenna(live *livePlayer, req clientpackets.RequestHennaSymbol) {
	d := l.symbols.Delete(live.Character, int(req.SymbolID))
	if d.NotEnoughAdena {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouNotEnoughAdena))
		return
	}
	if !d.Removed {
		return
	}
	if l.hennas != nil {
		slot, classIndex := d.DBSlot, live.ClassIndex()
		l.queueRowWrite(live.ObjectID(), "delete henna", func(ctx context.Context, ownerID int32) error {
			return l.hennas.Delete(ctx, ownerID, classIndex, slot)
		})
	}
	l.sendHennaChange(live)
	if d.DyesReturned > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageItemNameItemNumber(serverpackets.SystemMessageYouPickedUpS2S1, d.Symbol.DyeID, int32(d.DyesReturned)))
	}
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSymbolDeleted))
}

// saveHennaSlot queues the row of h drawn into dbSlot.
func (l *GameClientLink) saveHennaSlot(live *livePlayer, h henna.Henna, dbSlot int) {
	if l.hennas == nil {
		return
	}
	symbolID, classIndex := h.SymbolID, live.ClassIndex()
	l.queueRowWrite(live.ObjectID(), "insert henna", func(ctx context.Context, ownerID int32) error {
		return l.hennas.Insert(ctx, ownerID, classIndex, symbolID, dbSlot)
	})
}

// sendHennaChange shows live its symbols and the stats they now give.
func (l *GameClientLink) sendHennaChange(live *livePlayer) {
	live.SendFrame(serverpackets.FrameHennaInfo(live.HennaSnapshot()))
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
}

func liveAdena(live *livePlayer) int {
	if inv := live.Inventory(); inv != nil {
		return inv.Adena()
	}
	return 0
}

func liveHennaStats(live *livePlayer) serverpackets.HennaStats {
	return serverpackets.HennaStats{INT: live.INT(), STR: live.STR(), CON: live.CON(), MEN: live.MEN(), DEX: live.DEX(), WIT: live.WIT()}
}
