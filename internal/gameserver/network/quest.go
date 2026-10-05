package network

import (
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
	list := c.Quests().List()
	entries := make([]serverpackets.QuestListEntry, len(list))
	for i, e := range list {
		entries[i] = serverpackets.QuestListEntry{QuestID: e.QuestID, Flags: e.Flags}
	}
	return serverpackets.FrameQuestList(entries)
}
