package npcs

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// schemeBufferID is the shipped scheme buffer, whose pages live in
// data/html/mods/buffer.
const schemeBufferID = 50002

// schemeBufferSkills loads the two skill files holding the fixture buffs:
// 1035 Mental Shield (4 levels), 1036 Magic Barrier, 1040 Shield, 1045
// Blessed Body, 1048 Blessed Soul, 1059 Greater Empower, 1068 Might (3
// levels) and 271 Dance of the Warrior.
func schemeBufferSkills(t *testing.T) *modelskill.Table {
	t.Helper()
	dir := t.TempDir()
	for _, file := range []string{"1000-1099.xml", "0200-0299.xml"} {
		data, err := os.ReadFile(datapack.Path(t, "data", "xml", "skills", file))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	defs, err := xmldata.LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	return defs
}

// schemeBufferBuffs is the fixture offer: seven Buffs, two pages of the
// editor, the first and last priced, then one Dance.
func schemeBufferBuffs(t *testing.T) *modelskill.BufferTable {
	t.Helper()
	buff := func(id modelskill.ID, level, price int, category string) modelskill.BufferSkill {
		return modelskill.BufferSkill{Skill: modelskill.Ref{ID: id, Level: level}, Price: price, Category: category, Description: "d" + strconv.Itoa(int(id))}
	}
	table, err := modelskill.NewBufferTable([]modelskill.BufferSkill{
		buff(1035, 4, 1000, "Buffs"),
		buff(1036, 2, 0, "Buffs"),
		buff(1040, 3, 0, "Buffs"),
		buff(1045, 6, 0, "Buffs"),
		buff(1048, 6, 0, "Buffs"),
		buff(1059, 3, 0, "Buffs"),
		buff(1068, 3, 2500, "Buffs"),
		buff(271, 1, 0, "Dances"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// schemeBufferWorld enters the world holding adena next to the shipped
// scheme buffer's pages, wired to buffer.
func schemeBufferWorld(t *testing.T, buffer *schemebuffer.Manager, adena int32) (*folkWorld, *npc.Folk) {
	t.Helper()
	pages := dialogPages()
	for _, name := range []string{"50002.htm", "50002-1.htm", "50002-2.htm", "50002-3.htm"} {
		data, err := os.ReadFile(datapack.Path(t, "data", "html", "mods", "buffer", name))
		if err != nil {
			t.Fatal(err)
		}
		pages["mods/buffer/"+name] = string(data)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithSchemeBuffer(buffer),
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	f := w.spawnFolk(t, folkTemplate("SchemeBuffer", schemeBufferID), 60)
	return w, f
}

// datapackPage is a shipped scheme buffer page, as the page cache holds
// it, with its placeholders set to the given values, in the order the
// dialog fills them, then %objectId% set to f's id.
func datapackPage(t *testing.T, f *npc.Folk, name string, values ...string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, "data", "html", "mods", "buffer", name))
	if err != nil {
		t.Fatal(err)
	}
	// The page cache reads a page line by line, ending each line with \n.
	page := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	for i := 0; i < len(values); i += 2 {
		page = strings.ReplaceAll(page, values[i], values[i+1])
	}
	return strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(f.ObjectID())))
}

// assertSchemePage checks frames are the scheme buffer's page html, opened
// with no NPC object id, then the dispatcher's release.
func assertSchemePage(t *testing.T, what string, frames [][]byte, html string) {
	t.Helper()
	want := []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("%s: answer = %x, want %x", what, got, want)
	}
	if objectID, got, itemID := htmlMessage(t, frames[0]); objectID != 0 || itemID != 0 || got != html {
		t.Fatalf("%s: NpcHtmlMessage = object %d item %d\n%q\nwant object 0 item 0\n%q", what, objectID, itemID, got, html)
	}
}

// schemePageOf is the one page among frames.
func schemePageOf(t *testing.T, frames [][]byte) string {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("answer %x opens no page", opcodes(frames))
	}
	_, html, _ := htmlMessage(t, frame)
	return html
}

