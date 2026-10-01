package player

import "sync/atomic"

// clanState is the character's side of its clan membership. A clan
// operation another player runs (an expulsion, a leader's level-up) and the
// packets other players build read it from their own queues, so every field
// is atomic.
type clanState struct {
	id          atomic.Int32
	pledgeClass atomic.Int32
	// joinExpiry and createExpiry are epoch milliseconds before which the
	// character may not join, respectively found, a clan; zero when free.
	joinExpiry   atomic.Int64
	createExpiry atomic.Int64
}

// ClanID is the id of the character's clan, or 0 when it has none.
func (c *Character) ClanID() int32 { return c.clan.id.Load() }

// SetClanID records the character's clan, 0 for none.
func (c *Character) SetClanID(id int32) { c.clan.id.Store(id) }

// PledgeClass is the clan rank shown in the status window, as last
// computed by the clan system.
func (c *Character) PledgeClass() int { return int(c.clan.pledgeClass.Load()) }

// SetPledgeClass records the character's clan rank.
func (c *Character) SetPledgeClass(class int) { c.clan.pledgeClass.Store(int32(class)) }

// ClanJoinExpiryTime is the epoch millisecond until which the character may
// not join a clan, 0 when it may.
func (c *Character) ClanJoinExpiryTime() int64 { return c.clan.joinExpiry.Load() }

// SetClanJoinExpiryTime records the clan join penalty's end.
func (c *Character) SetClanJoinExpiryTime(ms int64) { c.clan.joinExpiry.Store(ms) }

// ClanCreateExpiryTime is the epoch millisecond until which the character
// may not found a clan, 0 when it may.
func (c *Character) ClanCreateExpiryTime() int64 { return c.clan.createExpiry.Load() }

// SetClanCreateExpiryTime records the clan creation penalty's end.
func (c *Character) SetClanCreateExpiryTime(ms int64) { c.clan.createExpiry.Store(ms) }

// Title is the character's title, "" when it has none.
func (c *Character) Title() string {
	if t := c.title.Load(); t != nil {
		return *t
	}
	return ""
}

// SetTitle replaces the character's title.
func (c *Character) SetTitle(title string) { c.title.Store(&title) }
