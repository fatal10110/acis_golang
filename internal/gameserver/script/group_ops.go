package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// ChannelParties returns the parties of the command channel the player's
// party is in, in the order they joined, each as its players in party
// order, the leader first; nil when it is in none. No range or state is
// checked.
func (p *Player) ChannelParties() [][]*Player {
	_, parties, ok := p.character().Channel()
	if !ok {
		return nil
	}
	out := make([][]*Player, len(parties))
	for i, members := range parties {
		out[i] = playersOf(members)
	}
	return out
}

// ChannelMembers returns the players of every party of the command channel
// the player's party is in, party by party as ChannelParties lists them;
// nil when it is in none.
func (p *Player) ChannelMembers() []*Player {
	_, parties, ok := p.character().Channel()
	if !ok {
		return nil
	}
	var out []*Player
	for _, members := range parties {
		out = append(out, playersOf(members)...)
	}
	return out
}

// ChannelLeader returns the leader of the command channel the player's
// party is in, nil when it is in none.
func (p *Player) ChannelLeader() *Player {
	leader, _, ok := p.character().Channel()
	if !ok {
		return nil
	}
	return playerOfCharacter(leader)
}

// ShowPartyScreenMessage shows text at the top center of the screen of
// every member of the player's party, the player included, in party order,
// for durationMs milliseconds. The party is the one the player is in as the
// message goes out; a player in no party shows it to no one.
func (p *Player) ShowPartyScreenMessage(durationMs int32, text string) {
	p.character().ShowPartyScreenMessage(text, durationMs)
}

// ShowChannelScreenMessage shows text at the top center of the screen of
// every member of the command channel the player's party is in, party by
// party, for durationMs milliseconds. A player in no channel shows it to no
// one.
func (p *Player) ShowChannelScreenMessage(durationMs int32, text string) {
	p.character().ShowChannelScreenMessage(text, durationMs)
}

// playersOf returns the handles of the characters still in the world, in
// order.
func playersOf(members []*player.Character) []*Player {
	out := make([]*Player, 0, len(members))
	for _, m := range members {
		if h := playerOfCharacter(m); h != nil {
			out = append(out, h)
		}
	}
	return out
}

// playerOfCharacter returns the handle of c, nil when c is not in the world.
func playerOfCharacter(c *player.Character) *Player {
	if self, ok := c.WorldHandle().(attackable.Combatant); ok {
		return PlayerOf(self)
	}
	return nil
}
