package npcs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The subclass scenarios play a level 75 Gladiator (class 2) at a generic
// village master and add Spellhowler (class 40), a dark elf mystic.
const (
	villageMasterID = 30704
	gladiatorClass  = 2
	spellhowler     = 40
	phantomSummoner = 41
	subclassLevel   = 75
	// learnedSkill is granted to the dark mystic line at level 20, so a new
	// Spellhowler subclass starts knowing it; laterSkill only at 44.
	learnedSkill = 294
	laterSkill   = 248
	// commonRecipe is mk_lesser_healing_potion of gameservertest.RecipeTemplates.
	commonRecipe = 686
)

// Subclass system messages.
const (
	smAddNewSubclass      = 1269
	smSubclassTransferred = 1270
	smSubclassOverweight  = 1894
	smRecipeDeleted       = 848 // S1_HAS_BEEN_DELETED
	smRecipeAdded         = 851 // S1_ADDED
)

// darkMysticLine is the dark mystic line up to Spellhowler. Each class
// keeps the fixture body, with a safe fall height a dark elf's base class
// does not share with the human fighter.
func darkMysticLine() []*player.Template {
	line := make([]*player.Template, 0, 3)
	for _, id := range []int{38, 39, spellhowler, phantomSummoner} {
		tmpl := gameservertest.ClassTemplate()
		tmpl.ID = id
		tmpl.Skills = nil
		tmpl.SafeFallHeightMale, tmpl.SafeFallHeightFemale = 400, 420
		if id == 38 {
			tmpl.Skills = []player.SkillGrant{
				{SkillID: learnedSkill, Level: 1, MinLevel: 20, Cost: 100},
				{SkillID: laterSkill, Level: 1, MinLevel: 44, Cost: 100},
			}
		}
		line = append(line, tmpl)
	}
	return line
}

// subclassPages are the shipped village master pages the dialog reads.
func subclassPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "villagemaster")
	matches, err := filepath.Glob(filepath.Join(dir, "SubClass*.htm"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob subclass pages: %v (%d)", err, len(matches))
	}
	pages := map[string]string{}
	for _, path := range append(matches, filepath.Join(dir, strconv.Itoa(villageMasterID)+".htm")) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		pages["villagemaster/"+filepath.Base(path)] = string(data)
	}
	return pages
}

// subclassWorld is a level 75 Gladiator in the world beside a generic
// village master, with the subclass dialog's page open.
type subclassWorld struct {
	*folkWorld
	folk   *npc.Folk
	master int32
}

// bootSubclassWorld enters the world as the Gladiator, its recipe book
// holding commonRecipe, and opens the master's subclass menu.
func bootSubclassWorld(t *testing.T, extra ...gameservertest.Option) *subclassWorld {
	t.Helper()
	return bootSubclassWorldWith(t, nil, extra...)
}

// bootSubclassWorldWith is bootSubclassWorld with before run once the
// character is stored, before it enters the world.
func bootSubclassWorldWith(t *testing.T, before func(srv *gameservertest.Server, objID int32), extra ...gameservertest.Option) *subclassWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", subclassLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(subclassPages(t)),
		gameservertest.WithClassTemplates(darkMysticLine()...),
		gameservertest.WithSubclassRules(true, 0),
		noBypassReuse,
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	exec(t, srv, `UPDATE characters SET classid = ?, base_class = ? WHERE obj_Id = ?`, gladiatorClass, gladiatorClass, w.player)
	exec(t, srv, `INSERT INTO character_recipebook (charId, recipeId) VALUES (?, ?)`, w.player, commonRecipe)
	if before != nil {
		before(srv, w.player)
	}
	startInWorld(t, w.srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	f := w.spawnFolk(t, folkTemplate("VillageMaster", villageMasterID), 30)
	w.selectFolk(t, f)
	sw := &subclassWorld{folkWorld: w, folk: f, master: f.ObjectID()}
	sw.openMenu(t)
	return sw
}

// openMenu talks to the master and follows its subclass link.
func (w *subclassWorld) openMenu(t *testing.T) {
	t.Helper()
	if _, ok := firstOpcode(w.talk(t, w.folk, false), serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatal("talk opened no page")
	}
	page := w.bypass(t, npcCommand(w.folk, "Link villagemaster/SubClass.htm"))
	if _, html := pageOf(t, page); !strings.Contains(html, "_Subclass 1") {
		t.Fatalf("subclass menu = %q, want its add link", html)
	}
}

