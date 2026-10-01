package social

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/rs/zerolog"
)

// Chat channels, as the client numbers them.
const (
	sayAll              int32 = 0
	sayShout            int32 = 1
	sayTell             int32 = 2
	sayParty            int32 = 3
	sayClan             int32 = 4
	sayGM               int32 = 5
	sayTrade            int32 = 8
	sayAlliance         int32 = 9
	sayAnnouncement     int32 = 10
	sayPartyRoomCommand int32 = 15
	sayPartyRoomAll     int32 = 16
	sayHeroVoice        int32 = 17
	sayCritical         int32 = 18
)

// farX is a position a tile east of the class template's spawn point,
// outside every chat range of it.
const farX = 10 + 40000

func encodeSay2(typ int32, text string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeTell(text, target string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSay2)
	w.WriteString(text)
	w.WriteInt32(sayTell)
	w.WriteString(target)
	return w.Bytes()
}

// readSay reads c's frames up to its next CreatureSay and returns it.
// Frames that are no chat line (party positions, status updates) are
// skipped.
func readSay(t *testing.T, c *testsupport.ScriptedClient) []byte {
	t.Helper()
	for range 50 {
		if frame := c.Read(); frame[0] == serverpackets.OpcodeCreatureSay {
			return frame
		}
	}
	t.Fatal("no CreatureSay within 50 frames")
	return nil
}

// assertSay reads c's next chat line and checks it, by hand from the
// reference layout: speaker object id, channel, name, text.
func assertSay(t *testing.T, c *testsupport.ScriptedClient, objectID, typ int32, name, text string) {
	t.Helper()
	r := wire.NewReader(readSay(t, c)[1:])
	gotID, gotType, gotName, gotText := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadString()
	if gotID != objectID || gotType != typ || gotName != name || gotText != text {
		t.Fatalf("CreatureSay = (%d, %d, %q, %q), want (%d, %d, %q, %q)", gotID, gotType, gotName, gotText, objectID, typ, name, text)
	}
	if r.Remaining() != 0 {
		t.Fatalf("CreatureSay has %d trailing bytes", r.Remaining())
	}
}

// assertNoSay fails when c receives a chat line before the server goes
// quiet.
func assertNoSay(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	for _, frame := range drainFrames(t, c) {
		if frame[0] == serverpackets.OpcodeCreatureSay {
			t.Fatalf("%s: received a chat line, want none", what)
		}
	}
}

// enterAt seeds name on account standing at (x, y, z) and enters it into
// the world.
func (p *pair) enterAt(t *testing.T, account, name string, x, y, z int) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	id := p.srv.SeedCharacterFor(t, account, name, 1, 0).ID
	if _, err := p.srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", x, y, z, id); err != nil {
		t.Fatalf("place %s: %v", name, err)
	}
	c := p.srv.DialClient(t, account, 1)
	startInWorld(t, c)
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, p.bobby)
	drainUntilQuiet(t, c)
	return c, id
}

func gmLevels(t *testing.T) gameservertest.Option {
	t.Helper()
	levels, err := admin.NewData([]admin.AccessLevel{
		{Level: 0, Name: "User", NameColor: "FFFFFF", TitleColor: "FFFF77", AllowTransaction: true},
		{Level: 7, Name: "GM", NameColor: "FFFFFF", TitleColor: "FFFF77", IsGM: true, AllowTransaction: true},
	}, nil)
	if err != nil {
		t.Fatalf("admin.NewData: %v", err)
	}
	return gameservertest.WithAdmin(levels)
}

// TestChatAllHeardInRange pins the general channel: the speaker and every
// player within 1250 units hear the line, a player further away does not.
// The line is delivered with every literal backslash-n removed.
func TestChatAllHeardInRange(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)
	carol, _ := p.enterAt(t, "player3", "Carol", farX, 20, gameservertest.SpawnZ)

	p.alice.Send(encodeSay2(sayAll, `hel\nlo`))
	assertSay(t, p.alice, p.aliceID, sayAll, "Alice", "hello")
	assertSay(t, p.bobby, p.aliceID, sayAll, "Alice", "hello")
	assertNoSay(t, carol, "player out of range")
	assertSilent(t, p.alice, "speaker after its line")
	assertSilent(t, p.bobby, "listener after the line")
}

