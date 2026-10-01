package npcs

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// dialogPage is a merchant's first page: a chat page link, an unported
	// shop command and a link page.
	dialogPage = `<html><body>Trader:<br>` +
		`<a action="bypass -h npc_%objectId%_Chat 1">More</a>` +
		`<a action="bypass -h npc_%objectId%_Buy 1">Buy</a>` +
		`<a action="bypass npc_%objectId%_Link merchant/info.htm">Info</a>` +
		`</body></html>`
	// dialogPage1 is its chat page 1, whose link takes an edit-box value.
	dialogPage1 = `<html><body>Page one %objectId%<br><edit var="n">` +
		`<a action="bypass -h npc_%objectId%_Chat $n">Go</a></body></html>`
	dialogPage2 = `<html><body>Page two</body></html>`
	infoPage    = `<html><body>Info %objectId%</body></html>`
	// anyNpcPage admits every npc_ command: its one link takes the whole
	// command after npc_ from an edit box.
	anyNpcPage = `<html><body><edit var="c"><a action="bypass -h npc_$c">Go</a>` +
		`<a action="bypass -h Quest $c">Quest</a></body></html>`
	helpPage = `<html><body>Help</body></html>`
)

// noBypassReuse lifts the bypass reuse delay, so a scenario can send its
// commands back to back.
var noBypassReuse = gameservertest.WithReuseDelays(3*time.Second, 0)

func dialogPages() map[string]string {
	return map[string]string{
		"merchant/30001.htm":   dialogPage,
		"merchant/30001-1.htm": dialogPage1,
		"merchant/30001-2.htm": dialogPage2,
		"merchant/info.htm":    infoPage,
		"test/any.htm":         anyNpcPage,
		"help/tutorial.htm":    helpPage,
		"npcdefault.htm":       "<html><body>Nothing to say %objectId%</body></html>",
	}
}

func encodeBypass(command string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
	w.WriteString(command)
	return w.Bytes()
}

func encodeLinkHTML(link string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestLinkHtml)
	w.WriteString(link)
	return w.Bytes()
}

// bypass sends command and returns every frame it is answered with.
func (w *folkWorld) bypass(t *testing.T, command string) [][]byte {
	t.Helper()
	w.c.Send(encodeBypass(command))
	return drainFrames(t, w.c)
}

// openAnyNpcPage opens the page that admits every npc_ and Quest command.
func (w *folkWorld) openAnyNpcPage(t *testing.T) {
	t.Helper()
	w.c.Send(encodeLinkHTML("test/any.htm"))
	if frames := drainFrames(t, w.c); string(opcodes(frames)) != string([]byte{serverpackets.OpcodeNpcHtmlMessage}) {
		t.Fatalf("link page = %x, want NpcHtmlMessage", opcodes(frames))
	}
}

// talkTo selects f, talks to it and drains its first page.
func (w *folkWorld) talkTo(t *testing.T, f *npc.Folk) {
	t.Helper()
	w.selectFolk(t, f)
	if _, ok := firstOpcode(w.talk(t, f, false), serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatal("talk opened no page")
	}
}

func npcCommand(f *npc.Folk, command string) string {
	return "npc_" + strconv.Itoa(int(f.ObjectID())) + "_" + command
}

// assertAnswer checks frames are exactly want, in order, and that the one
// NpcHtmlMessage among them, if any, is html from f.
func assertAnswer(t *testing.T, frames [][]byte, want []byte, f *npc.Folk, html string) {
	t.Helper()
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("answer = %x, want %x", got, want)
	}
	frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		return
	}
	if objectID, got, itemID := htmlMessage(t, frame); objectID != f.ObjectID() || itemID != 0 || got != html {
		t.Fatalf("NpcHtmlMessage = object %d item %d %q, want %d/0 %q", objectID, itemID, got, f.ObjectID(), html)
	}
}

var (
	chatWindowAnswer = []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}
	releaseOnly      = []byte{serverpackets.OpcodeActionFailed}
)

