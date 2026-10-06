package quest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

const (
	q001     = "Q001_LettersOfLove"
	noblesse = "NoblesseTeleporter"
)

// bootJournal boots one character with the journal scripts.
func bootJournal(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		journalScripts(),
	}, opts...)...)
	return srv, srv.SoleObjectID(t)
}

// enterWorld selects the character and enters the world, reading every
// frame the login sent.
func enterWorld(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, c)
}

// restart goes back to character select.
func restart(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelectInfo)
}

// sentLines renders every frame the server queued to the client since the
// last read as the goldens write packets.
func sentLines(t *testing.T, srv *gameservertest.Server) []string {
	t.Helper()
	var out []string
	for _, f := range srv.ReadQueued(t, srv.Client) {
		line, err := scriptcontract.Packet(f, nil)
		if err != nil {
			t.Fatalf("frame %x: %v", f, err)
		}
		out = append(out, line)
	}
	return out
}

// statementLines renders journal writes as the golden's statement kinds
// and parameters after the player id.
func statementLines(writes []questlog.Write) []string {
	var out []string
	for _, w := range writes {
		switch w.Op {
		case questlog.OpSet:
			out = append(out, fmt.Sprintf("upsert %s | %s | %s", w.Quest, w.Var, w.Value))
		case questlog.OpUnset:
			out = append(out, fmt.Sprintf("delete-var %s | %s", w.Quest, w.Var))
		case questlog.OpDelete:
			out = append(out, "delete-quest "+w.Quest)
		case questlog.OpComplete:
			out = append(out, "delete-except-state "+w.Quest)
		}
	}
	return out
}

// questRows returns objID's character_quests rows of quest as var:value.
func questRows(t *testing.T, srv *gameservertest.Server, objID int32, quest string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range readJournal(t, srv.DB) {
		if r.charID == objID && r.name == quest {
			out[r.variable] = r.value.String
		}
	}
	return out
}

// varsLine writes vars as the golden does: sorted key:value, - for none.
func varsLine(vars map[string]string) string {
	if len(vars) == 0 {
		return "-"
	}
	var kv []string
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		kv = append(kv, k+":"+vars[k])
	}
	return strings.Join(kv, ",")
}

// TestJournalWritesMatchReferenceOrder runs the journal.write_order golden
// end to end: a character enters with one seeded quest state, a script
// operation runs on its queue, and the client receives the reference's
// packets in order while the database gets the reference's statements in
// order, leaving the rows and the journal the reference leaves.
//
// The golden's item-removal system messages are not expected: removing a
// quest's items on exit belongs to the script item helpers, and the seeded
// character holds none.
func TestJournalWritesMatchReferenceOrder(t *testing.T) {
	t.Parallel()
	scriptcontract.Run(t, "journal.write_order", func(t *testing.T, r scriptcontract.Row) {
		t.Parallel()
		srv, objID := bootJournal(t)
		quest := q001
		if r.Str(t, "quest") == "script" {
			quest = noblesse
		}
		if seed := r.Str(t, "seed_state"); seed != "-" {
			rows := []journalRow{{objID, quest, questlog.KeyState, val(seed)}}
			if cond := r.Str(t, "seed_cond"); cond != "0" {
				rows = append(rows, journalRow{objID, quest, questlog.KeyCond, val(cond)})
			}
			if flags := r.Str(t, "seed_flags"); flags != "-" {
				rows = append(rows, journalRow{objID, quest, questlog.KeyFlags, val(flags)})
			}
			if seed == "STARTED" {
				rows = append(rows, journalRow{objID, quest, "ex", val("1")})
			}
			insertJournal(t, srv.DB, rows...)
		}
		enterWorld(t, srv)

		op := strings.Fields(r.Str(t, "op"))
		srv.RunQuest(t, objID, quest, func(q *script.Quests, c *player.Character, sc *script.Script) {
			st := q.State(c, sc)
			switch op[0] {
			case "set":
				st.Set(op[1], op[2])
			case "unset":
				st.Unset(op[1])
			case "setState":
				st.SetStatus(questlog.StatusStarted)
			case "setCond":
				n, _ := strconv.Atoi(op[1])
				st.SetCond(int32(n))
			case "exitQuest":
				st.Exit(op[1] == "true")
			case "newQuestState":
				q.NewState(c, sc)
			}
		})

		var wantS, wantQ []string
		for _, line := range r.Lines {
			switch {
			case strings.HasPrefix(line, "S SystemMessage"):
			case strings.HasPrefix(line, "S "):
				wantS = append(wantS, line)
			default:
				s, err := scriptcontract.ParseStatement(line)
				if err != nil {
					t.Fatal(err)
				}
				wantQ = append(wantQ, string(s.Kind)+" "+strings.Join(s.Params[1:], " | "))
			}
		}
		if got := sentLines(t, srv); !slices.Equal(got, wantS) {
			t.Fatalf("packets:\n got %q\nwant %q", got, wantS)
		}
		srv.FlushPersistence(t)
		if got := statementLines(srv.TakeJournalWrites()); !slices.Equal(got, wantQ) {
			t.Fatalf("statements:\n got %q\nwant %q", got, wantQ)
		}

		vars := srv.QuestVars(t, objID, quest)
		state := "-"
		if v, ok := vars[questlog.KeyState]; ok {
			state = v
		}
		if state != r.Str(t, "state") || varsLine(vars) != r.Str(t, "vars") {
			t.Fatalf("journal after: state=%s vars=%s, want %s %s", state, varsLine(vars), r.Str(t, "state"), r.Str(t, "vars"))
		}
		// A new state writes nothing; every other row leaves the rows the
		// journal holds.
		wantRows := r.Str(t, "vars")
		if op[0] == "newQuestState" {
			wantRows = "-"
		}
		if got := varsLine(questRows(t, srv, objID, quest)); got != wantRows {
			t.Fatalf("rows after = %s, want %s", got, wantRows)
		}
	})
}