// TestChatShoutAndTradeReachRegion pins the shout and trade channels: the
// line reaches every player whose position lies in the speaker's restart
// region, however far, and nobody in another region. Players outside every
// region share one.
func TestChatShoutAndTradeReachRegion(t *testing.T) {
	farTile := location.Point{X: (farX-world.MinX)/world.TileSize + world.TileXMin, Y: (20-world.MinY)/world.TileSize + world.TileYMin}
	table := &restart.Table{Points: []restart.Point{{Name: "far", MapRegions: []location.Point{farTile}}}}
	if table.PointIndexAt(location.Location{X: 10, Y: 20}) != -1 {
		t.Fatal("the spawn point lies in the far region")
	}
	p := bootPair(t, gameservertest.WithRestartPoints(table))
	p.enterAll(t)
	carol, carolID := p.enterAt(t, "player3", "Carol", farX, 20, gameservertest.SpawnZ)
	dave, _ := p.enterAt(t, "player4", "Dave", farX+5000, 20, gameservertest.SpawnZ)

	for _, typ := range []int32{sayShout, sayTrade} {
		p.alice.Send(encodeSay2(typ, "wts"))
		assertSay(t, p.alice, p.aliceID, typ, "Alice", "wts")
		assertSay(t, p.bobby, p.aliceID, typ, "Alice", "wts")
		assertNoSay(t, carol, "player in another region")
		assertNoSay(t, dave, "player in another region")

		carol.Send(encodeSay2(typ, "wtb"))
		assertSay(t, carol, carolID, typ, "Carol", "wtb")
		assertSay(t, dave, carolID, typ, "Carol", "wtb")
		assertNoSay(t, p.alice, "player in another region")
		assertNoSay(t, p.bobby, "player in another region")
	}

	// The general channel stays within its range in the same region.
	carol.Send(encodeSay2(sayAll, "hi"))
	assertSay(t, carol, carolID, sayAll, "Carol", "hi")
	assertNoSay(t, dave, "same region, out of general range")
}

// TestChatTell pins whispers: the target hears the line under the
// speaker's name, the speaker sees it addressed "->" to the target's own
// name. A name nobody online carries answers TARGET_IS_NOT_FOUND_IN_THE_GAME;
// a target blocking everything answers S1_BLOCKED_EVERYTHING naming it, and
// one blocking the speaker THE_PERSON_IS_IN_MESSAGE_REFUSAL_MODE. A game
// master's whisper passes both blocks.
func TestChatTell(t *testing.T) {
	p := bootPair(t, gmLevels(t))
	carolID := p.srv.SeedCharacterFor(t, "player3", "Carol", 1, 0).ID
	p.setAccessLevel(t, carolID, 7)
	p.enterAll(t)
	carol := p.srv.DialClient(t, "player3", 1)
	startInWorld(t, carol)
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, p.bobby)
	drainUntilQuiet(t, carol)

	p.alice.Send(encodeTell("psst", "bobby"))
	assertSay(t, p.bobby, p.aliceID, sayTell, "Alice", "psst")
	assertSay(t, p.alice, p.aliceID, sayTell, "->Bobby", "psst")
	assertNoSay(t, carol, "bystander")

	p.alice.Send(encodeTell("psst", "Nobody"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageTargetNotFound)
	p.alice.Send(encodeTell("psst", ""))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageTargetNotFound)

	p.bobby.Send(encodeBlock(clientpackets.BlockAll, ""))
	drainUntilQuiet(t, p.bobby)
	p.alice.Send(encodeTell("psst", "Bobby"))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1BlockedEverything, "Bobby")
	assertNoSay(t, p.bobby, "target blocking everything")
	carol.Send(encodeTell("gm", "Bobby"))
	assertSay(t, p.bobby, carolID, sayTell, "Carol", "gm")
	assertSay(t, carol, carolID, sayTell, "->Bobby", "gm")

	p.bobby.Send(encodeBlock(clientpackets.BlockAllRelease, ""))
	p.bobby.Send(encodeBlock(clientpackets.BlockAdd, "Alice"))
	p.bobby.Send(encodeBlock(clientpackets.BlockAdd, "Carol"))
	drainUntilQuiet(t, p.bobby)
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, carol)
	p.alice.Send(encodeTell("psst", "Bobby"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageInMessageRefusalMode)
	assertNoSay(t, p.bobby, "target blocking the speaker")
	carol.Send(encodeTell("gm", "Bobby"))
	assertSay(t, p.bobby, carolID, sayTell, "Carol", "gm")
	assertSay(t, carol, carolID, sayTell, "->Bobby", "gm")
}