// TestBypassChatAndLinkFollowTheLastPage pins the dialog walk: a Chat link
// opens that chat page then releases the client twice (the chat window's
// ActionFailed, then the dispatcher's); a Link opens its page and releases
// once. Each validated page replaces the links the player may send back,
// so a link of an earlier page is dropped silently, and a link taking an
// edit-box value admits any command starting with its text before '$'.
func TestBypassChatAndLinkFollowTheLastPage(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.talkTo(t, f)

	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), chatWindowAnswer, f, wantChatPage(dialogPage1, f))
	// Page one holds only "npc_<id>_Chat $n": the first page's links are gone.
	assertAnswer(t, w.bypass(t, npcCommand(f, "Link merchant/info.htm")), nil, f, "")
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 2")), chatWindowAnswer, f, wantChatPage(dialogPage2, f))
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), nil, f, "")

	w.talkTo(t, f)
	assertAnswer(t, w.bypass(t, npcCommand(f, "Link merchant/info.htm")), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}, f, wantChatPage(infoPage, f))
}

// TestBypassPlayerHelpKeepsTheDialogLinks pins that a help page is not
// validated: it leaves the NPC page's links usable.
func TestBypassPlayerHelpKeepsTheDialogLinks(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	w.talkTo(t, f)

	frames := w.bypass(t, "player_help tutorial.htm")
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeNpcHtmlMessage}) {
		t.Fatalf("help = %x, want NpcHtmlMessage", got)
	}
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), chatWindowAnswer, f, wantChatPage(dialogPage1, f))
}

// TestBypassNpcRejections pins the dispatcher's gates on whitelisted npc_
// commands: an object id that does not parse is dropped silently; an
// unknown object, a non-civilian NPC, an NPC out of interaction distance,
// a command naming no dialog command, an unported command and a Link
// climbing out of the page tree are each answered ActionFailed alone; a
// bare Link aborts with nothing sent.
func TestBypassNpcRejections(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	near := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	far := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 1000)
	hostile := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X + 30, Y: w.at.Y, Z: w.at.Z})
	drainUntilQuiet(t, w.c)
	w.openAnyNpcPage(t)

	for _, tc := range []struct {
		command string
		want    []byte
	}{
		{"npc_x1_Chat 1", nil},
		{"npc_999999_Chat 1", releaseOnly},
		{"npc_" + strconv.Itoa(int(hostile.ObjectID())) + "_Chat 1", releaseOnly},
		{npcCommand(far, "Chat 1"), releaseOnly},
		{"npc_" + strconv.Itoa(int(near.ObjectID())), releaseOnly},
		{npcCommand(near, "Buy 1"), releaseOnly},
		{npcCommand(near, "TerritoryStatus"), releaseOnly},
		{npcCommand(near, "Quest"), releaseOnly},
		{npcCommand(near, "Link ../../config/server.properties"), releaseOnly},
		{npcCommand(near, "Link"), nil},
	} {
		assertAnswer(t, w.bypass(t, tc.command), tc.want, near, "")
	}
}

// TestBypassChatPagePaths pins chat page n of the types without a page
// folder: data/html/default/<id>-<n>.htm, else npcdefault.htm, and a Chat
// whose page number does not parse opens page 0.
func TestBypassChatPagePaths(t *testing.T) {
	t.Parallel()
	pages := dialogPages()
	pages["default/30100.htm"] = "<html><body>zero</body></html>"
	pages["default/30100-3.htm"] = "<html><body>three %objectId%</body></html>"
	w := bootFolkWorld(t, pages, noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("Folk", 30100), 50)

	for _, tc := range []struct{ command, page string }{
		{"Chat 3", pages["default/30100-3.htm"]},
		{"Chat 4", pages["npcdefault.htm"]},
		{"Chat x", pages["default/30100.htm"]},
		{"Chat", pages["default/30100.htm"]},
	} {
		w.openAnyNpcPage(t)
		assertAnswer(t, w.bypass(t, npcCommand(f, tc.command)), chatWindowAnswer, f, wantChatPage(tc.page, f))
	}
}

