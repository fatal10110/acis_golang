package event

import "github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"

// QuestListChanged reports that the character's quest window changed:
// Entries is the window as the change left it.
type QuestListChanged struct{ Entries []questlog.Entry }

func (QuestListChanged) event() {}

// QuestMarked reports that the quest QuestID moved to a new step, which the
// client marks in its quest window.
type QuestMarked struct{ QuestID int32 }

func (QuestMarked) event() {}

// DialogPageShown reports a dialog window opening on the character as the
// NPC ObjectID's (0 for none): the datapack page File, or HTML when File is
// empty, with the NPC's object id filled in. The client is released right
// after it.
type DialogPageShown struct {
	ObjectID int32
	File     string
	HTML     string
}

func (DialogPageShown) event() {}

// DialogReleased reports that the dialog the character's client waits on
// ends with nothing shown.
type DialogReleased struct{}

func (DialogReleased) event() {}

// ScriptMessage reports a chat line a script tells the character.
type ScriptMessage struct{ Text string }

func (ScriptMessage) event() {}

// QuestOverweight reports a quest dialog refused because the character
// carries too much.
type QuestOverweight struct{}

func (QuestOverweight) event() {}
