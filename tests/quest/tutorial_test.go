package quest

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const tutorial = "Tutorial"

// tutorialLog records the events the test tutorial quest hears. Its hook
// runs on whichever goroutine raises the event.
type tutorialLog struct {
	mu     sync.Mutex
	events []string
}

func (l *tutorialLog) add(name string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, name)
}

// take returns the events heard since the last call.
func (l *tutorialLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.events
	l.events = nil
	return out
}

// tutorialScripts registers a tutorial quest under the datapack's path. Its
// event hook records every event and answers through the tutorial helpers:
// UC shows question mark 1, QM<n> question mark n, CE<n> enables client
// event n, and the commands "page <file>", "close", "voice <file>",
// "radar x y z", "unradar x y z", "memo <key> <value>" and "unmemo <key>"
// drive the helper they name.
func tutorialScripts(heard *tutorialLog) gameservertest.Option {
	hook := func(_ *script.Script, e script.Event) string {
		heard.add(e.Name)
		p, f := e.Player, strings.Fields(e.Name)
		num := func(s string) int32 {
			n, _ := strconv.Atoi(s)
			return int32(n)
		}
		switch {
		case e.Name == "UC":
			p.ShowQuestionMark(1)
		case strings.HasPrefix(e.Name, "QM"):
			p.ShowQuestionMark(num(e.Name[2:]))
		case strings.HasPrefix(e.Name, "CE"):
			p.EnableTutorialEvent(num(e.Name[2:]))
		case f[0] == "page":
			p.ShowTutorialHTML(f[1])
		case f[0] == "close":
			p.CloseTutorialHTML()
		case f[0] == "voice":
			p.PlayTutorialVoice(f[1])
		case f[0] == "radar":
			p.AddRadarMarker(num(f[1]), num(f[2]), num(f[3]))
		case f[0] == "unradar":
			p.RemoveRadarMarker(num(f[1]), num(f[2]), num(f[3]))
		case f[0] == "memo":
			p.SetMemo(f[1], f[2])
		case f[0] == "unmemo":
			p.UnsetMemo(f[1])
		}
		return ""
	}
	path := "script.feature." + tutorial
	return gameservertest.WithScripts([]script.Listing{{Path: path}}, script.Catalog{
		path: func() script.Script { return script.Script{QuestID: -1, Hooks: script.Hooks{OnEvent: hook}} },
	})
}

// bootTutorial boots one character with the tutorial quest; started seeds
// the character's tutorial quest state, started.
func bootTutorial(t *testing.T, started bool, opts ...gameservertest.Option) (*gameservertest.Server, int32, *tutorialLog) {
	t.Helper()
	heard := &tutorialLog{}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		tutorialScripts(heard),
	}, opts...)...)
	objID := srv.SoleObjectID(t)
	if started {
		insertJournal(t, srv.DB, journalRow{objID, tutorial, questlog.KeyState, val("STARTED")})
	}
	return srv, objID, heard
}

func encodeTutorialString(opcode byte, s string) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteString(s)
	return w.Bytes()
}

func encodeTutorialNumber(opcode byte, n int32) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(n)
	return w.Bytes()
}

// payloadOf is frame's packet without its length header.
func payloadOf(frame wire.Frame) []byte {
	defer frame.Release()
	return append([]byte(nil), frame.Bytes()[2:]...)
}

// expectFrames fails unless got holds exactly want's packets, in order.
func expectFrames(t *testing.T, what string, got [][]byte, want ...wire.Frame) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d frames %x, want %d", what, len(got), got, len(want))
	}
	for i, w := range want {
		if p := payloadOf(w); !bytes.Equal(got[i], p) {
			t.Fatalf("%s: frame %d = %x, want %x", what, i, got[i], p)
		}
	}
}