// TestBypassDungeonGatekeeperReleasesFirst pins the dungeon gatekeeper,
// which releases the client ahead of every command.
func TestBypassDungeonGatekeeperReleasesFirst(t *testing.T) {
	t.Parallel()
	pages := dialogPages()
	pages["gatekeeper/31095-1.htm"] = "<html><body>dungeon</body></html>"
	w := bootFolkWorld(t, pages, noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", 31095), 50)

	w.openAnyNpcPage(t)
	want := append([]byte{serverpackets.OpcodeActionFailed}, chatWindowAnswer...)
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), want, f, wantChatPage(pages["gatekeeper/31095-1.htm"], f))
	w.openAnyNpcPage(t)
	assertAnswer(t, w.bypass(t, npcCommand(f, "necro")), []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}, f, "")
}

// TestBypassKarmaGateCoversEveryCommand pins the shop karma gate on
// dialog commands: with KarmaPlayerCanShop off, a player carrying karma
// gets the refusal page, as is, for every command. A fisherman checks its
// own refusal page, then, past its own commands, the merchant one: with no
// fisherman refusal page, its FishSkillList still opens the fishing list
// (here empty: nothing left to learn, then AcquireSkillDone).
func TestBypassKarmaGateCoversEveryCommand(t *testing.T) {
	t.Parallel()
	const pkPage = "<html><body>No trade with killers %objectId%</body></html>"
	pages := dialogPages()
	pages["merchant/30001-pk.htm"] = pkPage
	pages["merchant/31562-pk.htm"] = pkPage
	pages["fisherman/31562-1.htm"] = "<html><body>fish</body></html>"
	w := bootFolkWorldAs(t, withKarmaCharacter(100), pages, withShop(false), noBypassReuse)
	merchant := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	fisherman := w.spawnFolk(t, folkTemplate("Fisherman", 31562), 50)

	for _, tc := range []struct {
		f       *npc.Folk
		command string
		want    []byte
	}{
		{merchant, "Chat 1", chatWindowAnswer},
		{merchant, "Buy 1", chatWindowAnswer},
		{fisherman, "Chat 1", chatWindowAnswer},
		{fisherman, "FishSkillList", []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeAcquireSkillDone, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed}},
	} {
		w.openAnyNpcPage(t)
		html := ""
		if tc.want[0] == serverpackets.OpcodeNpcHtmlMessage {
			html = pkPage + "\n"
		}
		assertAnswer(t, w.bypass(t, npcCommand(tc.f, tc.command)), tc.want, tc.f, html)
	}
}

// TestBypassWarehouseCancelsEnchantSelection pins the warehouse keeper,
// which drops the player's enchant scroll selection ahead of every command.
func TestBypassWarehouseCancelsEnchantSelection(t *testing.T) {
	t.Parallel()
	pages := dialogPages()
	pages["warehouse/30005-1.htm"] = "<html><body>vault</body></html>"
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	scroll := srv.GiveItem(t, w.player, 955, 1)
	startInWorld(t, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at.X, w.at.Y, w.at.Z = x, y, z
	f := w.spawnFolk(t, folkTemplate("WarehouseKeeper", 30005), 50)

	w.c.Send(encodeUseItem(scroll))
	drainUntilQuiet(t, w.c)
	w.openAnyNpcPage(t)
	frames := w.bypass(t, npcCommand(f, "Chat 1"))
	want := append([]byte{serverpackets.OpcodeEnchantResult, serverpackets.OpcodeSystemMessage}, chatWindowAnswer...)
	assertAnswer(t, frames, want, f, wantChatPage(pages["warehouse/30005-1.htm"], f))
}

// TestBypassRefusedWhileTradeRequestPending pins the interact gate on
// dialog commands and talks: a player holding an unexpired trade request
// cannot interact, so a command is answered ActionFailed alone and a talk
// opens no page.
func TestBypassRefusedWhileTradeRequestPending(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	otherID := w.srv.SeedCharacterFor(t, "player2", "Other", playerLevel, 0).ID
	other := w.srv.DialClient(t, "player2", 1)
	startInWorld(t, other)
	drainUntilQuiet(t, w.c)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	drainUntilQuiet(t, other)
	w.talkTo(t, f)

	w.c.Send(encodeTradeRequest(otherID))
	drainUntilQuiet(t, w.c)
	drainUntilQuiet(t, other)
	assertAnswer(t, w.bypass(t, npcCommand(f, "Chat 1")), releaseOnly, f, "")
	if frames := w.talk(t, f, false); len(frames) == 0 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("talk with a trade request pending = %x, want ActionFailed first", opcodes(frames))
	} else if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatalf("talk with a trade request pending opened a page: %x", opcodes(frames))
	}
}