func noticeFrame(text string) []byte { return sysMsg(serverpackets.SystemMessageS1, textParam(text)) }

// activeBuff reads the level of skill id's effect on the player.
func (w *folkWorld) activeBuff(t *testing.T, id int) (level int, ok bool) {
	t.Helper()
	w.onPlayer(t, func(pc *player.Character) { level, ok = pc.EffectList().ActiveBySkillID(id) })
	return level, ok
}

// TestSchemeBufferSchemeLifecycle walks SchemeBuffer.onBypassFeedback from
// the shipped pages: the schemes page (50002-1.htm) lists none, a scheme is
// created and named again in another case, the editor (50002-2.htm) adds a
// buff from each of its two pages, the schemes page then shows the fee,
// "Use on Me" first refuses a talker short of it, then lands the buffs at
// the buffer's level once paid, "Use on Pet" without a pet is told so, and
// the scheme is deleted. Every page opens with no NPC object id, and every
// command ends with the dispatcher's ActionFailed. The schemes reach
// buffer_schemes only when saved.
func TestSchemeBufferSchemeLifecycle(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t))
	w, f := schemeBufferWorld(t, buffer, 3000)
	oid := strconv.Itoa(int(f.ObjectID()))
	w.talkTo(t, f)

	none := `<font color="LEVEL">You haven't defined any scheme.</font>`
	assertSchemePage(t, "support", w.bypass(t, npcCommand(f, "support")),
		datapackPage(t, f, "50002-1.htm", "%schemes%", none, "%max_schemes%", "4"))

	created := `<font color="LEVEL">Fight [0 / 20]</font><br1>` +
		`<a action="bypass npc_` + oid + `_givebuffs Fight 0">Use on Me</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_givebuffs Fight 0 pet">Use on Pet</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_editschemes Buffs Fight 1">Edit</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_deletescheme Fight">Delete</a><br>`
	assertSchemePage(t, "createscheme", w.bypass(t, npcCommand(f, "createscheme Fight")),
		datapackPage(t, f, "50002-1.htm", "%schemes%", created, "%max_schemes%", "4"))
	assertFrames(t, "createscheme again", w.bypass(t, npcCommand(f, "createscheme fIGHT")),
		noticeFrame("The scheme name already exists."), []byte{serverpackets.OpcodeActionFailed})
	assertFrames(t, "createscheme too long", w.bypass(t, npcCommand(f, "createscheme abcdefghijklmno")),
		noticeFrame("Scheme's name must contain up to 14 chars. Spaces are trimmed."), []byte{serverpackets.OpcodeActionFailed})

	types := `<table><tr><td width=65>Buffs</td><td width=65><a action="bypass npc_` + oid + `_editschemes Dances Fight 1">Dances</a></td></tr></table>`
	row0 := `<table width="280" bgcolor="000000"><tr><td height=40 width=40><img src="icon.skill1035" width=32 height=32></td>` +
		`<td width=190>Mental Shield<br1><font color="B09878">d1035</font></td><td><button action="bypass npc_` + oid +
		`_skillselect Buffs Fight 1035 1" width=32 height=32 back="L2UI_CH3.mapbutton_zoomin2" fore="L2UI_CH3.mapbutton_zoomin1"></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
	row1 := `<table width="280"><tr><td height=40 width=40><img src="icon.skill1036" width=32 height=32></td>` +
		`<td width=190>Magic Barrier<br1><font color="B09878">d1036</font></td><td><button action="bypass npc_` + oid +
		`_skillselect Buffs Fight 1036 1" width=32 height=32 back="L2UI_CH3.mapbutton_zoomin2" fore="L2UI_CH3.mapbutton_zoomin1"></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
	footer1 := `<br><img src="L2UI.SquareGray" width=280 height=1><table width="100%" bgcolor=000000><tr>` +
		`<td align=left width=70>Previous</td><td align=center width=100>Page 1</td>` +
		`<td align=right width=70><a action="bypass npc_` + oid + `_editschemes Buffs Fight 2">Next</a></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
	edit := schemePageOf(t, w.bypass(t, npcCommand(f, "editschemes Buffs Fight 1")))
	wantHead := datapackPage(t, f, "50002-2.htm", "%schemename%", "Fight", "%count%", "0 / 20", "%typesframe%", types, "%skilllistframe%", "@@")
	head, tail, _ := strings.Cut(wantHead, "@@")
	if !strings.HasPrefix(edit, head+row0+row1) || !strings.HasSuffix(edit, footer1+tail) {
		t.Fatalf("editor page 1 = %s\nwant %s%s%s...%s%s", edit, head, row0, row1, footer1, tail)
	}
	if n := strings.Count(edit, `<table width="280"`); n != 6 {
		t.Fatalf("editor page 1 lists %d buffs, want 6", n)
	}

	selected := schemePageOf(t, w.bypass(t, npcCommand(f, "skillselect Buffs Fight 1035 1")))
	if !strings.Contains(selected, "Fight</font> scheme holds 1 / 20 buffs.") ||
		!strings.Contains(selected, `_skillunselect Buffs Fight 1035 1" width=32 height=32 back="L2UI_CH3.mapbutton_zoomout2" fore="L2UI_CH3.mapbutton_zoomout1">`) {
		t.Fatalf("editor after select = %s", selected)
	}

	page2 := `<table width="280" bgcolor="000000"><tr><td height=40 width=40><img src="icon.skill1068" width=32 height=32></td>` +
		`<td width=190>Might<br1><font color="B09878">d1068</font></td><td><button action="bypass npc_` + oid +
		`_skillselect Buffs Fight 1068 2" width=32 height=32 back="L2UI_CH3.mapbutton_zoomin2" fore="L2UI_CH3.mapbutton_zoomin1"></td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>` +
		`<img height=41><img height=41><img height=41><img height=41><img height=41>` +
		`<br><img src="L2UI.SquareGray" width=280 height=1><table width="100%" bgcolor=000000><tr>` +
		`<td align=left width=70><a action="bypass npc_` + oid + `_editschemes Buffs Fight 1">Previous</a></td>` +
		`<td align=center width=100>Page 2</td><td align=right width=70>Next</td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
	assertSchemePage(t, "editschemes page 2", w.bypass(t, npcCommand(f, "editschemes Buffs Fight 2")),
		datapackPage(t, f, "50002-2.htm", "%schemename%", "Fight", "%count%", "1 / 20", "%typesframe%", types, "%skilllistframe%", page2))
	w.bypass(t, npcCommand(f, "skillselect Buffs Fight 1068 2"))

	priced := `<font color="LEVEL">Fight [2 / 20] - cost: 3,500</font><br1>` +
		`<a action="bypass npc_` + oid + `_givebuffs Fight 3500">Use on Me</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_givebuffs Fight 3500 pet">Use on Pet</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_editschemes Buffs Fight 1">Edit</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_deletescheme Fight">Delete</a><br>`
	assertSchemePage(t, "support with a priced scheme", w.bypass(t, npcCommand(f, "support")),
		datapackPage(t, f, "50002-1.htm", "%schemes%", priced, "%max_schemes%", "4"))

	assertFrames(t, "givebuffs short of the fee", w.bypass(t, npcCommand(f, "givebuffs Fight 3500")),
		sysMsg(serverpackets.SystemMessageYouNotEnoughAdena), []byte{serverpackets.OpcodeActionFailed})
	if _, ok := w.activeBuff(t, 1035); ok {
		t.Fatal("unpaid scheme landed Mental Shield")
	}
	assertFrames(t, "givebuffs on a missing pet", w.bypass(t, npcCommand(f, "givebuffs Fight 3500 pet")),
		noticeFrame("You don't have a pet."), []byte{serverpackets.OpcodeActionFailed})

	// Trading Might for Magic Barrier drops the fee to Mental Shield's
	// 1,000; each command follows a link of the page before it.
	for _, command := range []string{
		"editschemes Buffs Fight 1", "editschemes Buffs Fight 2", "skillunselect Buffs Fight 1068 2",
		"editschemes Buffs Fight 1", "skillselect Buffs Fight 1036 1", "support",
	} {
		schemePageOf(t, w.bypass(t, npcCommand(f, command)))
	}
	frames := w.bypass(t, npcCommand(f, "givebuffs Fight 1000"))
	if len(frames) < 2 || string(frames[0]) != string(sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1000))) ||
		frames[len(frames)-1][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("givebuffs = %x, want the fee taken first and the release last", opcodes(frames))
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatal("givebuffs opened a page")
	}
	if got := w.savedCount(t, item.AdenaID); got != 2000 {
		t.Fatalf("adena saved = %d, want 2000", got)
	}
	for id, want := range map[int]int{1035: 4, 1036: 2} {
		if level, ok := w.activeBuff(t, id); !ok || level != want {
			t.Fatalf("buff %d = level %d (%v), want level %d", id, level, ok, want)
		}
	}

	// Nothing is stored until the shutdown save.
	store := gamesql.NewBufferSchemeStore(w.srv.DB)
	ctx := context.Background()
	if rows, err := store.Load(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("stored schemes before the save = %v, %v; want none", rows, err)
	}
	if err := store.Save(ctx, buffer.Rows()); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []schemebuffer.Row{{OwnerID: w.player, Name: "Fight", Skills: "1035,1036"}}; len(rows) != 1 || rows[0] != want[0] {
		t.Fatalf("stored schemes = %+v, want %+v", rows, want)
	}

	// A scheme is named in any case.
	w.openAnyNpcPage(t)
	assertSchemePage(t, "deletescheme", w.bypass(t, npcCommand(f, "deletescheme FIGHT")),
		datapackPage(t, f, "50002-1.htm", "%schemes%", none, "%max_schemes%", "4"))
}