// TestTutorialRequestsReachTheTutorialQuest sends the four tutorial
// requests. Each hands the tutorial quest its event, the link and the
// command as sent, the question mark as QM<n> and the client event as
// CE<n>, and the quest answers through the tutorial window: a page (the
// missing-page notice for a page that does not exist), the window's close,
// a question mark, a client event, the tutorial voice bound to the player
// and the radar marker's packets.
func TestTutorialRequestsReachTheTutorialQuest(t *testing.T) {
	t.Parallel()
	srv, objID, heard := bootTutorial(t, true, gameservertest.WithHTMLPages(map[string]string{
		"script/feature/Tutorial/tutorial_02.htm": "<html><body>step two</body></html>",
	}))
	c := srv.Client
	enterWorld(t, srv)
	heard.take()
	var at location.Location
	onCharacter(t, srv, objID, func(c *player.Character) { at = c.CurrentLocation() })

	for _, tc := range []struct {
		send  []byte
		event string
		want  []wire.Frame
	}{
		{
			encodeTutorialString(clientpackets.OpcodeRequestTutorialLinkHTML, "page tutorial_02.htm"), "page tutorial_02.htm",
			[]wire.Frame{serverpackets.FrameTutorialShowHTML("<html><body>step two</body></html>\n")},
		},
		{
			encodeTutorialString(clientpackets.OpcodeRequestTutorialLinkHTML, "page missing.htm"), "page missing.htm",
			[]wire.Frame{serverpackets.FrameTutorialShowHTML("<html><body>My html is missing:<br>data/html/script/feature/Tutorial/missing.htm</body></html>")},
		},
		{
			encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "close"), "close",
			[]wire.Frame{serverpackets.FrameTutorialCloseHTML()},
		},
		{
			encodeTutorialNumber(clientpackets.OpcodeRequestTutorialQuestionMark, 26), "QM26",
			[]wire.Frame{serverpackets.FrameTutorialShowQuestionMark(26)},
		},
		{
			encodeTutorialNumber(clientpackets.OpcodeRequestTutorialClientEvent, 8), "CE8",
			[]wire.Frame{serverpackets.FrameTutorialEnableClientEvent(8)},
		},
		{
			encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "voice tutorial_voice_006"), "voice tutorial_voice_006",
			[]wire.Frame{serverpackets.FramePlaySoundAt(serverpackets.Sound{Type: 2, File: "tutorial_voice_006", BindToObject: true, ObjectID: objID, Location: at})},
		},
		{
			encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "radar -84318 244579 -3730"), "radar -84318 244579 -3730",
			[]wire.Frame{
				serverpackets.FrameRadarControl(serverpackets.Radar{Show: 2, Type: 2, X: -84318, Y: 244579, Z: -3730}),
				serverpackets.FrameRadarControl(serverpackets.Radar{Show: 0, Type: 1, X: -84318, Y: 244579, Z: -3730}),
			},
		},
		{
			encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "unradar -84318 244579 -3730"), "unradar -84318 244579 -3730",
			[]wire.Frame{serverpackets.FrameRadarControl(serverpackets.Radar{Show: 1, Type: 1, X: -84318, Y: 244579, Z: -3730})},
		},
	} {
		c.Send(tc.send)
		expectFrames(t, tc.event, srv.ReadQueued(t, c), tc.want...)
		if got := heard.take(); !slices.Equal(got, []string{tc.event}) {
			t.Fatalf("heard %q, want %q", got, tc.event)
		}
	}
}

// TestTutorialRequestsWithoutTutorialStateAnswerNothing: a player with no
// tutorial quest state sends the four requests; the quest hears nothing
// and nothing is sent back, not even an ActionFailed.
func TestTutorialRequestsWithoutTutorialStateAnswerNothing(t *testing.T) {
	t.Parallel()
	srv, _, heard := bootTutorial(t, false)
	c := srv.Client
	enterWorld(t, srv)
	c.Send(encodeTutorialString(clientpackets.OpcodeRequestTutorialLinkHTML, "page tutorial_02.htm"))
	c.Send(encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "close"))
	c.Send(encodeTutorialNumber(clientpackets.OpcodeRequestTutorialQuestionMark, 1))
	c.Send(encodeTutorialNumber(clientpackets.OpcodeRequestTutorialClientEvent, 1))
	if got := srv.ReadQueued(t, c); len(got) != 0 {
		t.Fatalf("sent %x, want nothing", got)
	}
	if got := heard.take(); len(got) != 0 {
		t.Fatalf("heard %q without a tutorial state", got)
	}
}

// TestTutorialHearsEnterWorldLastInTheBurst: the tutorial quest hears UC
// once, at the end of the EnterWorld burst, so its answer lands just ahead
// of the burst's closing ActionFailed.
func TestTutorialHearsEnterWorldLastInTheBurst(t *testing.T) {
	t.Parallel()
	srv, _, heard := bootTutorial(t, true)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	burst := srv.ReadQueued(t, c)
	last := slices.IndexFunc(burst, func(f []byte) bool { return f[0] == serverpackets.OpcodeTutorialShowQuestionMark })
	if last < 0 || last+2 != len(burst) {
		t.Fatalf("question mark at %d of %d burst frames, want second to last", last, len(burst))
	}
	expectFrames(t, "burst tail", burst[last:], serverpackets.FrameTutorialShowQuestionMark(1), serverpackets.FrameActionFailed())
	if got := heard.take(); !slices.Equal(got, []string{"UC"}) {
		t.Fatalf("heard %q, want [UC]", got)
	}
}