// TestJournalWritesSurviveRelog drives a quest through its life and
// relogs: a new state writes nothing and is not saved, a started quest's
// variables and condition are saved and load again on the next selection,
// unsetting a variable that has no row deletes nothing else, and a quest
// completed is listed completed after the relog with its variables gone.
func TestJournalWritesSurviveRelog(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t)
	enterWorld(t, srv)

	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		q.NewState(c, sc)
	})
	if lines := sentLines(t, srv); len(lines) != 0 {
		t.Fatalf("a new state sent %q", lines)
	}
	srv.FlushPersistence(t)
	if rows := questRows(t, srv, objID, q001); len(rows) != 0 {
		t.Fatalf("a new state wrote %v", rows)
	}

	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		st := q.State(c, sc)
		st.SetStatus(questlog.StatusStarted)
		st.Set("ex", "7")
		st.SetCond(1)
		st.SetCond(3)
		st.Unset("missing")
	})
	if got, want := sentLines(t, srv), []string{
		"S QuestList 1:0x80000001", "S ExShowQuestMark quest=1",
		"S QuestList 1:0x80000005", "S ExShowQuestMark quest=1",
	}; !slices.Equal(got, want) {
		t.Fatalf("packets = %q, want %q", got, want)
	}
	srv.FlushPersistence(t)
	want := map[string]string{"<state>": "STARTED", "ex": "7", "<cond>": "3", "<flags>": "-2147483643"}
	if got := questRows(t, srv, objID, q001); !maps.Equal(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}

	restart(t, srv)
	srv.Client.Send(encodeRequestGameStart(0))
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelected)
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	assertQuestList(t, "relog", srv.Client.Read(), []questLine{{1, -0x7ffffffb}})
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, srv.Client)
	if got := srv.QuestVars(t, objID, q001); !maps.Equal(got, want) {
		t.Fatalf("journal after relog = %v, want %v", got, want)
	}

	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		q.State(c, sc).Exit(false)
	})
	if got := sentLines(t, srv); !slices.Equal(got, []string{"S QuestList 1:0x00000000"}) {
		t.Fatalf("exit packets = %q", got)
	}
	restart(t, srv)
	srv.FlushPersistence(t)
	if got := questRows(t, srv, objID, q001); !maps.Equal(got, map[string]string{"<state>": "COMPLETED"}) {
		t.Fatalf("rows after the exit = %v", got)
	}
	srv.Client.Send(encodeRequestGameStart(0))
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelected)
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	assertQuestList(t, "after the exit", srv.Client.Read(), []questLine{{1, 0}})
}