func exec(t *testing.T, srv *gameservertest.Server, query string, args ...any) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// addSpellhowler opens the add menu and adds Spellhowler from it,
// returning the add's answer.
func (w *subclassWorld) addSpellhowler(t *testing.T) [][]byte {
	t.Helper()
	w.command(t, "Subclass 1")
	frames := w.command(t, "Subclass 4 "+strconv.Itoa(spellhowler))
	return frames
}

// changeTo opens the change menu anew and changes to slot index.
func (w *subclassWorld) changeTo(t *testing.T, index int) [][]byte {
	t.Helper()
	w.openMenu(t)
	w.command(t, "Subclass 2")
	return w.command(t, "Subclass 5 "+strconv.Itoa(index))
}

// command sends the master command and returns its answer.
func (w *subclassWorld) command(t *testing.T, command string) [][]byte {
	t.Helper()
	return w.bypass(t, "npc_"+strconv.Itoa(int(w.master))+"_"+command)
}

// pageOf returns the one NpcHtmlMessage of frames.
func pageOf(t *testing.T, frames [][]byte) (int32, string) {
	t.Helper()
	frame, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("answer = %x, want a page", opcodes(frames))
	}
	objectID, html, _ := htmlMessage(t, frame)
	return objectID, html
}

var addLink = regexp.MustCompile(`_Subclass 4 (\d+)" msg="1268;([^"]+)">([^<]+)</a><br>`)

func TestSubclassAddMenuListsTheBaseClassesSubclasses(t *testing.T) {
	w := bootSubclassWorld(t)
	_, html := pageOf(t, w.command(t, "Subclass 1"))
	var ids []int
	for _, m := range addLink.FindAllStringSubmatch(html, -1) {
		id, _ := strconv.Atoi(m[1])
		if m[2] != m[3] {
			t.Fatalf("link %q names %q in its confirmation", m[3], m[2])
		}
		ids = append(ids, id)
	}
	// The second professions a Gladiator may take: every one but its own,
	// the Overlord and the Warsmith, in profession id order.
	want := []int{3, 5, 6, 8, 9, 12, 13, 14, 16, 17, 20, 21, 23, 24, 27, 28, 30, 33, 34, 36, 37, 40, 41, 43, 46, 48, 52, 55}
	if !slices.Equal(ids, want) {
		t.Fatalf("offered subclasses = %v, want %v", ids, want)
	}
	if !strings.Contains(html, `msg="1268;Spellhowler">Spellhowler</a>`) {
		t.Fatalf("add menu = %q, want Spellhowler named", html)
	}
}

