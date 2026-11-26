package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// partyMembers returns the members of p's party, or false outside one.
func (l *GameClientLink) partyMembers(p *livePlayer) ([]*livePlayer, bool) {
	if l.parties == nil {
		return nil, false
	}
	view, ok := l.parties.View(p.ObjectID())
	return view.Members, ok
}

// shareAdena pays each member within party range of origin its share of
// adena (party.AdenaShares). Each one hears what it earned, even nothing.
func (l *GameClientLink) shareAdena(members []*livePlayer, adena int, origin party.LootOrigin) {
	takers, share := party.AdenaShares(members, adena, origin, l.killRewards.PartyRange())
	for _, m := range takers {
		if share < 1 {
			m.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageEarnedS1Adena, 0))
			continue
		}
		m.AddRewardItem(item.AdenaID, share, false, l.nextObjectID)
	}
}

// sendPartyLootNotice tells every member but looter what looter took: its
// count when more than one, else an enchanted item's enchant level. A spoil
// reads as swept up.
func sendPartyLootNotice(members []*livePlayer, looter *livePlayer, spoil bool, itemID int32, count, enchant int) {
	broadcastFrame(func() wire.Frame {
		return partyLootNoticeFrame(looter.CharacterName(), spoil, itemID, count, enchant)
	}, func(send func(frameReceiver)) {
		for _, m := range members {
			if m != looter {
				send(m)
			}
		}
	})
}

func partyLootNoticeFrame(looter string, spoil bool, itemID int32, count, enchant int) wire.Frame {
	name := serverpackets.TextParam(looter)
	switch {
	case count > 1 && spoil:
		return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1SweptUpS3S2, name, serverpackets.ItemNameParam(itemID), serverpackets.ItemNumberParam(int32(count)))
	case count > 1:
		return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1ObtainedS3S2, name, serverpackets.ItemNameParam(itemID), serverpackets.ItemNumberParam(int32(count)))
	case spoil:
		return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1SweptUpS2, name, serverpackets.ItemNameParam(itemID))
	case enchant > 0:
		return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1ObtainedS2S3, name, serverpackets.NumberParam(int32(enchant)), serverpackets.ItemNameParam(itemID))
	default:
		return serverpackets.FrameSystemMessageParams(serverpackets.SystemMessageS1ObtainedS2, name, serverpackets.ItemNameParam(itemID))
	}
}

// lootForParty hands count units of itemID, which picker auto-looted or
// swept from origin, to its party: adena is shared, anything else goes to
// the member the loot rule picks, and the other members hear who took it.
// It reports false when picker is in no party, leaving the item to the
// caller.
func (l *GameClientLink) lootForParty(picker *livePlayer, itemID int32, count int, spoil bool, origin party.LootOrigin) bool {
	if l.parties == nil {
		return false
	}
	if itemID == item.AdenaID {
		members, ok := l.partyMembers(picker)
		if ok {
			l.shareAdena(members, count, origin)
		}
		return ok
	}
	rewards := l.killRewards.Settings()
	units := 1
	if rewards.MultipleItemDrop {
		units = count
	}
	looter, members, ok := l.parties.Looter(picker.ObjectID(), spoil, party.LootEligible[*livePlayer](rewards.PartyRange, itemID, units, origin))
	if !ok {
		return false
	}
	var added bool
	if spoil {
		added = looter.AddEarnedItem(itemID, count, l.nextObjectID)
	} else {
		added = looter.AddRewardItem(itemID, count, rewards.MultipleItemDrop, l.nextObjectID)
	}
	if !added {
		// The looter began leaving the world since it was picked: the
		// item stays the caller's.
		return false
	}
	sendPartyLootNotice(members, looter, spoil, itemID, count, 0)
	return true
}

var _ player.PartyLoot = (*GameClientLink)(nil)

// LootForParty implements player.PartyLoot: an item the online player
// playerID auto-looted or swept follows its party's loot rule.
func (l *GameClientLink) LootForParty(playerID int32, itemID int32, count int, spoil bool, origin party.LootOrigin) bool {
	if l.world == nil {
		return false
	}
	obj, ok := l.world.Player(playerID)
	if !ok {
		return false
	}
	live, ok := obj.(*livePlayer)
	return ok && l.lootForParty(live, itemID, count, spoil, origin)
}