// TestQuestAbortExitsByID aborts quests from the quest window. The request
// names a quest id: the first state of that id in the journal is exited as
// repeatable, so its rows go and a real quest's window is sent. An id with
// no state, or a state that is not started, answers nothing; a script that
// is not a real quest is aborted by its id too, silently.
func TestQuestAbortExitsByID(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t)
	insertJournal(t, srv.DB,
		journalRow{objID, q001, "<state>", val("STARTED")},
		journalRow{objID, q001, "<cond>", val("2")},
		journalRow{objID, "Q002_WhatWomenWant", "<state>", val("CREATED")},
		journalRow{objID, "Q003_WillTheSealBeBroken", "<state>", val("STARTED")},
		journalRow{objID, "Q003_WillTheSealBeBroken", "<cond>", val("1")},
		journalRow{objID, noblesse, "<state>", val("STARTED")},
		journalRow{objID, noblesse, "<cond>", val("1")},
	)
	enterWorld(t, srv)
	c := srv.Client

	for _, id := range []int32{9, 2, 0} {
		c.Send(encodeRequestQuestAbort(id))
		if lines := sentLines(t, srv); len(lines) != 0 {
			t.Fatalf("abort %d answered %q", id, lines)
		}
	}
	c.Send(encodeRequestQuestAbort(1))
	if got := sentLines(t, srv); !slices.Equal(got, []string{"S QuestList 3:0x80000001"}) {
		t.Fatalf("abort 1 packets = %q", got)
	}
	c.Send(encodeRequestQuestAbort(-1))
	if lines := sentLines(t, srv); len(lines) != 0 {
		t.Fatalf("abort -1 answered %q", lines)
	}
	srv.FlushPersistence(t)
	for quest, want := range map[string]map[string]string{
		q001:                       {},
		noblesse:                   {},
		"Q002_WhatWomenWant":       {"<state>": "CREATED"},
		"Q003_WillTheSealBeBroken": {"<state>": "STARTED", "<cond>": "1"},
	} {
		if got := questRows(t, srv, objID, quest); !maps.Equal(got, want) {
			t.Fatalf("%s rows = %v, want %v", quest, got, want)
		}
	}
	if srv.QuestVars(t, objID, q001) != nil {
		t.Fatal("aborted quest still in the journal")
	}
}

// TestJournalEmitOrderOnBothExecutors pins the packets of the set cond,
// exit and abort emit paths, in order, on the inline and the pool
// executors.
func TestJournalEmitOrderOnBothExecutors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		opts []gameservertest.Option
	}{
		{"inline", nil},
		{"pool", []gameservertest.Option{gameservertest.WithRealPool()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootJournal(t, tc.opts...)
			enterWorld(t, srv)
			srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
				st := q.NewState(c, sc)
				st.SetStatus(questlog.StatusStarted)
				st.SetCond(1)
				st.SetCond(2)
				st.Exit(false)
			})
			srv.RunQuest(t, objID, "Q002_WhatWomenWant", func(q *script.Quests, c *player.Character, sc *script.Script) {
				st := q.NewState(c, sc)
				st.SetStatus(questlog.StatusStarted)
				st.SetCond(1)
			})
			srv.Client.Send(encodeRequestQuestAbort(2))
			want := []string{
				"S QuestList 1:0x80000001", "S ExShowQuestMark quest=1",
				"S QuestList 1:0x80000003", "S ExShowQuestMark quest=1",
				"S QuestList 1:0x00000000",
				"S QuestList 1:0x00000000 2:0x80000001", "S ExShowQuestMark quest=2",
				"S QuestList 1:0x00000000",
			}
			if got := sentLines(t, srv); !slices.Equal(got, want) {
				t.Fatalf("packets:\n got %q\nwant %q", got, want)
			}
			srv.FlushPersistence(t)
			if got := questRows(t, srv, objID, q001); !maps.Equal(got, map[string]string{"<state>": "COMPLETED"}) {
				t.Fatalf("Q001 rows = %v", got)
			}
			if got := questRows(t, srv, objID, "Q002_WhatWomenWant"); len(got) != 0 {
				t.Fatalf("Q002 rows = %v", got)
			}
		})
	}
}

// TestJournalWritesBeforeDetachLandBeforeReselect writes while the player's
// persistence lane is held, then relogs: the writes land before the next
// selection loads the journal, which shows them.
func TestJournalWritesBeforeDetachLandBeforeReselect(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t)
	enterWorld(t, srv)
	release := srv.HoldPersistenceLane(t, objID)
	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		st := q.NewState(c, sc)
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(2)
	})
	sentLines(t, srv)
	if rows := questRows(t, srv, objID, q001); len(rows) != 0 {
		t.Fatalf("rows landed while the lane was held: %v", rows)
	}
	// The restart's detach queues its saves behind the held lane and waits
	// for them.
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	release()
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelectInfo)
	srv.Client.Send(encodeRequestGameStart(0))
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelected)
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	assertQuestList(t, "reselected", srv.Client.Read(), []questLine{{1, -0x7ffffffd}})
}