func TestSubclassAddSwitchesToTheNewSubclass(t *testing.T) {
	w := bootSubclassWorld(t)
	frames := w.addSpellhowler(t)

	// The switch's burst, opened by its cast stop's ActionFailed, then the
	// announcement, the master's page and the closing ActionFailed.
	order := keyOpcodes(frames)
	want := []byte{
		serverpackets.OpcodeActionFailed, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSkillList, serverpackets.OpcodeEtcStatusUpdate,
		serverpackets.OpcodeHennaInfo, serverpackets.OpcodeUserInfo, serverpackets.OpcodeShortCutInit,
		serverpackets.OpcodeSocialAction, serverpackets.OpcodeSkillCoolTime,
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
	}
	if string(order) != string(want) {
		t.Fatalf("add answer = %x, want %x", order, want)
	}
	if id := systemMessageID(frames[lastIndex(frames, serverpackets.OpcodeSystemMessage)]); id != smAddNewSubclass {
		t.Fatalf("message = %d, want ADD_NEW_SUBCLASS", id)
	}
	if _, html := pageOf(t, frames); !strings.Contains(html, "You've added a new subclass") {
		t.Fatalf("page = %q, want SubClass_AddOk", html)
	}
	if f := frames[len(frames)-1]; f[0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("last frame = %x, want ActionFailed", f[0])
	}
	info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)])
	if info.visibleClass != gladiatorClass || info.level != player.SubclassStartLevel {
		t.Fatalf("UserInfo class/level = %d/%d, want base class %d at %d", info.visibleClass, info.level, gladiatorClass, player.SubclassStartLevel)
	}
	if social := frames[firstIndex(frames, serverpackets.OpcodeSocialAction)]; wire.NewReader(social[5:]).ReadInt32() != 15 {
		t.Fatalf("SocialAction = %x, want action 15", social)
	}
	skills := skillListIDs(t, frames[firstIndex(frames, serverpackets.OpcodeSkillList)])
	if !slices.Contains(skills, learnedSkill) || slices.Contains(skills, laterSkill) {
		t.Fatalf("subclass skills = %v, want %d and not %d", skills, learnedSkill, laterSkill)
	}

	w.srv.FlushPersistence(t)
	var classID, index, level int
	var exp int64
	row := w.srv.DB.QueryRowContext(context.Background(), `SELECT class_id, class_index, level, exp FROM character_subclasses WHERE char_obj_id = ?`, w.player)
	if err := row.Scan(&classID, &index, &level, &exp); err != nil {
		t.Fatalf("subclass row: %v", err)
	}
	if classID != spellhowler || index != 1 || level != player.SubclassStartLevel || exp != 1000 {
		t.Fatalf("subclass row = class %d index %d level %d exp %d, want %d/1/40/1000", classID, index, level, exp, spellhowler)
	}
	var skillLevel int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT skill_level FROM character_skills WHERE char_obj_id = ? AND class_index = 1 AND skill_id = ?`, w.player, learnedSkill).Scan(&skillLevel); err != nil || skillLevel != 1 {
		t.Fatalf("starting skill row level = %d, %v; want 1", skillLevel, err)
	}
	// The characters row keeps the base class's progression.
	var rowClass, rowLevel int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT classid, level FROM characters WHERE obj_Id = ?`, w.player).Scan(&rowClass, &rowLevel); err != nil {
		t.Fatal(err)
	}
	if rowLevel != subclassLevel {
		t.Fatalf("characters level = %d, want the base class's %d", rowLevel, subclassLevel)
	}
	w.srv.TickAutosave(t)
	w.srv.FlushPersistence(t)
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT classid, level FROM characters WHERE obj_Id = ?`, w.player).Scan(&rowClass, &rowLevel); err != nil {
		t.Fatal(err)
	}
	if rowClass != spellhowler || rowLevel != subclassLevel {
		t.Fatalf("saved characters class/level = %d/%d, want %d/%d", rowClass, rowLevel, spellhowler, subclassLevel)
	}
}

func TestSubclassChangeReturnsToTheBaseClass(t *testing.T) {
	w := bootSubclassWorld(t)
	w.addSpellhowler(t)

	w.openMenu(t)
	_, html := pageOf(t, w.command(t, "Subclass 2"))
	if !strings.Contains(html, `_Subclass 5 0">Gladiator</a>`) || !strings.Contains(html, `_Subclass 5 1">Spellhowler</a>`) {
		t.Fatalf("change menu = %q, want the base class and the subclass", html)
	}
	if _, html := pageOf(t, w.command(t, "Subclass 5 1")); !strings.Contains(html, "That is your current subclass") {
		t.Fatalf("change to the class played = %q, want SubClass_Current", html)
	}
	frames := w.changeTo(t, 0)
	if n := len(frames); n < 2 || frames[n-1][0] != serverpackets.OpcodeActionFailed || systemMessageID(frames[n-2]) != smSubclassTransferred {
		t.Fatalf("switch answer = %x, want SUBCLASS_TRANSFER_COMPLETED then ActionFailed", opcodes(frames))
	}
	info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)])
	if info.visibleClass != gladiatorClass || info.level != subclassLevel {
		t.Fatalf("UserInfo class/level = %d/%d, want %d/%d", info.visibleClass, info.level, gladiatorClass, subclassLevel)
	}
	if skills := skillListIDs(t, frames[firstIndex(frames, serverpackets.OpcodeSkillList)]); slices.Contains(skills, learnedSkill) {
		t.Fatalf("base class skills = %v, want no subclass skill", skills)
	}
}

func TestSubclassRecipeBookStaysWithTheBaseClass(t *testing.T) {
	w := bootSubclassWorld(t)
	if got := w.recipeBook(t); !slices.Equal(got, []int32{commonRecipe}) {
		t.Fatalf("base class book = %v, want [%d]", got, commonRecipe)
	}
	w.addSpellhowler(t)
	if got := w.recipeBook(t); len(got) != 0 {
		t.Fatalf("subclass book = %v, want empty", got)
	}

	// Deleting on a subclass is reported and changes nothing.
	w.c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookDestroy, commonRecipe))
	frames := drainFrames(t, w.c)
	if len(frames) != 2 || systemMessageID(frames[0]) != smRecipeDeleted || frames[1][0] != serverpackets.OpcodeRecipeBookItemList {
		t.Fatalf("delete answer = %x, want the deletion message and the page", opcodes(frames))
	}
	w.srv.FlushPersistence(t)
	var n int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM character_recipebook WHERE charId = ?`, w.player).Scan(&n); err != nil || n != 1 {
		t.Fatalf("recipe rows = %d, %v; want the base class's row kept", n, err)
	}

	w.changeTo(t, 0)
	if got := w.recipeBook(t); !slices.Equal(got, []int32{commonRecipe}) {
		t.Fatalf("book back on the base class = %v, want [%d]", got, commonRecipe)
	}
}

