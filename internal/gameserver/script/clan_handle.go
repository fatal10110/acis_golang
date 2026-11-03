package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// ClanHandle is a script's handle on one clan, as a hall siege script reads
// its attackers and owner (#3502).
type ClanHandle struct {
	c   *clan.Clan
	env *Env
}

// Clan returns the clan id, nil when there is none.
func (s *Script) Clan(id int32) *ClanHandle {
	if s.env == nil || s.env.Clans == nil || id == 0 {
		return nil
	}
	c, ok := s.env.Clans.Get(id)
	if !ok {
		return nil
	}
	return &ClanHandle{c: c, env: s.env}
}

// ID returns the clan's id.
func (c *ClanHandle) ID() int32 { return c.c.ID() }

// Name returns the clan's name.
func (c *ClanHandle) Name() string { return c.c.Name() }

// LeaderID returns the object id of the clan's leader.
func (c *ClanHandle) LeaderID() int32 { return c.c.LeaderID() }

// LeaderName returns the name of the clan's leader.
func (c *ClanHandle) LeaderName() string { return c.c.Info().LeaderName }

// MembersCount returns how many members the clan has, online or not.
func (c *ClanHandle) MembersCount() int { return c.c.MembersCount() }

// HasHall reports whether the clan owns a clan hall.
func (c *ClanHandle) HasHall() bool { return c.c.Info().HallID > 0 }

// DissolvingExpiry returns the epoch millisecond the clan's requested
// dissolution completes, 0 when none is pending.
func (c *ClanHandle) DissolvingExpiry() int64 { return c.c.Info().DissolvingExpiry }

// OnlineMembers returns the clan's members in the world.
func (c *ClanHandle) OnlineMembers() []*Player {
	if c.env.World == nil {
		return nil
	}
	var out []*Player
	for _, id := range c.c.OnlineMemberIDs() {
		if o, ok := c.env.World.Player(id); ok {
			if self, ok := o.(attackable.Combatant); ok {
				out = append(out, PlayerOf(self))
			}
		}
	}
	return out
}
