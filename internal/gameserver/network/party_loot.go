package network

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// defaultPartyRange is the shipped players.properties PartyRange.
const defaultPartyRange = 1500

// partyLootRange is how near the loot a member must be to share it.
func (l *GameClientLink) partyLootRange() int {
	if l.playerConfig.PartyRange == 0 {
		return defaultPartyRange
	}
	return l.playerConfig.PartyRange
}

// inPartyLootRange reports whether m is within party range of origin, body
// to body in 3D. A range of -1 is unlimited.
func (l *GameClientLink) inPartyLootRange(origin party.LootOrigin, m *livePlayer) bool {
	r := l.partyLootRange()
	if r == -1 {
		return true
	}
	ox, oy, oz := origin.Position()
	mx, my, mz := m.Position()
	dx, dy, dz := int64(ox-mx), int64(oy-my), int64(oz-mz)
	reach := float64(r) + origin.CollisionRadius() + m.CollisionRadius()
	return float64(dx*dx+dy*dy+dz*dz) <= reach*reach
}

// partyLootEligible accepts a member that may take one itemID from origin
// under a random or by-turn loot rule: alive, with room for it, and within
// party range of origin.
func (l *GameClientLink) partyLootEligible(itemID int32, origin party.LootOrigin) func(*livePlayer) bool {
	return func(m *livePlayer) bool {
		inv := m.Inventory()
		return !m.Dead() && inv != nil && inv.ValidateCapacityByItemID(itemID, 1) && l.inPartyLootRange(origin, m)
	}
}

// partyMembers returns the members of p's party, or false outside one.
func (l *GameClientLink) partyMembers(p *livePlayer) ([]*livePlayer, bool) {
	if l.parties == nil {
		return nil, false
	}
	view, ok := l.parties.View(p.ObjectID())
	return view.Members, ok
}

// shareAdena splits adena evenly among members within party range of
// origin whose adena is not already at its cap; the remainder of the
// division is lost. Each one hears what it earned, even nothing.
func (l *GameClientLink) shareAdena(members []*livePlayer, adena int, origin party.LootOrigin) {
	var rewarded []*livePlayer
	for _, m := range members {
		inv := m.Inventory()
		if inv == nil || inv.Adena() == math.MaxInt32 || !l.inPartyLootRange(origin, m) {
			continue
		}
		rewarded = append(rewarded, m)
	}
	if len(rewarded) == 0 {
		return
	}
	share := adena / len(rewarded)
	for _, m := range rewarded {
		if share < 1 {
			m.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageEarnedS1Adena, 0))
			continue
		}
		id, err := l.nextObjectID()
		if err != nil {
			l.log.Error().Err(err).Msg("allocate shared adena")
			continue
		}
		m.AddRewardItem(item.AdenaID, share, id)
	}
}

// sendPartyLootNotice tells every member but looter what looter took: its
// count when more than one, else an enchanted item's enchant level. A spoil
// reads as swept up.
func sendPartyLootNotice(members []*livePlayer, looter *livePlayer, spoil bool, itemID int32, count, enchant int) {
	broadcastFrame(func() wire.Frame {
		return partyLootNoticeFrame(looter.Name, spoil, itemID, count, enchant)
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
	looter, members, ok := l.parties.Looter(picker.ObjectID(), spoil, l.partyLootEligible(itemID, origin))
	if !ok {
		return false
	}
	if spoil {
		looter.AddEarnedItem(itemID, count, l.nextObjectID)
	} else if id, err := l.nextObjectID(); err == nil {
		looter.AddRewardItem(itemID, count, id)
	} else {
		l.log.Error().Err(err).Msg("allocate party loot")
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