func TestSubclassRegistersNoRecipe(t *testing.T) {
	const recipeItem = 6926 // Recipe: Lesser Healing Potion, which writes commonRecipe
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: learnedSkill, Level: 1},
		{ID: laterSkill, Level: 1},
		{ID: modelskill.CreateCommonSkillID, Level: 1, Activation: modelskill.ActivationPassive},
	}), gamesql.NewCharacterSkillStore(db))
	line := darkMysticLine()
	line[0].Skills = append(line[0].Skills, player.SkillGrant{SkillID: int(modelskill.CreateCommonSkillID), Level: 1, MinLevel: 20, Cost: 100})
	items := item.NewTable(append(slices.Clone(gameservertest.ItemTemplates().All()), &item.Template{
		ID: recipeItem, Name: "Recipe: Lesser Healing Potion", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true,
		EtcItem: &item.EtcItemDetail{Type: item.EtcItemRecipe, Handler: "Recipes"},
	}))
	var recipeObject int32
	w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
		recipeObject = srv.GiveItem(t, objID, recipeItem, 1)
	}, gameservertest.WithSkills(skills), gameservertest.WithClassTemplates(line...), gameservertest.WithItemTemplates(items))
	w.addSpellhowler(t)

	// The recipe item is used up and the recipe reported added, but the
	// book, the base class's, takes nothing on a subclass.
	frames := w.send(t, encodeUseItem(recipeObject))
	if len(frames) != 2 || systemMessageID(frames[0]) != smRecipeAdded || frames[1][0] != serverpackets.OpcodeRecipeBookItemList {
		t.Fatalf("register answer = %x, want S1_ADDED then the page", opcodes(frames))
	}
	if got := w.recipeBook(t); len(got) != 0 {
		t.Fatalf("subclass book = %v, want empty", got)
	}
	if inv := w.srv.PlayerInventory(t, w.player); inv.ItemByObjectID(recipeObject) != nil {
		t.Fatal("recipe item kept, want it used up")
	}
	w.srv.FlushPersistence(t)
	var n int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM character_recipebook WHERE charId = ?`, w.player).Scan(&n); err != nil || n != 1 {
		t.Fatalf("recipe rows = %d, %v; want the base class's one row alone", n, err)
	}
}

func TestSubclassReplaceStartsTheSlotOver(t *testing.T) {
	w := bootSubclassWorld(t)
	w.addSpellhowler(t)
	exec(t, w.srv, `INSERT INTO character_shortcuts (char_obj_id, slot, page, type, id, level, class_index) VALUES (?, 3, 0, 'SKILL', ?, 1, 1)`, w.player, learnedSkill)

	w.openMenu(t)
	_, html := pageOf(t, w.command(t, "Subclass 3"))
	if !strings.Contains(html, `_Subclass 6 1">Spellhowler</a>`) || strings.Contains(html, "Subclass 6 2") {
		t.Fatalf("replace menu = %q, want slot 1 alone", html)
	}
	_, html = pageOf(t, w.command(t, "Subclass 6 1"))
	if !strings.Contains(html, `_Subclass 7 1 41" msg="1445;">Phantom Summoner</a>`) || strings.Contains(html, `Subclass 7 1 40"`) {
		t.Fatalf("replace choice = %q, want Phantom Summoner offered and the held Spellhowler not", html)
	}
	frames := w.command(t, "Subclass 7 1 "+strconv.Itoa(phantomSummoner))
	if n := len(frames); n < 3 || frames[n-1][0] != serverpackets.OpcodeActionFailed || systemMessageID(frames[n-3]) != smAddNewSubclass {
		t.Fatalf("replace answer = %x, want ADD_NEW_SUBCLASS, the page and ActionFailed last", opcodes(frames))
	}
	if _, html := pageOf(t, frames); !strings.Contains(html, "You've changed subclasses") {
		t.Fatalf("page = %q, want SubClass_ModifyOk", html)
	}
	if sc := frames[lastIndex(frames, serverpackets.OpcodeShortCutInit)]; wire.NewReader(sc[1:]).ReadInt32() != 0 {
		t.Fatalf("ShortCutInit = %x, want the replaced slot's bar empty", sc)
	}

	w.srv.FlushPersistence(t)
	var classID, level int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT class_id, level FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&classID, &level); err != nil {
		t.Fatal(err)
	}
	if classID != phantomSummoner || level != player.SubclassStartLevel {
		t.Fatalf("slot 1 = class %d level %d, want %d at 40", classID, level, phantomSummoner)
	}
	var shortcuts int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM character_shortcuts WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&shortcuts); err != nil || shortcuts != 0 {
		t.Fatalf("slot 1 shortcuts = %d, %v; want them cleared", shortcuts, err)
	}
}

