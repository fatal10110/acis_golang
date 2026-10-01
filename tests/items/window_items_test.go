package items

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped items whose use only opens a client window or throws a die:
// 5588 Tutorial Guide and 6317 Mixing Manual (Books), 4393 Calculator
// (Calculators), 1665 World Map and 1863 Map: Elmore (Maps), 5555 Token of
// Love (SpecialXMas) and 4625 Dice (Heart) (RollingDices).
const (
	tutorialGuideID int32 = 5588
	mixingManualID  int32 = 6317
	calculatorID    int32 = 4393
	worldMapID      int32 = 1665
	elmoreMapID     int32 = 1863
	tokenOfLoveID   int32 = 5555
	heartDiceID     int32 = 4625
)

// competitionPeriod is the Seven Signs period ordinal of the seeded status
// row a test boot starts in.
const competitionPeriod = 1

// bootWindowItems boots a character holding one of each shipped window
// item, on the suite's item fixtures plus those templates. It returns the
// server, the character's object id, and each item's object id by template.
func bootWindowItems(t *testing.T, opts ...gameservertest.Option) (*gameservertest.Server, int32, map[int32]int32) {
	t.Helper()
	datapack.Require(t)
	_, shipped := shippedData()
	ids := []int32{tutorialGuideID, mixingManualID, calculatorID, worldMapID, elmoreMapID, tokenOfLoveID, heartDiceID}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range ids {
		tmpl, ok := shipped.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
	}, opts...)...)
	objID := srv.SoleObjectID(t)
	held := make(map[int32]int32, len(ids))
	for _, id := range ids {
		held[id] = srv.GiveItem(t, objID, id, 1)
	}
	return srv, objID, held
}

// shippedHelpPage reads a shipped data/html/help page.
func shippedHelpPage(t *testing.T, name string) string {
	t.Helper()
	root := datapack.Require(t)
	content, err := os.ReadFile(filepath.Join(root, "data", "html", "help", name))
	if err != nil {
		t.Fatalf("read shipped help page %s: %v", name, err)
	}
	return string(content)
}

// readNpcHtml reads an NpcHtmlMessage and returns its object id, page and
// item id.
func readNpcHtml(t *testing.T, frame []byte) (int32, string, int32) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeNpcHtmlMessage, "NpcHtmlMessage")
	r := wire.NewReader(frame[1:])
	return r.ReadInt32(), r.ReadString(), r.ReadInt32()
}

// assertQuiet fails on any frame c receives before the server goes quiet.
func assertQuiet(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("%s: unexpected opcode %#x", what, frame[0])
	}
}

// assertStillHeld fails unless the character still holds one of objectID.
func assertStillHeld(t *testing.T, srv *gameservertest.Server, ownerID, objectID int32) {
	t.Helper()
	if inst := mustFindItem(t, srv, ownerID, objectID); inst.Count != 1 || inst.Location != item.LocationInventory {
		t.Fatalf("used item %d = count %d at %v, want one still in the inventory", objectID, inst.Count, inst.Location)
	}
}

func encodeItemsBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

// TestUseBookOpensItsHelpPage pins Books.useItem: the book's
// data/html/help/<id>.htm page as NpcHtmlMessage(objectId 0, itemId = the
// book), then ActionFailed, and the book is kept. A book without a page
// shows the missing-page notice naming the path. The page goes through the
// validated path (NpcHtmlMessage.runImpl): its links replace the player's
// valid bypasses, so a Quest link on the page is answered afterwards.
func TestUseBookOpensItsHelpPage(t *testing.T) {
	t.Parallel()
	tutorial := shippedHelpPage(t, "5588.htm")
	// The probe link is sent twice in quick succession: no bypass reuse gate.
	srv, objID, held := bootWindowItems(t, gameservertest.WithReuseDelays(3*time.Second, 0), gameservertest.WithHTMLPages(map[string]string{
		"help/5588.htm": tutorial,
		// The test stands in a page whose link is one the server validates,
		// so recording the book's links is observable.
		"help/6317.htm": `<html><body><a action="bypass -h Quest Q999_Probe start">probe</a></body></html>`,
	}))
	c := srv.Client
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(held[tutorialGuideID], false))
	// The page cache's line-ending normalization is pinned with the cache;
	// only the page's text matters here.
	if objectID, page, itemID := readNpcHtml(t, c.Read()); objectID != 0 || strings.TrimSpace(page) != strings.TrimSpace(tutorial) || itemID != tutorialGuideID {
		t.Fatalf("Tutorial Guide page = object %d item %d %q, want object 0 item %d and the shipped 5588.htm", objectID, itemID, page, tutorialGuideID)
	}
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "after the book page")
	assertQuiet(t, c, "Tutorial Guide")
	assertStillHeld(t, srv, objID, held[tutorialGuideID])

	// The probe link is not on the last page yet: it is dropped silently.
	c.Send(encodeItemsBypass("Quest Q999_Probe start"))
	assertQuiet(t, c, "probe link before the book")
	c.Send(encodeUseItem(held[mixingManualID], false))
	if _, _, itemID := readNpcHtml(t, c.Read()); itemID != mixingManualID {
		t.Fatalf("Mixing Manual page item id = %d, want %d", itemID, mixingManualID)
	}
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "after the probe page")
	c.Send(encodeItemsBypass("Quest Q999_Probe start"))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "probe link from the book page")
	assertQuiet(t, c, "probe link")
}

