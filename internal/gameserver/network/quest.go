package network

import (
	"cmp"
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// questStore reads the saved quest journal rows.
type questStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]questlog.Row, error)
}

// scriptRegistry is what the link asks of the script registry.
type scriptRegistry interface {
	// JournalQuest resolves a journal row's quest name.
	JournalQuest(name string) (questlog.Quest, bool)
	player.TutorialEvents
	// PlayerDetached stops the script timers bound to c as it leaves.
	PlayerDetached(c *player.Character)
}

// questJournals writes the quest journals and aborts quests.
type questJournals interface {
	Abort(c *player.Character, questID int32)
	Seal(c *player.Character)
	Settle(ctx context.Context, ownerID int32) error
}

// abortQuest answers RequestQuestAbort: the first quest of live's journal
// with the quest id is exited as repeatable. An id live has no quest of
// answers nothing, as in the reference; the client sends the request once
// the player confirmed the abort, and waits on no answer.
func (l *GameClientLink) abortQuest(live *livePlayer, questID int32) {
	if l.journals == nil {
		return
	}
	l.journals.Abort(live.Character, questID)
}

// sealQuests drains c's journal for the last time as c leaves; c's
// MarkDetaching has already refused any later change.
func (l *GameClientLink) sealQuests(c *player.Character) {
	if l.journals != nil {
		l.journals.Seal(c)
	}
}

// settleQuests waits, before a selection of objectID loads its journal, for
// the writes an earlier session's journal still owes. An error refuses the
// selection: loading would lose those writes.
func (l *GameClientLink) settleQuests(objectID int32) error {
	if l.journals == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), cmp.Or(l.persistWait, LivePlayerPersistWait))
	defer cancel()
	return l.journals.Settle(ctx, objectID)
}

// restoreQuests loads c's quest journal at its selection. A row whose
// quest no script carries is skipped and logged; the rest loads. A failed
// read is returned: the selection must not go on with an empty journal,
// which would offer one-time quest rewards again.
func (l *GameClientLink) restoreQuests(ctx context.Context, c *player.Character) error {
	if l.quests == nil {
		return nil
	}
	rows, err := l.quests.ListByOwner(ctx, c.ID)
	if err != nil {
		return err
	}
	resolve := func(string) (questlog.Quest, bool) { return questlog.Quest{}, false }
	if l.scripts != nil {
		resolve = l.scripts.JournalQuest
	}
	for _, name := range c.Quests().Restore(rows, resolve) {
		l.log.Warn().Str("quest", name).Str("player", c.Name).Msg("select character: unknown quest in journal; skipped")
	}
	return nil
}

// questListFrame is c's QuestList: the quest window's lines from its
// journal.
func questListFrame(c *player.Character) wire.Frame {
	return questListEntriesFrame(c.Quests().List())
}

// questListEntriesFrame is the QuestList showing list.
func questListEntriesFrame(list []questlog.Entry) wire.Frame {
	entries := make([]serverpackets.QuestListEntry, len(list))
	for i, e := range list {
		entries[i] = serverpackets.QuestListEntry{QuestID: e.QuestID, Flags: e.Flags}
	}
	return serverpackets.FrameQuestList(entries)
}