func TestSubclassAddRefusals(t *testing.T) {
	t.Run("quests not done", func(t *testing.T) {
		w := bootSubclassWorld(t, gameservertest.WithSubclassRules(false, 0))
		if _, html := pageOf(t, w.addSpellhowler(t)); !strings.Contains(html, "You aren't eligible to add a subclass") {
			t.Fatalf("add without the quests = %q, want SubClass_Fail", html)
		}
	})
	t.Run("reuse delay", func(t *testing.T) {
		w := bootSubclassWorld(t, gameservertest.WithSubclassRules(true, time.Hour))
		w.addSpellhowler(t)
		frames := w.changeTo(t, 0)
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("change inside the reuse delay = %x, want ActionFailed alone", opcodes(frames))
		}
	})
	t.Run("overweight", func(t *testing.T) {
		w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
			srv.GiveItem(t, objID, 20, 1)
		}, gameservertest.WithInventorySlots(1, 1))
		frames := w.command(t, "Subclass 1")
		if len(frames) != 2 || systemMessageID(frames[0]) != smSubclassOverweight || frames[1][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("add menu with a full inventory = %x, want the overweight message then ActionFailed", opcodes(frames))
		}
	})
}

// recipeBook opens the common page and returns its recipe ids.
func (w *subclassWorld) recipeBook(t *testing.T) []int32 {
	t.Helper()
	w.c.Send(encodeRecipeRequest(clientpackets.OpcodeRequestRecipeBookOpen, 1))
	frames := drainFrames(t, w.c)
	frame, ok := firstOpcode(frames, serverpackets.OpcodeRecipeBookItemList)
	if !ok {
		t.Fatalf("book answer = %x, want RecipeBookItemList", opcodes(frames))
	}
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // page
	r.ReadInt32() // max MP
	ids := make([]int32, r.ReadInt32())
	for i := range ids {
		ids[i] = r.ReadInt32()
		r.ReadInt32()
	}
	return ids
}

func encodeRecipeRequest(opcode byte, value int32) []byte {
	w := wire.NewPacketWriter(opcode)
	w.WriteInt32(value)
	return w.Bytes()
}

// keyOpcodes keeps the frames a class change orders.
func keyOpcodes(frames [][]byte) []byte {
	var out []byte
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeUserInfo, serverpackets.OpcodeSkillList, serverpackets.OpcodeEtcStatusUpdate,
			serverpackets.OpcodeHennaInfo, serverpackets.OpcodeShortCutInit, serverpackets.OpcodeSocialAction,
			serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeNpcHtmlMessage,
			serverpackets.OpcodeActionFailed:
			out = append(out, f[0])
		}
	}
	return out
}

func firstIndex(frames [][]byte, opcode byte) int {
	return slices.IndexFunc(frames, func(f []byte) bool { return f[0] == opcode })
}

func lastIndex(frames [][]byte, opcode byte) int {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i][0] == opcode {
			return i
		}
	}
	return -1
}

// userInfoHead is the start of a UserInfo frame.
type userInfoHead struct {
	visibleClass, level int32
}