// TestDetachingPlayerGetsNoJournalWrite keeps a character's journal past
// its detach and writes to it: the journal is sealed, so nothing changes,
// nothing is written and nothing is sent.
func TestDetachingPlayerGetsNoJournalWrite(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t)
	insertJournal(t, srv.DB,
		journalRow{objID, q001, "<state>", val("STARTED")},
		journalRow{objID, q001, "<cond>", val("1")},
	)
	enterWorld(t, srv)
	var (
		left *script.QuestState
		char *player.Character
	)
	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		left = q.State(c, sc)
		char = c
	})
	if char.Detaching() {
		t.Fatal("a character in the world reports detaching")
	}
	restart(t, srv)
	if !char.Detaching() {
		t.Fatal("a detached character does not report detaching")
	}
	srv.FlushPersistence(t)
	srv.TakeJournalWrites()

	left.Set("ex", "1")
	left.SetCond(2)
	left.Exit(true)
	srv.FlushPersistence(t)
	if writes := srv.TakeJournalWrites(); len(writes) != 0 {
		t.Fatalf("a detached journal wrote %v", writes)
	}
	if got := questRows(t, srv, objID, q001); !maps.Equal(got, map[string]string{"<state>": "STARTED", "<cond>": "1"}) {
		t.Fatalf("rows = %v", got)
	}
	srv.Client.ExpectNoFrame()
}

// TestFailedSealedDrainRefusesSelection makes journal writes fail while a
// character sets a condition and leaves: its next selection is refused
// silently, since loading would lose the writes. Once writes succeed again
// the next selection applies them first and loads them.
func TestFailedSealedDrainRefusesSelection(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t, gameservertest.WithCapturedLog())
	enterWorld(t, srv)
	srv.FailJournalWrites(errors.New("database away"))
	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		st := q.NewState(c, sc)
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
	})
	sentLines(t, srv)
	restart(t, srv)

	srv.Client.Send(encodeRequestGameStart(0))
	srv.Client.ExpectNoFrame()
	if _, ok := srv.State.Player(objID); ok {
		t.Fatal("refused selection registered the player")
	}
	if rows := questRows(t, srv, objID, q001); len(rows) != 0 {
		t.Fatalf("rows = %v while writes fail", rows)
	}

	srv.FailJournalWrites(nil)
	srv.Client.Send(encodeRequestGameStart(0))
	readUntil(t, srv.Client, serverpackets.OpcodeCharSelected)
	srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	assertQuestList(t, "after the writes landed", srv.Client.Read(), []questLine{{1, -0x7fffffff}})
	if got := questRows(t, srv, objID, q001); !maps.Equal(got, map[string]string{"<state>": "STARTED", "<cond>": "1"}) {
		t.Fatalf("rows = %v", got)
	}
}

// TestShutdownDrainsSealedJournals leaves a character whose journal writes
// failed, then shuts the server down with writes working again: the
// shutdown's last drain writes them.
func TestShutdownDrainsSealedJournals(t *testing.T) {
	t.Parallel()
	srv, objID := bootJournal(t, gameservertest.WithCapturedLog())
	enterWorld(t, srv)
	srv.FailJournalWrites(errors.New("database away"))
	srv.RunQuest(t, objID, q001, func(q *script.Quests, c *player.Character, sc *script.Script) {
		st := q.NewState(c, sc)
		st.SetStatus(questlog.StatusStarted)
		st.Set("ex", "2")
	})
	restart(t, srv)
	srv.FlushPersistence(t)
	if rows := questRows(t, srv, objID, q001); len(rows) != 0 {
		t.Fatalf("rows = %v while writes fail", rows)
	}
	srv.FailJournalWrites(nil)
	srv.Shutdown(t)
	var n int
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM character_quests WHERE charId=? AND name=?", objID, q001).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("rows after shutdown = %d, want 2", n)
	}
}

func encodeRequestQuestAbort(questID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestQuestAbort)
	w.WriteInt32(questID)
	return w.Bytes()
}
