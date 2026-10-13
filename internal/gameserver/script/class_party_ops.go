package script

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The class change, party, world and quest lookups the third-class saga
// core makes (#3598), each for the player handle.

// ClassParent returns the id of the class classID upgrades from, -1 for a
// starting class. An id that is no class panics, so the
// invocation aborts there.
func ClassParent(classID int32) int32 {
	parent, ok := player.ClassParent(int(classID))
	if !ok {
		panic(fmt.Sprintf("script: no class %d", classID))
	}
	return int32(parent)
}

// SetClassID moves the player to classID as every occupation change does,
// all of it done when the call returns: the change's
// visual shown around the player, the new class played (in an active
// subclass's slot when one is active) with its template, the transfer
// message, the player's row refreshed in its party's and its clan's
// windows, the skills granted when the server grants them automatically,
// and an academy member's graduation on its second occupation. The base
// class does not move; SetBaseClass moves it. Nothing happens while
// another class change of the player is in progress.
func (p *Player) SetClassID(classID int32) {
	p.character().RequestClassChange(int(classID))
}

// SubClassActive reports whether the player plays one of its subclasses.
func (p *Player) SubClassActive() bool { return p.character().SubclassActive() }

// BaseClass returns the id of the player's base class: the class it was
// created into, or the one it last
// moved to on that line.
func (p *Player) BaseClass() int32 { return int32(p.character().BaseClassID()) }

// SetBaseClass makes classID the player's base class.
// The class it plays does not change.
func (p *Player) SetBaseClass(classID int32) {
	p.character().RequestBaseClass(int(classID))
}

// BroadcastUserInfo shows the player anew:
// UserInfo to the player, CharInfo to every player around it.
func (p *Player) BroadcastUserInfo() { p.character().BroadcastUserInfo() }

// BroadcastMagicSkillUse shows the player and every player around it the
// cast animation of skillID at level from caster to target, with hitTime
// and reuseDelay in milliseconds (a MagicSkillUse). It is only shown: no
// skill is used, and caster and
// target may be any creatures. Each stands where it is at the call; a
// handle on nothing panics.
func (p *Player) BroadcastMagicSkillUse(caster, target Creature, skillID, level int32, hitTime, reuseDelay int) {
	from, to := locOf(caster), locOf(target)
	p.character().BroadcastSkillAnimation(
		caster.ObjectID(), location.Location{X: from.X, Y: from.Y, Z: from.Z},
		target.ObjectID(), location.Location{X: to.X, Y: to.Y, Z: to.Z},
		skillID, level, hitTime, reuseDelay,
	)
}

// InParty reports whether the player is in a party.
func (p *Player) InParty() bool { return p.character().IsInParty() }

// PartyMembers returns the players of the player's party, the player
// included, in party order, the leader first; nil when it is in none. No
// range or state is checked.
func (p *Player) PartyMembers() []*Player {
	members := p.character().PartyMembers()
	if members == nil {
		return nil
	}
	out := make([]*Player, 0, len(members))
	for _, m := range members {
		if self, ok := m.WorldHandle().(attackable.Combatant); ok {
			out = append(out, PlayerOf(self))
		}
	}
	return out
}

// WorldPlayer returns the player in the world whose object id is objectID,
// nil when there is none.
func (s *Script) WorldPlayer(objectID int32) *Player {
	if s.env == nil || s.env.World == nil {
		return nil
	}
	o, ok := s.env.World.Player(objectID)
	if !ok {
		return nil
	}
	self, ok := o.(attackable.Combatant)
	if !ok {
		return nil
	}
	return PlayerOf(self)
}

// Quest returns the registered script the state belongs to, so a script
// may call another script's helpers on
// a state of that script it found. nil when the state was not found
// through a registered script.
func (qs *QuestState) Quest() *Script {
	if qs.registry == nil {
		return nil
	}
	return qs.registry.byName[strings.ToLower(qs.state.Quest().Name)]
}