// TestChatDroppedLines pins the lines the reference drops without any
// answer: an empty line, one over 100 characters, a channel the client
// cannot name, an announcement from a player that is no game master, and a
// channel nobody delivers. A 100-character line is said.
func TestChatDroppedLines(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	for _, tc := range []struct {
		what    string
		payload []byte
	}{
		{"empty line", encodeSay2(sayAll, "")},
		{"101 characters", encodeSay2(sayAll, strings.Repeat("a", 101))},
		{"unknown channel", encodeSay2(19, "hi")},
		{"negative channel", encodeSay2(-1, "hi")},
		{"announcement", encodeSay2(sayAnnouncement, "hi")},
		{"critical announcement", encodeSay2(sayCritical, "hi")},
		{"GM channel", encodeSay2(sayGM, "hi")},
		{"alliance without an alliance", encodeSay2(sayAlliance, "hi")},
		{"party without a party", encodeSay2(sayParty, "hi")},
		{"clan without a clan", encodeSay2(sayClan, "hi")},
		{"command channel without a channel", encodeSay2(sayPartyRoomAll, "hi")},
		{"command channel commander without a channel", encodeSay2(sayPartyRoomCommand, "hi")},
		{"hero voice from a non-hero", encodeSay2(sayHeroVoice, "hi")},
	} {
		p.alice.Send(tc.payload)
		assertSilent(t, p.alice, tc.what)
		assertSilent(t, p.bobby, tc.what)
	}

	full := strings.Repeat("a", 100)
	p.alice.Send(encodeSay2(sayAll, full))
	assertSay(t, p.alice, p.aliceID, sayAll, "Alice", full)
	assertSay(t, p.bobby, p.aliceID, sayAll, "Alice", full)
}

// TestChatHeroVoice pins the hero channel: a hero's line reaches every
// player online, however far.
func TestChatHeroVoice(t *testing.T) {
	p := bootPair(t)
	if _, err := p.srv.DB.ExecContext(context.Background(), "UPDATE characters SET hero = 1 WHERE obj_Id = ?", p.bobbyID); err != nil {
		t.Fatal(err)
	}
	p.enterAll(t)
	carol, _ := p.enterAt(t, "player3", "Carol", farX, 20, gameservertest.SpawnZ)

	p.bobby.Send(encodeSay2(sayHeroVoice, "glory"))
	assertSay(t, p.alice, p.bobbyID, sayHeroVoice, "Bobby", "glory")
	assertSay(t, p.bobby, p.bobbyID, sayHeroVoice, "Bobby", "glory")
	assertSay(t, carol, p.bobbyID, sayHeroVoice, "Bobby", "glory")
}

