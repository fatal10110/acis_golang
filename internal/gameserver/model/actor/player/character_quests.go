package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
)

// Quests returns c's quest journal.
func (c *Character) Quests() *questlog.Journal {
	return &c.quests
}

// QuestStarted reports whether c's state in the quest named name is
// started.
func (c *Character) QuestStarted(name string) bool {
	st := c.quests.State(name)
	return st != nil && st.Status() == questlog.StatusStarted
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

// SetLastQuestNPC records objectID as the NPC c last talked to about
// quests: the one a quest event link c sends acts through.
func (c *Character) SetLastQuestNPC(objectID int32) { c.lastQuestNPC.Store(objectID) }

// LastQuestNPC returns the object id SetLastQuestNPC recorded, 0 when none.
func (c *Character) LastQuestNPC() int32 { return c.lastQuestNPC.Load() }

// ShowDialogPage opens a dialog window on c, as the NPC objectID's (0 for
// none), showing the datapack page file, or html when file is empty; the
// client is then released.
func (c *Character) ShowDialogPage(objectID int32, file, html string) {
	c.emit(event.DialogPageShown{ObjectID: objectID, File: file, HTML: html})
}

// ShowTeleportWindow opens on c the list of the destinations of kind the
// NPC objectID, of template npcID, offers, priced for c.
func (c *Character) ShowTeleportWindow(objectID int32, npcID int, kind travel.Kind) {
	c.emit(event.TeleportWindowShown{ObjectID: objectID, NpcID: npcID, Kind: kind})
}

// ReleaseDialog releases c's client from the dialog it is waiting on.
func (c *Character) ReleaseDialog() { c.emit(event.DialogReleased{}) }

// NotifyScriptMessage shows c the chat line text.
func (c *Character) NotifyScriptMessage(text string) {
	c.emit(event.ScriptMessage{Text: text})
}

// NotifyQuestOverweight tells c it carries too much to talk about a quest.
func (c *Character) NotifyQuestOverweight() { c.emit(event.QuestOverweight{}) }