// TestUseBookWithoutPageShowsMissingNotice pins HtmCache.getHtmForce for a
// book: no data/html/help/<id>.htm renders the reference notice.
func TestUseBookWithoutPageShowsMissingNotice(t *testing.T) {
	t.Parallel()
	srv, objID, held := bootWindowItems(t)
	c := srv.Client
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(held[tutorialGuideID], false))
	const want = "<html><body>My html is missing:<br>data/html/help/5588.htm</body></html>"
	if objectID, page, itemID := readNpcHtml(t, c.Read()); objectID != 0 || page != want || itemID != tutorialGuideID {
		t.Fatalf("missing book page = object %d %q item %d, want object 0 %q item %d", objectID, page, itemID, want, tutorialGuideID)
	}
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "after the missing page")
	assertStillHeld(t, srv, objID, held[tutorialGuideID])
}

// TestUseWindowItemsOpenTheirWindow pins Calculators, Maps and SpecialXMas:
// each answers with its one window packet and nothing else (no
// ActionFailed), and keeps the item. A map carries the live Seven Signs
// period. The client's own mini-map request (RequestShowMiniMap) opens the
// World Map the same way.
func TestUseWindowItemsOpenTheirWindow(t *testing.T) {
	t.Parallel()
	srv, objID, held := bootWindowItems(t)
	c := srv.Client
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	for _, tt := range []struct {
		name   string
		itemID int32
		want   []byte
	}{
		{"Calculator", calculatorID, []byte{serverpackets.OpcodeShowCalculator, 0x29, 0x11, 0x00, 0x00}},
		{"World Map", worldMapID, []byte{serverpackets.OpcodeShowMiniMap, 0x81, 0x06, 0x00, 0x00, competitionPeriod, 0x00, 0x00, 0x00}},
		{"Map: Elmore", elmoreMapID, []byte{serverpackets.OpcodeShowMiniMap, 0x47, 0x07, 0x00, 0x00, competitionPeriod, 0x00, 0x00, 0x00}},
		{"Token of Love", tokenOfLoveID, []byte{serverpackets.OpcodeShowXMasSeal, 0xb3, 0x15, 0x00, 0x00}},
	} {
		c.Send(encodeUseItem(held[tt.itemID], false))
		if got := c.Read(); string(got) != string(tt.want) {
			t.Fatalf("%s window = %x, want %x", tt.name, got, tt.want)
		}
		assertQuiet(t, c, tt.name)
		assertStillHeld(t, srv, objID, held[tt.itemID])
	}

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestShowMiniMap))
	want := []byte{serverpackets.OpcodeShowMiniMap, 0x81, 0x06, 0x00, 0x00, competitionPeriod, 0x00, 0x00, 0x00}
	if got := c.Read(); string(got) != string(want) {
		t.Fatalf("RequestShowMiniMap answer = %x, want %x", got, want)
	}
	assertQuiet(t, c, "RequestShowMiniMap")
}