// TestChatFloodProtection pins the chat reuse delays: general and shout
// share GlobalChatTime, trade has TradeChatTime and the hero channel
// HeroVoiceTime. A line inside its delay is dropped without an answer; the
// delay is per client.
func TestChatFloodProtection(t *testing.T) {
	p := bootPair(t, gameservertest.WithChat(network.ChatConfig{GlobalDelay: time.Hour, TradeDelay: time.Hour, HeroVoiceDelay: time.Hour}))
	if _, err := p.srv.DB.ExecContext(context.Background(), "UPDATE characters SET hero = 1 WHERE obj_Id = ?", p.aliceID); err != nil {
		t.Fatal(err)
	}
	p.enterAll(t)

	p.alice.Send(encodeSay2(sayAll, "one"))
	assertSay(t, p.alice, p.aliceID, sayAll, "Alice", "one")
	assertSay(t, p.bobby, p.aliceID, sayAll, "Alice", "one")
	p.alice.Send(encodeSay2(sayAll, "two"))
	p.alice.Send(encodeSay2(sayShout, "three"))
	assertSilent(t, p.alice, "general and shout inside the global delay")
	assertSilent(t, p.bobby, "general and shout inside the global delay")

	p.bobby.Send(encodeSay2(sayShout, "mine"))
	assertSay(t, p.alice, p.bobbyID, sayShout, "Bobby", "mine")
	assertSay(t, p.bobby, p.bobbyID, sayShout, "Bobby", "mine")

	for _, typ := range []int32{sayTrade, sayHeroVoice} {
		p.alice.Send(encodeSay2(typ, "first"))
		assertSay(t, p.alice, p.aliceID, typ, "Alice", "first")
		assertSay(t, p.bobby, p.aliceID, typ, "Alice", "first")
		p.alice.Send(encodeSay2(typ, "second"))
		assertSilent(t, p.alice, "second line inside its delay")
		assertSilent(t, p.bobby, "second line inside its delay")
	}

	// Whispers have no delay.
	for range 2 {
		p.alice.Send(encodeTell("psst", "Bobby"))
		assertSay(t, p.bobby, p.aliceID, sayTell, "Alice", "psst")
		assertSay(t, p.alice, p.aliceID, sayTell, "->Bobby", "psst")
	}
}

// TestChatWalkerProtection pins L2WalkerProtection: with it on, a whisper
// starting with a bot script command is dropped without an answer; other
// channels and other whispers are said.
func TestChatWalkerProtection(t *testing.T) {
	p := bootPair(t, gameservertest.WithChat(network.ChatConfig{WalkerProtection: true}))
	p.enterAll(t)

	p.alice.Send(encodeTell("USESKILL 1", "Bobby"))
	assertSilent(t, p.alice, "bot whisper")
	assertSilent(t, p.bobby, "bot whisper")

	p.alice.Send(encodeTell("use skill", "Bobby"))
	assertSay(t, p.bobby, p.aliceID, sayTell, "Alice", "use skill")
	assertSay(t, p.alice, p.aliceID, sayTell, "->Bobby", "use skill")

	p.alice.Send(encodeSay2(sayAll, "USESKILL 1"))
	assertSay(t, p.alice, p.aliceID, sayAll, "Alice", "USESKILL 1")
	assertSay(t, p.bobby, p.aliceID, sayAll, "Alice", "USESKILL 1")
}

// TestChatParty pins the party channel: every member hears the line, the
// speaker included, even one that blocks the speaker; a player outside the
// party hears nothing.
func TestChatParty(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)
	carol, _ := p.third(t, "player3", "Carol")

	p.alice.Send(encodeJoinParty("Bobby"))
	drainUntilQuiet(t, p.alice)
	p.bobby.Send(encodeAnswerJoinParty(1))
	p.bobby.Send(encodeBlock(clientpackets.BlockAdd, "Alice"))
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, p.bobby)
	drainUntilQuiet(t, carol)

	p.alice.Send(encodeSay2(sayParty, "pull"))
	assertSay(t, p.alice, p.aliceID, sayParty, "Alice", "pull")
	assertSay(t, p.bobby, p.aliceID, sayParty, "Alice", "pull")
	assertNoSay(t, carol, "player outside the party")

	// A party member leads no command channel and is in none.
	p.alice.Send(encodeSay2(sayPartyRoomAll, "cc"))
	p.alice.Send(encodeSay2(sayPartyRoomCommand, "cc"))
	assertNoSay(t, p.alice, "command channel line outside a channel")
	assertNoSay(t, p.bobby, "command channel line outside a channel")
}

