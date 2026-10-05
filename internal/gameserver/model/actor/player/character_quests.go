package player

import "github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"

// Quests returns c's quest journal.
func (c *Character) Quests() *questlog.Journal {
	return &c.quests
}
