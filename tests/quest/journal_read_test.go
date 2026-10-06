package quest

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// journalScripts are the scripts the suites boot: four real quests and a
// script that is not a quest. Q001 takes the golden's quest items on exit.
func journalScripts() gameservertest.Option {
	quest := func(id int32, items ...int32) func() script.Script {
		return func() script.Script { return script.Script{QuestID: id, Items: items} }
	}
	catalog := script.Catalog{
		"quest.Q001_LettersOfLove":           quest(1, q001Items...),
		"quest.Q002_WhatWomenWant":           quest(2),
		"quest.Q003_WillTheSealBeBroken":     quest(3),
		"quest.Q006_StepIntoTheFuture":       quest(6),
		"script.teleport.NoblesseTeleporter": quest(-1),
	}
	var list []script.Listing
	for _, p := range []string{
		"quest.Q001_LettersOfLove", "quest.Q002_WhatWomenWant", "quest.Q003_WillTheSealBeBroken",
		"quest.Q006_StepIntoTheFuture", "script.teleport.NoblesseTeleporter",
	} {
		list = append(list, script.Listing{Path: p})
	}
	return gameservertest.WithScripts(list, catalog)
}

// TestJournalLoadsAtSelectionAndAnswersQuestList seeds a journal and
// selects the character. The quest list the client asks for while loading,
// the one in the EnterWorld burst and the one the in-game request answers
// all list the real quests that are started or completed, in name order,
// with their flags: a row names its quest in any case, a quest's flags are
// its <flags> or come from its <cond>, a created quest and a script that is
// not a quest stay off the list, and an unknown quest is skipped with a
// warning per row while the rest loads. Loading writes nothing. A later
// selection loads the journal afresh.
func TestJournalLoadsAtSelectionAndAnswersQuestList(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithCapturedLog(),
		journalScripts(),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	other := objID + 1000
	insertJournal(t, srv.DB,
		journalRow{objID, "q006_stepintothefuture", "<state>", val("STARTED")},
		journalRow{objID, "q006_stepintothefuture", "<cond>", val("2")},
		journalRow{objID, "Q003_WillTheSealBeBroken", "<state>", val("STARTED")},
		journalRow{objID, "Q003_WillTheSealBeBroken", "<cond>", val("3")},
		journalRow{objID, "Q003_WillTheSealBeBroken", "<flags>", val("-2147483643")},
		journalRow{objID, "Q002_WhatWomenWant", "<state>", val("CREATED")},
		journalRow{objID, "Q001_LettersOfLove", "<state>", val("COMPLETED")},
		journalRow{objID, "Q001_LettersOfLove", "ex", sql.NullString{}},
		journalRow{objID, "Gone", "<state>", val("STARTED")},
		journalRow{objID, "Gone", "<cond>", val("1")},
		journalRow{objID, "NoblesseTeleporter", "<state>", val("STARTED")},
		journalRow{objID, "NoblesseTeleporter", "<cond>", val("1")},
		journalRow{other, "Q002_WhatWomenWant", "<state>", val("STARTED")},
	)
	before := readJournal(t, srv.DB)
	want := []questLine{{1, 0}, {3, -0x7ffffffb}, {6, -0x7ffffffd}}

	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	assertQuestList(t, "loading", c.Read(), want)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	assertQuestList(t, "EnterWorld", readUntil(t, c, serverpackets.OpcodeQuestList), want)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestQuestListInGame))
	assertQuestList(t, "in-game", readUntil(t, c, serverpackets.OpcodeQuestList), want)

	if n := strings.Count(srv.LogText(), `"quest":"Gone"`); n != 2 {
		t.Fatalf("unknown quest warnings = %d, want one per row (2):\n%s", n, srv.LogText())
	}
	srv.FlushPersistence(t)
	if after := readJournal(t, srv.DB); !reflect.DeepEqual(after, before) {
		t.Fatalf("journal rows after the load = %v, want unchanged %v", after, before)
	}

	// Back at character select, the journal changes; the next selection
	// sees the change.
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntil(t, c, serverpackets.OpcodeCharSelectInfo)
	srv.FlushPersistence(t)
	if _, err := srv.DB.ExecContext(context.Background(), "DELETE FROM character_quests WHERE charId=? AND name='Q003_WillTheSealBeBroken'", objID); err != nil {
		t.Fatal(err)
	}
	insertJournal(t, srv.DB, journalRow{objID, "Q002_WhatWomenWant", "<cond>", val("1")})
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE character_quests SET value='STARTED' WHERE charId=? AND name='Q002_WhatWomenWant' AND var='<state>'", objID); err != nil {
		t.Fatal(err)
	}
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	assertQuestList(t, "reselected", c.Read(), []questLine{{1, 0}, {2, -0x7fffffff}, {6, -0x7ffffffd}})
}

// TestFailedJournalLoadRefusesSelection selects a character whose journal
// cannot be read. The selection is refused silently and the connection
// stays at character select: entering with an empty journal would offer
// one-time quest rewards again.
func TestFailedJournalLoadRefusesSelection(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithQuestLoadFault(errors.New("journal unreadable")),
		journalScripts(),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)

	c.Send(encodeRequestGameStart(0))
	c.ExpectNoFrame()
	if _, ok := srv.State.Player(objID); ok {
		t.Fatal("refused selection registered the player in the world")
	}

	// Still at character select: a create is answered.
	c.Send(encodeRequestCharacterCreate("Second"))
	if frame := c.Read(); frame[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("create after the refusal: opcode %#x, want CharCreateOk (%#x)", frame[0], serverpackets.OpcodeCharCreateOk)
	}
}

// TestCharacterPurgeDeletesJournalAndMemo deletes a character with no grace
// period: its character_quests and character_memo rows go with it, another
// character's stay.
func TestCharacterPurgeDeletesJournalAndMemo(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithCharacterDeleteAfter(0),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	other := objID + 1000
	ctx := context.Background()
	insertJournal(t, srv.DB,
		journalRow{objID, "Q001_LettersOfLove", "<state>", val("STARTED")},
		journalRow{objID, "Q001_LettersOfLove", "<cond>", val("1")},
		journalRow{other, "Q001_LettersOfLove", "<state>", val("STARTED")},
	)
	for _, id := range []int32{objID, other} {
		if _, err := srv.DB.ExecContext(ctx, "INSERT INTO character_memo (charId,var,val) VALUES (?,'tutorial','1')", id); err != nil {
			t.Fatal(err)
		}
	}

	c.Send(encodeRequestCharacterDelete(0))
	readUntil(t, c, serverpackets.OpcodeCharDeleteOk)
	readUntil(t, c, serverpackets.OpcodeCharSelectInfo)
	srv.FlushPersistence(t)

	if got, want := readJournal(t, srv.DB), []journalRow{{other, "Q001_LettersOfLove", "<state>", val("STARTED")}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("character_quests after the purge = %v, want only %v", got, want)
	}
	var memos []int32
	rows, err := srv.DB.QueryContext(ctx, "SELECT charId FROM character_memo ORDER BY charId")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		memos = append(memos, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(memos, []int32{other}) {
		t.Fatalf("character_memo owners after the purge = %v, want only %d", memos, other)
	}
}
