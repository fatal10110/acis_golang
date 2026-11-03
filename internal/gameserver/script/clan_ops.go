package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// The reads of the player's clan that quests and behaviors make on the
// player handle.

// ClanLeader returns the leader of the player's clan while that leader is in
// the world, the player itself when it leads; nil while the leader is
// offline. A player in no clan panics.
func (p *Player) ClanLeader() *Player {
	leader, ok := p.clanCharacter().OnlineClanLeader()
	if !ok {
		return nil
	}
	self, ok := leader.WorldHandle().(attackable.Combatant)
	if !ok {
		return nil
	}
	return PlayerOf(self)
}

// ClanHallID returns the id of the clan hall the player's clan owns, a
// siegable one included, 0 for none. A player in no clan panics.
func (p *Player) ClanHallID() int32 { return p.clanCharacter().ClanHallID() }
