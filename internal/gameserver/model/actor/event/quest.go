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
