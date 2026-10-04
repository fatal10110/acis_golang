package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// LootChannel implements manager.LootChannels: the command channel the
// player's party is in, held by identity.
func (l *GameClientLink) LootChannel(playerID int32) (manager.LootChannel, bool) {
	if l == nil || l.parties == nil {
		return nil, false
	}
	ref, ok := l.parties.ChannelRef(playerID)
	if !ok {
		return nil, false
	}
	return lootChannel{ref: ref}, true
}

// lootChannel is one command channel of the link's registry; equal values
// are the same channel.
type lootChannel struct {
	ref party.ChannelRef[*livePlayer]
}

func (c lootChannel) MembersCount() int { return c.ref.MembersCount() }

func (c lootChannel) Leader() *player.Character { return c.ref.Leader().Character }