func decodeUserInfoHead(t *testing.T, frame []byte) userInfoHead {
	t.Helper()
	if frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("opcode = %#x, want UserInfo", frame[0])
	}
	r := wire.NewReader(frame[1:])
	for range 5 { // x, y, z, heading, object id
		r.ReadInt32()
	}
	r.ReadString()
	r.ReadInt32() // race
	r.ReadInt32() // sex
	return userInfoHead{visibleClass: r.ReadInt32(), level: r.ReadInt32()}
}

func skillListIDs(t *testing.T, frame []byte) []int {
	t.Helper()
	r := wire.NewReader(frame[1:])
	ids := make([]int, r.ReadInt32())
	for i := range ids {
		r.ReadInt32() // passive
		r.ReadInt32() // level
		ids[i] = int(r.ReadInt32())
		r.ReadUint8() // disabled
	}
	return ids
}

func TestSubclassRestartKeepsPlayingTheSubclass(t *testing.T) {
	// No selection reuse delay: the scenario selects the character twice.
	w := bootSubclassWorld(t, gameservertest.WithReuseDelays(0, 0))
	w.addSpellhowler(t)

	// The list shows the subclass's level beside the base class's model.
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	frames := drainFrames(t, w.c)
	list, ok := firstOpcode(frames, serverpackets.OpcodeCharSelectInfo)
	if !ok {
		t.Fatalf("restart answer = %x, want CharSelectInfo", opcodes(frames))
	}
	r := wire.NewReader(list[1:])
	if n := r.ReadInt32(); n != 1 {
		t.Fatalf("listed characters = %d, want 1", n)
	}
	r.ReadString() // name
	r.ReadInt32()  // object id
	r.ReadString() // account
	for range 5 {  // session, clan, builder level, sex, race
		r.ReadInt32()
	}
	shown := r.ReadInt32()
	r.ReadInt32() // active
	for range 3 {
		r.ReadInt32()
	}
	r.ReadFloat64()
	r.ReadFloat64()
	r.ReadInt32() // sp
	r.ReadInt64() // exp
	if level := r.ReadInt32(); shown != gladiatorClass || level != player.SubclassStartLevel {
		t.Fatalf("listed class/level = %d/%d, want base class %d at the subclass's level 40", shown, level, gladiatorClass)
	}

	// Selecting it plays the subclass again, with its skills.
	w.c.Send(encodeRequestGameStart(0))
	if reply := w.c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo", reply[0])
	}
	selected := w.c.Read()
	if selected[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected", selected[0])
	}
	r = wire.NewReader(selected[1:])
	r.ReadString()
	r.ReadInt32()
	r.ReadString()
	for range 5 { // session, clan, unknown, sex, race
		r.ReadInt32()
	}
	classID := r.ReadInt32()
	r.ReadInt32()
	for range 3 {
		r.ReadInt32()
	}
	r.ReadFloat64()
	r.ReadFloat64()
	r.ReadInt32()
	r.ReadInt64()
	if level := r.ReadInt32(); classID != spellhowler || level != player.SubclassStartLevel {
		t.Fatalf("selected class/level = %d/%d, want %d at 40", classID, level, spellhowler)
	}
	burst := enterWorld(t, w.srv, w.c)
	if skills := skillListIDs(t, burst[firstIndex(burst, serverpackets.OpcodeSkillList)]); !slices.Contains(skills, learnedSkill) {
		t.Fatalf("entered skills = %v, want the subclass's %d", skills, learnedSkill)
	}
	info := decodeUserInfoHead(t, burst[firstIndex(burst, serverpackets.OpcodeUserInfo)])
	if info.visibleClass != gladiatorClass || info.level != player.SubclassStartLevel {
		t.Fatalf("entered UserInfo class/level = %d/%d, want %d/40", info.visibleClass, info.level, gladiatorClass)
	}
}

func TestSubclassFallsFromTheBaseClassesSafeHeight(t *testing.T) {
	w := bootSubclassWorld(t)
	w.addSpellhowler(t)
	// The Gladiator's body falls 250 safely; the Spellhowler template's 400
	// does not apply to it.
	w.c.Send(encodeValidatePosition(location.Location{X: w.at.X, Y: w.at.Y, Z: w.at.Z - 300}))
	frames := drainFrames(t, w.c)
	i := firstIndex(frames, serverpackets.OpcodeSystemMessage)
	if i < 0 || systemMessageID(frames[i]) != 296 {
		t.Fatalf("300-unit drop answer = %x, want FALL_DAMAGE_S1", opcodes(frames))
	}
}

