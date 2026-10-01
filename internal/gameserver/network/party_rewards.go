package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// RewardParty implements manager.RewardParties: the members a player in a
// party shares a kill with, from a copy of its party or command channel.
func (l *GameClientLink) RewardParty(playerID int32) (manager.RewardParty, bool) {
	if l == nil || l.parties == nil {
		return manager.RewardParty{}, false
	}
	group, ok := l.parties.RewardGroup(playerID)
	if !ok {
		return manager.RewardParty{}, false
	}
	members := make([]*player.Character, len(group.Members))
	for i, m := range group.Members {
		members[i] = m.Character
	}
	return manager.RewardParty{Members: members, InChannel: group.InChannel, ChannelLevel: group.ChannelLevel}, true
}