// TestChatClan pins the clan channel: every member in the world hears the
// line, the speaker included; a player outside the clan hears nothing.
func TestChatClan(t *testing.T) {
	p := bootPair(t, gameservertest.WithClanSeed(func(db *sql.DB) {
		for _, q := range []string{
			"UPDATE characters SET clanid = 268435456, power_grade = 0 WHERE char_name = 'Alice'",
			"INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT 268435456, 'Chatters', 0, obj_Id FROM characters WHERE char_name = 'Alice'",
		} {
			if _, err := db.ExecContext(context.Background(), q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	}))
	// Alice enters as a clan leader, whose burst carries the clan's own
	// packets.
	for _, c := range []*testsupport.ScriptedClient{p.alice, p.bobby} {
		c.Send(encodeRequestGameStart(0))
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
		drainUntilQuiet(t, c)
	}
	carol, _ := p.third(t, "player3", "Carol")

	p.alice.Send(encodeJoinPledge(p.bobbyID))
	drainUntilQuiet(t, p.bobby)
	p.bobby.Send(encodeAnswerJoinPledge(1))
	drainUntilQuiet(t, p.bobby)
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, carol)

	p.bobby.Send(encodeSay2(sayClan, "for the clan"))
	assertSay(t, p.alice, p.bobbyID, sayClan, "Bobby", "for the clan")
	assertSay(t, p.bobby, p.bobbyID, sayClan, "Bobby", "for the clan")
	assertNoSay(t, carol, "player outside the clan")
}

// TestChatLog pins LogChat: every said line is logged with its channel and
// speaker (and, for a whisper, the name it was addressed to as typed),
// with its text as said; a delivered friend message is logged as PRIV_MSG.
// A dropped line is not logged.
func TestChatLog(t *testing.T) {
	var log syncBuffer
	p := bootPair(t, gameservertest.WithChat(network.ChatConfig{Log: zerolog.New(&log)}))
	p.enterAll(t)

	p.alice.Send(encodeSay2(sayAll, `a\nb`))
	readSay(t, p.alice)
	readSay(t, p.bobby)
	p.alice.Send(encodeSay2(sayAll, ""))
	p.alice.Send(encodeTell("psst", "bobby"))
	readSay(t, p.bobby)
	readSay(t, p.alice)
	p.alice.Send(encodeTell("psst", "Nobody"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageTargetNotFound)

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest")
	p.bobby.Send(encodeAnswerFriendInvite(1))
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, p.bobby)
	p.alice.Send(encodeFriendSay("hey", "Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeL2FriendSay, "L2FriendSay")

	want := []string{`ALL [Alice] a\nb`, "TELL [Alice to bobby] psst", "TELL [Alice to Nobody] psst", "PRIV_MSG [Alice to Bobby] hey"}
	if got := log.messages(t); !slices.Equal(got, want) {
		t.Fatalf("chat log = %q, want %q", got, want)
	}
}

// syncBuffer is a log sink the server writes while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// messages returns the message of every record written so far.
func (b *syncBuffer) messages(t *testing.T) []string {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("chat log record %q: %v", line, err)
		}
		msg, _ := record[zerolog.MessageFieldName].(string)
		out = append(out, msg)
	}
	return out
}

func encodeJoinParty(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeAnswerJoinParty(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeJoinPledge(target int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinPledge)
	w.WriteInt32(target)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeAnswerJoinPledge(answer int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinPledge)
	w.WriteInt32(answer)
	return w.Bytes()
}