// TestBypassCommandFamilies pins the families whose systems are not in
// place: each is answered ActionFailed. A Quest command must be on the
// last page, like an npc_ one, and is dropped silently when it is not. An
// admin_ command from a player without the access rights is refused with a
// message only.
func TestBypassCommandFamilies(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)

	assertAnswer(t, w.bypass(t, "Quest Q001_LettersOfLove 30048-03.htm"), nil, nil, "")
	w.openAnyNpcPage(t)
	assertAnswer(t, w.bypass(t, "Quest Q001_LettersOfLove 30048-03.htm"), releaseOnly, nil, "")
	assertAnswer(t, w.bypass(t, "admin_admin"), []byte{serverpackets.OpcodeSystemMessage}, nil, "")
	for _, command := range []string{"bbs_default", "_bbshome", "_friendlist_0_", "_maillist_0_1_0_", "_block", "manor_menu_select?ask=1", "_match?class=88&page=1", "_diary?class=88&page=1", "arenachange 1"} {
		assertAnswer(t, w.bypass(t, command), releaseOnly, nil, "")
	}
	assertAnswer(t, w.bypass(t, "unknown_command"), nil, nil, "")
}

func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

// TestBypassAugmentOpensVariationWindows pins a blacksmith's Augment
// command: "Augment 1" prompts for the item to augment and opens the
// augmentation window, "Augment 2" prompts for the item to restore and opens
// the removal window, each then released by the dispatcher; any other
// choice is only released, and a choice too short or not a digit aborts
// with nothing sent.
func TestBypassAugmentOpensVariationWindows(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	smith := w.spawnFolk(t, folkTemplate("Trainer", 30300), 50)

	for _, tc := range []struct {
		command string
		want    []byte
		message int32
		window  uint16
	}{
		{"Augment 1", []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeExtended, serverpackets.OpcodeActionFailed}, serverpackets.SystemMessageSelectItemToAugment, serverpackets.OpcodeExShowVariationMakeWindow},
		{"Augment 2", []byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeExtended, serverpackets.OpcodeActionFailed}, serverpackets.SystemMessageSelectItemToRemoveAugmentation, serverpackets.OpcodeExShowVariationCancelWindow},
		{"Augment 3", releaseOnly, 0, 0},
		{"Augment", nil, 0, 0},
		{"Augment x", nil, 0, 0},
	} {
		w.openAnyNpcPage(t)
		frames := w.bypass(t, npcCommand(smith, tc.command))
		assertAnswer(t, frames, tc.want, smith, "")
		if tc.message == 0 {
			continue
		}
		if got := wire.NewReader(frames[0][1:]).ReadInt32(); got != tc.message {
			t.Fatalf("%s: system message = %d, want %d", tc.command, got, tc.message)
		}
		r := wire.NewReader(frames[1][1:])
		if sub := r.ReadUint16(); sub != tc.window || r.Remaining() != 0 {
			t.Fatalf("%s: window = %#x with %d more bytes, want %#x alone", tc.command, sub, r.Remaining(), tc.window)
		}
	}
}