// TestTutorialHearsTheGameTriggers drives the game's tutorial triggers on
// the player's queue. An HP write that leaves the player below 30% of its
// maximum raises CE45 once, one that does not raises nothing; a death from
// HP damage raises CE45 for the damage's write, CE45 for the death's own
// HP clear, then CE30, but no CE30 in the Olympiad; a level gain raises
// CE40 once, a loss nothing; sitting down raises CE8388608, on request and
// through Sit.
func TestTutorialHearsTheGameTriggers(t *testing.T) {
	t.Parallel()
	levels := flatLevels(t)
	srv, objID, heard := bootTutorial(t, true, gameservertest.WithLevels(levels))
	enterWorld(t, srv)
	heard.take()
	onPlayer := func(fn func(c *player.Character)) []string {
		t.Helper()
		onCharacter(t, srv, objID, fn)
		srv.ReadQueued(t, srv.Client)
		return heard.take()
	}
	expect := func(what string, got []string, want ...string) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Fatalf("%s: heard %q, want %q", what, got, want)
		}
	}

	expect("HP at 50%", onPlayer(func(c *player.Character) { c.SetHP(c.MaxHPValue() * 0.5) }))
	expect("HP at 20%", onPlayer(func(c *player.Character) { c.SetHP(c.MaxHPValue() * 0.2) }), "CE45")
	expect("lethal hit", onPlayer(func(c *player.Character) { c.TakeDamage(1_000_000, c) }), "CE45", "CE45", "CE30")

	onPlayer(func(c *player.Character) {
		c.Revive()
		c.SetHP(c.MaxHPValue())
		c.SetOlympiadMode(true)
	})
	expect("lethal hit in the Olympiad", onPlayer(func(c *player.Character) { c.TakeDamage(1_000_000, c) }), "CE45", "CE45")
	onPlayer(func(c *player.Character) {
		c.SetOlympiadMode(false)
		c.Revive()
		c.SetHP(c.MaxHPValue())
	})

	var level int
	expect("level gain", onPlayer(func(c *player.Character) {
		c.AddExpAndSp(levels.RequiredExpForLevel(4), 0)
		level = c.Level()
	}), "CE40")
	if level != 4 {
		t.Fatalf("level after the exp = %d, want 4", level)
	}
	expect("level loss", onPlayer(func(c *player.Character) {
		c.RemoveExpAndSp(levels, nil, c.Exp-levels.RequiredExpForLevel(2), 0)
		level = c.Level()
	}))
	if level != 2 {
		t.Fatalf("level after the loss = %d, want 2", level)
	}

	srv.Client.Send(encodeTutorialSit())
	srv.ReadQueued(t, srv.Client)
	expect("sit request", heard.take(), "CE8388608")
	// A store or a relax effect seats the player through Sit, which tells
	// the tutorial even when the player already sits.
	expect("seated again", onPlayer(func(c *player.Character) { c.Sit() }), "CE8388608")
}

func encodeTutorialSit() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(0)
	return w.Bytes()
}

