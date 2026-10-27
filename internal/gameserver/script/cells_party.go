package script

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// PartyCell holds one party by identity, or none (#4287). The party is
// followed as it changes, whoever joins or leaves it meanwhile; once it
// disbands it has no members, even after its former members group again,
// which makes another party. Like the other cells it is leaf-locked, takes
// no closure, and its zero value is ready to use.
type PartyCell struct {
	mu    sync.Mutex
	party player.Party
}

// SetPartyOf makes the cell hold the party p is in; a player in no party
// empties it.
func (c *PartyCell) SetPartyOf(p *Player) {
	ref, _ := p.character().Party()
	c.mu.Lock()
	c.party = ref
	c.mu.Unlock()
}

// ShowScreenMessage shows text at the top center of the screen of every
// member of the held party as it stands now, in join order, for durationMs
// milliseconds. A disbanded party, and an empty cell, show it to no one.
func (c *PartyCell) ShowScreenMessage(durationMs int32, text string) {
	c.mu.Lock()
	ref := c.party
	c.mu.Unlock()
	if ref == nil {
		return
	}
	for _, m := range ref.Members() {
		m.ShowScreenMessage(text, durationMs)
	}
}