// TestSchemeBufferHealAndCleanup pins the menu's heal and cleanup: heal
// fills the talker's CP, HP and MP and shows it (StatusUpdate), cleanup
// ends the talker's buffs and refreshes its info (UserInfo); each then
// opens the first page, 50002.htm, with no NPC object id.
func TestSchemeBufferHealAndCleanup(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t))
	w, f := schemeBufferWorld(t, buffer, 0)
	w.talkTo(t, f)
	w.bypass(t, npcCommand(f, "support"))
	w.bypass(t, npcCommand(f, "createscheme Free"))
	w.bypass(t, npcCommand(f, "editschemes Buffs Free 1"))
	w.bypass(t, npcCommand(f, "skillselect Buffs Free 1036 1"))
	w.bypass(t, npcCommand(f, "support"))
	w.bypass(t, npcCommand(f, "givebuffs Free 0"))
	if _, ok := w.activeBuff(t, 1036); !ok {
		t.Fatal("a free scheme landed nothing")
	}
	w.bypass(t, npcCommand(f, "menu"))

	menu := datapackPage(t, f, "50002.htm")
	w.onPlayer(t, func(pc *player.Character) { pc.SetCP(0) })
	drainUntilQuiet(t, w.c)
	frames := w.bypass(t, npcCommand(f, "heal"))
	if _, ok := firstOpcode(frames, serverpackets.OpcodeStatusUpdate); !ok {
		t.Fatalf("heal = %x, want a StatusUpdate", opcodes(frames))
	}
	assertSchemePage(t, "heal", frames[len(frames)-2:], menu)
	if cur, maxCP := w.cp(t); cur != maxCP {
		t.Fatalf("CP after heal = %v, want %v", cur, maxCP)
	}

	frames = w.bypass(t, npcCommand(f, "cleanup"))
	if _, ok := firstOpcode(frames, serverpackets.OpcodeUserInfo); !ok {
		t.Fatalf("cleanup = %x, want a UserInfo", opcodes(frames))
	}
	assertSchemePage(t, "cleanup", frames[len(frames)-2:], menu)
	if _, ok := w.activeBuff(t, 1036); ok {
		t.Fatal("cleanup left Magic Barrier")
	}
}