// TestTutorialHearsAdenaPickup: picking adena up raises CE57 between the
// pickup's GetItem and the item's DeleteObject; another item raises
// nothing.
func TestTutorialHearsAdenaPickup(t *testing.T) {
	t.Parallel()
	srv, objID, heard := bootTutorial(t, true)
	c := srv.Client
	enterWorld(t, srv)
	heard.take()
	var x, y, z int
	onCharacter(t, srv, objID, func(c *player.Character) { x, y, z = c.Position() })

	pickUp := func(templateID int32) [][]byte {
		t.Helper()
		srv.SeedGroundItem(t, 0, templateID, 10, x, y, z)
		srv.ReadQueued(t, c)
		snaps := srv.GroundItems.Snapshots(nil)
		if len(snaps) != 1 {
			t.Fatalf("ground items = %d, want 1", len(snaps))
		}
		c.Send(encodeTutorialAction(snaps[0].ObjectID, location.Location{X: x, Y: y, Z: z}))
		return srv.ReadQueued(t, c)
	}

	frames := pickUp(item.AdenaID)
	get := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeGetItem })
	if get < 0 || get+2 >= len(frames) {
		t.Fatalf("pickup frames %x: no GetItem followed by two frames", frames)
	}
	expectFrames(t, "after GetItem", frames[get+1:get+2], serverpackets.FrameTutorialEnableClientEvent(item.AdenaID))
	if frames[get+2][0] != serverpackets.OpcodeDeleteObject {
		t.Fatalf("frame after the tutorial's = %x, want DeleteObject", frames[get+2])
	}
	if got := heard.take(); !slices.Equal(got, []string{"CE57"}) {
		t.Fatalf("heard %q, want [CE57]", got)
	}

	// The pickup holds the player for 200 ms.
	srv.Advance(t, 250*time.Millisecond)
	if frames := pickUp(20); !slices.ContainsFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeGetItem }) {
		t.Fatalf("potion pickup frames %x: no GetItem", frames)
	}
	if got := heard.take(); len(got) != 0 {
		t.Fatalf("potion pickup: heard %q, want nothing", got)
	}
}

func encodeTutorialAction(objectID int32, at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteUint8(0)
	return w.Bytes()
}

// TestMemosSurviveRelog: a memo the tutorial quest sets is saved as a
// character_memo row and loads again at the next selection; unsetting it
// deletes the row.
func TestMemosSurviveRelog(t *testing.T) {
	t.Parallel()
	srv, objID, _ := bootTutorial(t, true)
	c := srv.Client
	enterWorld(t, srv)

	c.Send(encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "memo Tutorial_OneTimeQuestFlag true"))
	srv.ReadQueued(t, c)
	srv.FlushPersistence(t)
	if got := memoRows(t, srv, objID); got != "Tutorial_OneTimeQuestFlag=true" {
		t.Fatalf("memo rows = %q, want the flag", got)
	}

	restart(t, srv)
	enterWorld(t, srv)
	var v string
	var ok bool
	onCharacter(t, srv, objID, func(c *player.Character) { v, ok = c.Memos().Get("Tutorial_OneTimeQuestFlag") })
	if !ok || v != "true" {
		t.Fatalf("memo after relog = %q, %v; want true", v, ok)
	}

	c.Send(encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "unmemo Tutorial_OneTimeQuestFlag"))
	srv.ReadQueued(t, c)
	srv.FlushPersistence(t)
	if got := memoRows(t, srv, objID); got != "" {
		t.Fatalf("memo rows after unset = %q, want none", got)
	}
}

// TestMemoLoadFailureRefusesSelection: memos that cannot be read refuse the
// selection silently, as a journal that cannot be read does, so a one-time
// reward a memo records cannot be taken again.
func TestMemoLoadFailureRefusesSelection(t *testing.T) {
	t.Parallel()
	srv, objID, _ := bootTutorial(t, false, gameservertest.WithMemoLoadFault(errors.New("memos unreadable")))
	srv.Client.Send(encodeRequestGameStart(0))
	srv.Client.ExpectNoFrame()
	if _, ok := srv.State.Player(objID); ok {
		t.Fatal("refused selection registered the player in the world")
	}
}

// memoRows returns objID's character_memo rows as var=val, sorted, joined
// by commas.
func memoRows(t *testing.T, srv *gameservertest.Server, objID int32) string {
	t.Helper()
	rows, err := srv.DB.QueryContext(context.Background(), "SELECT var,val FROM character_memo WHERE charId=? ORDER BY var", objID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, k+"="+v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(out, ",")
}

// onCharacter runs fn on the online player objID's queue and waits for it.
func onCharacter(t *testing.T, srv *gameservertest.Server, objID int32, fn func(c *player.Character)) {
	t.Helper()
	srv.RunQuest(t, objID, tutorial, func(_ *script.Quests, c *player.Character, _ *script.Script) { fn(c) })
}

// flatLevels is a level table whose every level takes 1000 more exp
// than the one before.
func flatLevels(t *testing.T) *player.LevelTable {
	t.Helper()
	levels := make(map[int]player.Level, 85)
	for lvl := 1; lvl <= 85; lvl++ {
		levels[lvl] = player.Level{RequiredExpToLevelUp: int64(lvl-1) * 1000}
	}
	table, err := player.NewLevelTable(levels)
	if err != nil {
		t.Fatal(err)
	}
	return table
}