// assertDiceThrow reads one throw as a viewer receives it: Dice for the
// roller's die landing 30 units ahead of it, then S1_ROLLED_S2 naming the
// roller and the same number. It returns the number.
func assertDiceThrow(t *testing.T, c *testsupport.ScriptedClient, who string, rollerID int32) int32 {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeDice, who+" Dice")
	r := wire.NewReader(frame[1:])
	gotRoller, gotDie, number := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	x, y, z := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	// The spawn heading is 0, so the die lands 30 units along +X.
	if gotRoller != rollerID || gotDie != heartDiceID || number < 1 || number > 6 || x != spawnX+30 || y != spawnY || z != spawnZ {
		t.Fatalf("%s Dice = roller %d die %d number %d at (%d,%d,%d), want roller %d die %d number 1-6 at (%d,%d,%d)",
			who, gotRoller, gotDie, number, x, y, z, rollerID, heartDiceID, spawnX+30, spawnY, spawnZ)
	}
	msg := c.Read()
	if id := systemMessageID(t, msg); id != serverpackets.SystemMessageS1RolledS2 {
		t.Fatalf("%s message = %d, want S1_ROLLED_S2", who, id)
	}
	r = wire.NewReader(msg[5:])
	if n, typ, name, numTyp, got := r.ReadInt32(), r.ReadInt32(), r.ReadString(), r.ReadInt32(), r.ReadInt32(); n != 2 || typ != serverpackets.SystemMessageParamText || name != "Newbie" || numTyp != serverpackets.SystemMessageParamNumber || got != number {
		t.Fatalf("%s S1_ROLLED_S2 params = %d [%d %q %d %d], want 2 [text \"Newbie\" number %d]", who, n, typ, name, numTyp, got, number)
	}
	return number
}

// TestRollingDiceBroadcastsThrowAndHonorsReuseDelay pins RollingDices: a
// throw reaches the roller and a player that knows it as Dice then
// S1_ROLLED_S2 with the same number, and the die is kept. A second throw
// inside RollDiceTime tells the roller alone to wait (835) and shows nothing
// to the observer; once the delay has passed the die rolls again.
func TestRollingDiceBroadcastsThrowAndHonorsReuseDelay(t *testing.T) {
	t.Parallel()
	const delay = 2 * time.Second
	srv, objID, held := bootWindowItems(t, gameservertest.WithRollDiceDelay(delay))
	c := srv.Client
	startInWorld(t, c)
	srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	observer := srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, c)

	c.Send(encodeUseItem(held[heartDiceID], false))
	thrownAt := time.Now()
	rolled := assertDiceThrow(t, c, "roller", objID)
	// The server armed the gate no later than the roller saw the throw.
	seenAt := time.Now()
	if seen := assertDiceThrow(t, observer, "observer", objID); seen != rolled {
		t.Fatalf("observer saw %d, roller %d", seen, rolled)
	}
	assertStillHeld(t, srv, objID, held[heartDiceID])

	c.Send(encodeUseItem(held[heartDiceID], false))
	if time.Since(thrownAt) >= delay {
		t.Skip("second throw left too late to land inside the reuse delay")
	}
	if id := systemMessageID(t, c.Read()); id != serverpackets.SystemMessageCannotThrowDiceAtThisTime {
		t.Fatalf("refused throw message = %d, want 835", id)
	}
	assertQuiet(t, c, "refused throw")
	assertQuiet(t, observer, "observer of a refused throw")

	time.Sleep(time.Until(seenAt.Add(delay + 50*time.Millisecond)))
	c.Send(encodeUseItem(held[heartDiceID], false))
	assertDiceThrow(t, c, "roller after the delay", objID)
	assertDiceThrow(t, observer, "observer after the delay", objID)
	assertStillHeld(t, srv, objID, held[heartDiceID])
}

// TestRollingDiceUngatedWhenDelayIsZero pins RollDiceTime = 0: the gate
// never refuses, so back-to-back throws both roll.
func TestRollingDiceUngatedWhenDelayIsZero(t *testing.T) {
	t.Parallel()
	srv, objID, held := bootWindowItems(t, gameservertest.WithRollDiceDelay(0))
	c := srv.Client
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	for range 2 {
		c.Send(encodeUseItem(held[heartDiceID], false))
		assertDiceThrow(t, c, "roller", objID)
	}
	assertQuiet(t, c, "ungated throws")
}
