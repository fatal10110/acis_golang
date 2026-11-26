package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelmultisell "github.com/fatal10110/acis_golang/internal/gameserver/model/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/multisell"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// openMultisell shows live multisell list name as f prepares it, one
// MultiSellList per page of modelmultisell.PageSize entries and at least one,
// and makes it the list live's exchanges choose from. A list applying
// taxes adds the tax of f's castle when a clan owns it. A name naming no
// list f may open shows nothing and keeps the list live had.
func (l *GameClientLink) openMultisell(live *livePlayer, f *npc.Folk, name string, inventoryOnly bool) {
	list := l.multisell.Open(live.Character, name, f.NpcID(), inventoryOnly, l.npcOwnedCastleTaxRate(f.Instance))
	if list == nil {
		return
	}
	for index := 0; ; {
		live.SendFrame(serverpackets.FrameMultiSellList(list, index))
		if index += modelmultisell.PageSize; index >= len(list.Entries) {
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
// list, the NPC or live's reach, drops the list without a word by
// design. No client action waits on the answer: the multisell
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
	out := l.multisell.Choose(live.Character, live.shownMultisell.Load(), npcID, reachable, multisell.Choice{
		ListID: req.ListID, EntryID: req.EntryID, Amount: req.Amount,
	})
	if out.Err != nil {
		l.log.Error().Err(out.Err).Int32("multisell", req.ListID).Msg("multisell choose: allocate item id")
	}
	if out.Forget {
		live.shownMultisell.Store(nil)
	}
	for _, n := range out.Notices {
		switch n := n.(type) {
		case multisell.WeightExceeded:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWeightLimitExceeded))
		case multisell.SlotsFull:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSlotsFull))
		case multisell.QuantityExceeded:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExceededQuantityThatCanBeInput))
		case multisell.NotClanMember:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouAreNotAClanMember))
		case multisell.NotClanLeader:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOnlyClanLeaderEnabled))
		case multisell.ClanReputationTooLow:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanReputationScoreTooLow))
		case multisell.ClanReputationChanged:
			l.sendReputationChange(n.Clan, n.Change, live)
		case multisell.ReputationDeducted:
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageS1DeductedFromClanRep, int32(n.Points)))
		case multisell.NotEnoughItems:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughItems))
		case multisell.Consumed:
			sendDestroyedMessage(live, n.ItemID, n.Count)
		case multisell.Earned:
			if n.Count > 1 {
				live.SendFrame(serverpackets.FrameSystemMessageItemNameNumber(serverpackets.SystemMessageEarnedS2S1S, n.ItemID, int32(n.Count)))
			} else {
				live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageEarnedItemS1, n.ItemID))
			}
		case multisell.EnchantedAcquired:
			live.SendFrame(serverpackets.FrameSystemMessageNumberItemName(serverpackets.SystemMessageAcquiredS1S2, int32(n.Enchant), n.ItemID))
		case multisell.Traded:
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSuccessfullyTradedWithNpc))
		}
	}
	if out.Tax > 0 && folk != nil {
		if c, ok := l.npcCastle(folk.Instance); ok {
			c.RiseTaxRevenue(int64(out.Tax))
		}
	}
}