// TestSchemeBufferRestoredSchemes pins BufferManager.load and the
// configured limits: stored schemes come back in name order ignoring case,
// a skill the buffer no longer offers is dropped, schemes past
// BufferMaxSchemesPerChar are dropped, BufferStaticCostPerBuff prices each
// buff, a talker at the maximum cannot create another scheme, and a
// command too malformed to answer sends nothing at all.
func TestSchemeBufferRestoredSchemes(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.Config{MaxSchemes: 2, StaticCost: 100}, schemeBufferBuffs(t), schemeBufferSkills(t))
	w, f := schemeBufferWorld(t, buffer, 0)
	// A server restores the schemes before anyone logs in; nothing reads
	// them here before the first command.
	srvPlayer := w.player
	buffer.Restore([]schemebuffer.Row{
		{OwnerID: srvPlayer, Name: "beta", Skills: "1035,9999,1036"},
		{OwnerID: srvPlayer, Name: "Alpha", Skills: ""},
		{OwnerID: srvPlayer, Name: "gamma", Skills: "1040"},
		{OwnerID: srvPlayer + 1, Name: "a", Skills: "1035"},
		{OwnerID: srvPlayer + 1, Name: "b", Skills: "1035,x"},
		{OwnerID: srvPlayer + 1, Name: "c", Skills: "1035"},
	}, zerolog.Nop())
	if got, want := buffer.Rows(), []schemebuffer.Row{
		{OwnerID: srvPlayer, Name: "Alpha", Skills: ""},
		{OwnerID: srvPlayer, Name: "beta", Skills: "1035,1036"},
		{OwnerID: srvPlayer + 1, Name: "a", Skills: "1035"},
	}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("restored schemes = %+v, want %+v", got, want)
	}

	oid := strconv.Itoa(int(f.ObjectID()))
	w.talkTo(t, f)
	listed := `<font color="LEVEL">Alpha [0 / 20]</font><br1>` +
		`<a action="bypass npc_` + oid + `_givebuffs Alpha 0">Use on Me</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_givebuffs Alpha 0 pet">Use on Pet</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_editschemes Buffs Alpha 1">Edit</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_deletescheme Alpha">Delete</a><br>` +
		`<font color="LEVEL">beta [2 / 20] - cost: 200</font><br1>` +
		`<a action="bypass npc_` + oid + `_givebuffs beta 200">Use on Me</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_givebuffs beta 200 pet">Use on Pet</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_editschemes Buffs beta 1">Edit</a>&nbsp;|&nbsp;` +
		`<a action="bypass npc_` + oid + `_deletescheme beta">Delete</a><br>`
	assertSchemePage(t, "support", w.bypass(t, npcCommand(f, "support")),
		datapackPage(t, f, "50002-1.htm", "%schemes%", listed, "%max_schemes%", "2"))
	assertFrames(t, "createscheme at the maximum", w.bypass(t, npcCommand(f, "createscheme Delta")),
		noticeFrame("Maximum schemes amount is already reached."), []byte{serverpackets.OpcodeActionFailed})

	w.openAnyNpcPage(t)
	for _, command := range []string{"givebuffs beta x", "givebuffs beta", "editschemes Buffs beta 0", "skillselect Buffs Delta 1035 1", ""} {
		if frames := w.bypass(t, npcCommand(f, command)); len(frames) != 0 {
			t.Fatalf("%q = %x, want nothing sent", command, opcodes(frames))
		}
	}
}
