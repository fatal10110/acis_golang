package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/exchange"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// openMultisell shows live multisell list name as f prepares it, one
// MultiSellList per page of multisell.PageSize entries and at least one,
// and makes it the list live's exchanges choose from. A list applying
// taxes adds the tax of f's castle when a clan owns it. A name naming no
// list f may open shows nothing and keeps the list live had.
func (l *GameClientLink) openMultisell(live *livePlayer, f *npc.Folk, name string, inventoryOnly bool) {
	list := l.exchange.Open(live.Character, name, f.NpcID(), inventoryOnly, l.npcOwnedCastleTaxRate(f.Instance))
	if list == nil {
		return
	}
	for index := 0; ; {
		live.SendFrame(serverpackets.FrameMultiSellList(list, index))
		if index += multisell.PageSize; index >= len(list.Entries) {
			break
		}
	}
	live.shownMultisell.Store(list)
}

// requestMultiSellChoose answers MultiSellChoose: an exchange from the
// list live was last shown, at the civilian NPC live last selected. allowed
// is false inside the multisell reuse window. The castle tax a completed
// exchange took goes to the tax revenue of that NPC's castle.
//
// A choice inside the reuse window, or one that does not match the open
// list, the NPC or live's reach, drops the list without a word, as
// specified. No client action waits on the answer: the multisell
// window stays open and the client only sends again on the next click.
func (l *GameClientLink) requestMultiSellChoose(live *livePlayer, req clientpackets.MultiSellChoose, allowed bool) {
	if !allowed {
		live.shownMultisell.Store(nil)
		return
	}
	npcID, reachable := 0, false
	folk := live.currentFolk.Load()
	if folk != nil {
		npcID, reachable = folk.NpcID(), l.playerCanDoInteract(live, folk)
	}
	out := l.exchange.Choose(live.Character, live.shownMultisell.Load(), npcID, reachable, exchange.Choice{
		ListID: req.ListID, EntryID: req.EntryID, Amount: req.Amount,
	})
	if out.Forget {
		live.shownMultisell.Store(nil)
	}
	for _, n := range out.Notices {
		switch n := n.(type) {
		case exchange.WeightExceeded:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWeightLimitExceeded))
		case exchange.SlotsFull:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
		case exchange.QuantityExceeded:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExceededQuantityThatCanBeInput))
		case exchange.NotClanMember:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouAreNotAClanMember))
		case exchange.NotClanLeader:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyClanLeaderEnabled))
		case exchange.ClanReputationTooLow:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanReputationScoreTooLow))
		case exchange.ClanReputationChanged:
			l.sendReputationChange(n.Clan, n.Change, live)
		case exchange.ReputationDeducted:
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DeductedFromClanRep, int32(n.Points)))
		case exchange.NotEnoughItems:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		case exchange.Consumed:
			sendDestroyedMessage(live, n.ItemID, n.Count)
		case exchange.Earned:
			if n.Count > 1 {
				live.SendFrame(serverpackets.FrameSystemMessageItemNameNumber(serverpackets.SystemMessageEarnedS2S1S, n.ItemID, int32(n.Count)))
			} else {
				live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageEarnedItemS1, n.ItemID))
			}
		case exchange.EnchantedAcquired:
			live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageAcquiredS1S2, int32(n.Enchant), n.ItemID))
		case exchange.Traded:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSuccessfullyTradedWithNpc))
		}
	}
	if out.Tax > 0 && folk != nil {
		if c, ok := l.npcCastle(folk.Instance); ok {
			c.RiseTaxRevenue(int64(out.Tax))
		}
	}
}