func encodeValidatePosition(at location.Location) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	w.WriteInt32(int32(at.X))
	w.WriteInt32(int32(at.Y))
	w.WriteInt32(int32(at.Z))
	w.WriteInt32(0) // heading
	w.WriteInt32(0) // boat
	return w.Bytes()
}

func TestSubclassSwitchGivesTheClassesFreeSkills(t *testing.T) {
	// learnedSkill as a free grant: a class at level 20 or above holds it
	// whether or not a character_skills row says so.
	line := darkMysticLine()
	line[0].Skills[0].Cost = 0
	w := bootSubclassWorld(t, gameservertest.WithClassTemplates(line...))
	w.addSpellhowler(t)
	w.changeTo(t, 0)

	// The subclass's stored skills lose it; switching back must grant it
	// again from the class's own grants.
	w.srv.FlushPersistence(t)
	exec(t, w.srv, `DELETE FROM character_skills WHERE char_obj_id = ? AND class_index = 1 AND skill_id = ?`, w.player, learnedSkill)
	frames := w.changeTo(t, 1)
	if skills := skillListIDs(t, frames[firstIndex(frames, serverpackets.OpcodeSkillList)]); !slices.Contains(skills, learnedSkill) || slices.Contains(skills, laterSkill) {
		t.Fatalf("subclass skills after the switch = %v, want the free %d and not %d", skills, learnedSkill, laterSkill)
	}
}

func TestSubclassReplaceOfTheActiveSlotStartsItAtForty(t *testing.T) {
	const (
		subLevel = 78
		subExp   = 5000
	)
	// The character plays a level 78 Spellhowler in slot 1.
	w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
		exec(t, srv, `INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, ?, 0, ?, 1)`, objID, spellhowler, subExp, subLevel)
		exec(t, srv, `UPDATE characters SET classid = ? WHERE obj_Id = ?`, spellhowler, objID)
	})
	w.command(t, "Subclass 3")
	w.command(t, "Subclass 6 1")
	frames := w.command(t, "Subclass 7 1 "+strconv.Itoa(phantomSummoner))
	if n := len(frames); n < 3 || frames[n-1][0] != serverpackets.OpcodeActionFailed || systemMessageID(frames[n-3]) != smAddNewSubclass {
		t.Fatalf("replace answer = %x, want ADD_NEW_SUBCLASS, the page and ActionFailed last", opcodes(frames))
	}
	info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)])
	if info.visibleClass != gladiatorClass || info.level != player.SubclassStartLevel {
		t.Fatalf("UserInfo class/level = %d/%d, want %d at 40", info.visibleClass, info.level, gladiatorClass)
	}

	w.srv.FlushPersistence(t)
	var classID, level int
	var exp int64
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT class_id, level, exp FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&classID, &level, &exp); err != nil {
		t.Fatal(err)
	}
	if classID != phantomSummoner || level != player.SubclassStartLevel || exp != 1000 {
		t.Fatalf("slot 1 = class %d level %d exp %d, want %d at 40 with 1000", classID, level, exp, phantomSummoner)
	}
	// The autosave keeps the fresh progression too.
	w.srv.TickAutosave(t)
	w.srv.FlushPersistence(t)
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT level, exp FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&level, &exp); err != nil {
		t.Fatal(err)
	}
	if level != player.SubclassStartLevel || exp != 1000 {
		t.Fatalf("slot 1 after the autosave = level %d exp %d, want 40 with 1000", level, exp)
	}
}

// subclassFault fails, or panics on, the class change writes whose op it
// names, once armed.
type subclassFault struct {
	armed atomic.Bool
	op    string
	panic bool
}

func (f *subclassFault) check(op string, _ int) error {
	if !f.armed.Load() || op != f.op {
		return nil
	}
	if f.panic {
		f.armed.Store(false)
		panic("subclass store: injected panic")
	}
	return errors.New("subclass store: injected failure")
}

func TestSubclassAddWriteFailureChangesNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		panic bool
	}{
		{"insert fails", false},
		// The persistence lane recovers a panicking job: the change still
		// ends, and the lock is free again.
		{"insert panics", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fault := &subclassFault{op: "insert", panic: tc.panic}
			w := bootSubclassWorld(t, gameservertest.WithSubclassFault(fault.check))
			fault.armed.Store(true)
			frames := w.addSpellhowler(t)
			if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("failed add answer = %x, want ActionFailed alone", opcodes(frames))
			}
			w.openMenu(t)
			if _, html := pageOf(t, w.command(t, "Subclass 2")); !strings.Contains(html, "_Subclass 1") || strings.Contains(html, "Subclass 5") {
				t.Fatalf("change menu after the failed add = %q, want SubClass_ChangeNo", html)
			}
			w.srv.FlushPersistence(t)
			var n int
			if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM character_subclasses WHERE char_obj_id = ?`, w.player).Scan(&n); err != nil || n != 0 {
				t.Fatalf("subclass rows = %d, %v; want none", n, err)
			}
			if !tc.panic {
				return
			}
			// The lock was released: the next add goes through.
			w.openMenu(t)
			frames = w.addSpellhowler(t)
			if _, html := pageOf(t, frames); !strings.Contains(html, "You've added a new subclass") {
				t.Fatalf("add after the panicked one = %x, want SubClass_AddOk", opcodes(frames))
			}
		})
	}
}

func TestSubclassReplaceWriteFailureRevertsToTheBaseClass(t *testing.T) {
	fault := &subclassFault{op: "delete"}
	w := bootSubclassWorld(t, gameservertest.WithSubclassFault(fault.check))
	w.addSpellhowler(t)
	w.srv.FlushPersistence(t)

	fault.armed.Store(true)
	w.openMenu(t)
	w.command(t, "Subclass 3")
	w.command(t, "Subclass 6 1")
	frames := w.command(t, "Subclass 7 1 "+strconv.Itoa(phantomSummoner))
	if n := len(frames); n < 2 || frames[n-1][0] != serverpackets.OpcodeActionFailed || frames[n-2][0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("failed replace answer = %x, want the reverted text then ActionFailed", opcodes(frames))
	}
	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); ok {
		t.Fatal("failed replace sent a page, want none")
	}
	if text := systemMessageText(t, frames[len(frames)-2]); text != "The sub class could not be added, you have been reverted to your base class." {
		t.Fatalf("failed replace message = %q, want the reverted text", text)
	}
	info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)])
	if info.visibleClass != gladiatorClass || info.level != subclassLevel {
		t.Fatalf("UserInfo class/level = %d/%d, want the base class %d at %d", info.visibleClass, info.level, gladiatorClass, subclassLevel)
	}
	if skills := skillListIDs(t, frames[firstIndex(frames, serverpackets.OpcodeSkillList)]); slices.Contains(skills, learnedSkill) {
		t.Fatalf("base class skills = %v, want no subclass skill", skills)
	}
	// The slot is gone in memory.
	w.openMenu(t)
	if _, html := pageOf(t, w.command(t, "Subclass 2")); strings.Contains(html, "Subclass 5") {
		t.Fatalf("change menu after the failed replace = %q, want SubClass_ChangeNo", html)
	}

	// The slot's row, which the failed delete left, is not written over
	// with the replacing class.
	w.srv.FlushPersistence(t)
	var classID int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT class_id FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if classID != spellhowler {
		t.Fatalf("slot 1 row class = %d, want the stored %d kept", classID, spellhowler)
	}
	var rowClass int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT classid FROM characters WHERE obj_Id = ?`, w.player).Scan(&rowClass); err != nil {
		t.Fatal(err)
	}
	w.srv.TickAutosave(t)
	w.srv.FlushPersistence(t)
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT class_id FROM character_subclasses WHERE char_obj_id = ? AND class_index = 1`, w.player).Scan(&classID); err != nil || classID != spellhowler {
		t.Fatalf("slot 1 row class after the autosave = %d, %v; want %d", classID, err, spellhowler)
	}
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT classid FROM characters WHERE obj_Id = ?`, w.player).Scan(&rowClass); err != nil || rowClass != gladiatorClass {
		t.Fatalf("characters classid after the autosave = %d, %v; want %d", rowClass, err, gladiatorClass)
	}
}

// systemMessageText returns the text parameter of a one-parameter S1
// system message.
func systemMessageText(t *testing.T, frame []byte) string {
	t.Helper()
	r := wire.NewReader(frame[1:])
	r.ReadInt32() // id
	if n := r.ReadInt32(); n != 1 {
		t.Fatalf("system message parameters = %d, want 1", n)
	}
	if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamText {
		t.Fatalf("system message parameter type = %d, want text", typ)
	}
	return r.ReadString()
}
