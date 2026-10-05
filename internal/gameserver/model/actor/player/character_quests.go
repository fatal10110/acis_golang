package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
)

// Quests returns c's quest journal.
func (c *Character) Quests() *questlog.Journal {
	return &c.quests
}

// NotifyQuestList shows c the quest window entries.
func (c *Character) NotifyQuestList(entries []questlog.Entry) {
	c.emit(event.QuestListChanged{Entries: entries})
}

// NotifyQuestMarked marks quest questID in c's quest window as moved to a
// new step.
func (c *Character) NotifyQuestMarked(questID int32) {
	c.emit(event.QuestMarked{QuestID: questID})
}

// MarkDetaching marks c as leaving the world and seals its quest journal:
// from here on nothing writes its journal, so nothing lands after the final
// saves of its departure. Detaching reports the mark to the item give and
// take paths, which must refuse once it is set.
func (c *Character) MarkDetaching() {
	c.detaching.Store(true)
	c.quests.Seal()
}

// Detaching reports whether MarkDetaching has run.
func (c *Character) Detaching() bool { return c.detaching.Load() }
