package npcs

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/classmaster"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The class manager scenarios play the level 20 human fighter (class 0) at
// the shipped class manager, Hasha (50000), changing to Warrior (class 1).
const (
	classManagerID    = 50000
	warriorOccupation = 1
	// The first change costs classPrice adena and hands out rewardCount
	// potions.
	classPrice  = 1000
	rewardItem  = 20
	rewardCount = 3
	// boughtSkill and freeSkill are Warrior grants at level 20, the first
	// bought with SP, the second free; fighterSkill is the human fighter's
	// level 5 grant, bought with SP.
	boughtSkill  = 900100
	freeSkill    = 900101
	fighterSkill = 3
)

// Occupation change system messages.
const (
	smClassTransfer       = 1308
	smNotEnoughItems      = 351
	smAdenaDisappeared    = 672
	smPickedUpS2S1        = 29
	occupationChangeSkill = 5103
)

// classManagerPages are the shipped class manager pages.
func classManagerPages(t *testing.T) map[string]string {
	t.Helper()
	dir := datapack.Path(t, "data", "html", "mods", "classmaster")
	matches, err := filepath.Glob(filepath.Join(dir, "*.htm"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("glob class manager pages: %v (%d)", err, len(matches))
	}
	pages := map[string]string{}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		pages["mods/classmaster/"+filepath.Base(path)] = string(data)
	}
	return pages
}

// warriorTemplate is the Warrior, granting boughtSkill and freeSkill at
// level 20 on top of the human fighter's grants.
func warriorTemplate() *player.Template {
	tmpl := gameservertest.ClassTemplate()
	tmpl.ID = warriorOccupation
	tmpl.Skills = []player.SkillGrant{
		{SkillID: boughtSkill, Level: 1, MinLevel: 20, Cost: 10},
		{SkillID: freeSkill, Level: 1, MinLevel: 20, Cost: 0},
	}
	return tmpl
}

// classManagerSkills knows every skill the scenarios grant.
func classManagerSkills(t *testing.T) gameservertest.Option {
	t.Helper()
	ids := []modelskill.ID{boughtSkill, freeSkill, fighterSkill, 900001}
	for _, ref := range modelskill.NobleSkills() {
		ids = append(ids, ref.ID)
	}
	defs := make([]modelskill.Definition, 0, len(ids))
	for _, id := range ids {
		defs = append(defs, modelskill.Definition{ID: id, Level: 1, Activation: modelskill.ActivationPassive})
	}
	db := sqltest.SharedDB(t)
	return gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db)))
}

// classManagerConfig offers the first change for classPrice adena with
// rewardCount potions and the second for free; the third is not offered.
func classManagerConfig(t *testing.T) classmaster.Config {
	t.Helper()
	jobs, err := classmaster.ParseJobs("1;[57(" + strconv.Itoa(classPrice) + ")];[20(" + strconv.Itoa(rewardCount) + ")];2;[];[]")
	if err != nil {
		t.Fatal(err)
	}
	return classmaster.NewConfig(false, jobs)
}

// classManagerWorld is the fighter in the world beside the class manager,
// its first page open.
type classManagerWorld struct {
	*folkWorld
	folk *npc.Folk
}

// bootClassManagerWorld enters the world as the fighter holding adena,
// beside the class manager, and opens its first page.
func bootClassManagerWorld(t *testing.T, adena int32, extra ...gameservertest.Option) *classManagerWorld {
	t.Helper()
	return bootClassManagerWorldWith(t, adena, nil, extra...)
}

// bootClassManagerWorldWith is bootClassManagerWorld with before run once
// the character is stored, before it enters the world.
func bootClassManagerWorldWith(t *testing.T, adena int32, before func(srv *gameservertest.Server, objID int32), extra ...gameservertest.Option) *classManagerWorld {
	t.Helper()
	opts := append([]gameservertest.Option{
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(classManagerPages(t)),
		gameservertest.WithClassMaster(classManagerConfig(t)),
		gameservertest.WithClassTemplates(warriorTemplate()),
		classManagerSkills(t),
		noBypassReuse,
	}, extra...)
	srv := gameservertest.Boot(t, opts...)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	if adena > 0 {
		srv.GiveItem(t, w.player, item.AdenaID, adena)
	}
	if before != nil {
		before(srv, w.player)
	}
	startInWorld(t, srv, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	f := w.spawnFolk(t, folkTemplate("ClassMaster", classManagerID), 30)
	w.selectFolk(t, f)
	if _, html := pageOf(t, w.talk(t, f, false)); !strings.Contains(html, "_1stClass") {
		t.Fatalf("first page = %q, want the first class transfer link", html)
	}
	return &classManagerWorld{folkWorld: w, folk: f}
}

// command sends the manager command and returns its answer.
func (w *classManagerWorld) command(t *testing.T, command string) [][]byte {
	t.Helper()
	return w.bypass(t, npcCommand(w.folk, command))
}

// fromFirstPage talks to the manager again, reopening its first page, and
// sends command from it.
func (w *classManagerWorld) fromFirstPage(t *testing.T, command string) [][]byte {
	t.Helper()
	w.talk(t, w.folk, false)
	return w.command(t, command)
}

// changeOrder keeps the frames an occupation change answers with, in
// order, and the system message ids among them.
func changeOrder(frames [][]byte) ([]byte, []int32) {
	var order []byte
	var messages []int32
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			messages = append(messages, systemMessageID(f))
			order = append(order, f[0])
		case serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSkillList, serverpackets.OpcodeHennaInfo,
			serverpackets.OpcodeUserInfo, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed:
			order = append(order, f[0])
		}
	}
	return order, messages
}

func TestClassManagerMenusListTheNextOccupations(t *testing.T) {
	w := bootClassManagerWorld(t, 0)
	master := strconv.Itoa(int(w.folk.ObjectID()))

	frames := w.command(t, "1stClass")
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("answer = %x, want the menu then ActionFailed", got)
	}
	objectID, html := pageOf(t, frames)
	if objectID != w.folk.ObjectID() {
		t.Fatalf("menu names object %d, want the manager %d", objectID, w.folk.ObjectID())
	}
	for _, want := range []string{
		`<a action="bypass -h npc_` + master + `_change_class 1">Warrior</a><br>` +
			`<a action="bypass -h npc_` + master + `_change_class 4">Human Knight</a><br>` +
			`<a action="bypass -h npc_` + master + `_change_class 7">Rogue</a><br>`,
		`<tr><td><font color="LEVEL">1000</font></td><td>&#57</td></tr>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("first menu = %q, want %q", html, want)
		}
	}

	// A base class cannot take a second occupation: the page names the
	// level the first change needs.
	_, html = pageOf(t, w.fromFirstPage(t, "2ndClass"))
	if !strings.Contains(html, "Come back here when you reach level 40 to change your class.") {
		t.Fatalf("second menu = %q, want the level 40 page", html)
	}

	// The third change is not offered: a built page names the next one
	// that is.
	_, html = pageOf(t, w.fromFirstPage(t, "3rdClass"))
	if html != "<html><body>Come back here when you reached level 20 to change your class.<br></body></html>" {
		t.Fatalf("third menu = %q, want the plain level 20 notice", html)
	}
}

func TestClassManagerChangesClassForItsPrice(t *testing.T) {
	w := bootClassManagerWorld(t, classPrice+5, gameservertest.WithAutoLearnSkills())
	w.command(t, "1stClass")

	frames := w.command(t, "change_class "+strconv.Itoa(warriorOccupation))
	order, messages := changeOrder(frames)
	wantOrder := []byte{
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSkillList,
		serverpackets.OpcodeHennaInfo, serverpackets.OpcodeUserInfo,
		serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed,
	}
	if string(order) != string(wantOrder) || !slices.Equal(messages, []int32{smAdenaDisappeared, smPickedUpS2S1, smClassTransfer}) {
		t.Fatalf("change answer = %x messages %v, want %x messages %v", order, messages, wantOrder, []int32{smAdenaDisappeared, smPickedUpS2S1, smClassTransfer})
	}
	cast := frames[firstIndex(frames, serverpackets.OpcodeMagicSkillUse)]
	r := wire.NewReader(cast[1:])
	if caster, target, skill, level, hit := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != w.player || target != w.player || skill != occupationChangeSkill || level != 1 || hit != 1000 {
		t.Fatalf("MagicSkillUse = %d->%d skill %d level %d hit %d, want the player's 5103 level 1 over 1000 ms", caster, target, skill, level, hit)
	}
	if info := decodeUserInfoHead(t, frames[lastIndex(frames, serverpackets.OpcodeUserInfo)]); info.visibleClass != warriorOccupation {
		t.Fatalf("UserInfo class = %d, want %d", info.visibleClass, warriorOccupation)
	}
	if _, html := pageOf(t, frames); !strings.Contains(html, `You have now become a <font color="LEVEL">Warrior</font>.`) {
		t.Fatalf("page = %q, want the Warrior confirmation", html)
	}
	// Every skill now available is granted.
	skills := skillListIDs(t, frames[firstIndex(frames, serverpackets.OpcodeSkillList)])
	if !slices.Contains(skills, boughtSkill) || !slices.Contains(skills, freeSkill) || !slices.Contains(skills, fighterSkill) {
		t.Fatalf("skill list = %v, want %d, %d and %d", skills, boughtSkill, freeSkill, fighterSkill)
	}
	inv := w.srv.PlayerInventory(t, w.player)
	if adena, potions := inv.ItemCount(item.AdenaID, -1, true), inv.ItemCount(rewardItem, -1, true); adena != 5 || potions != rewardCount {
		t.Fatalf("inventory adena/potions = %d/%d, want 5/%d", adena, potions, rewardCount)
	}

	// The bought grant is stored at once; the free one stays level-derived.
	w.srv.FlushPersistence(t)
	var stored []int
	rows, err := w.srv.DB.QueryContext(context.Background(), `SELECT skill_id FROM character_skills WHERE char_obj_id = ? AND class_index = 0`, w.player)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, id)
	}
	if !slices.Contains(stored, boughtSkill) || slices.Contains(stored, freeSkill) {
		t.Fatalf("stored skills = %v, want %d and not %d", stored, boughtSkill, freeSkill)
	}

	// The next save writes the new class as both the class played and the
	// base class.
	w.srv.TickAutosave(t)
	w.srv.FlushPersistence(t)
	var classID, baseClass int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT classid, base_class FROM characters WHERE obj_Id = ?`, w.player).Scan(&classID, &baseClass); err != nil {
		t.Fatal(err)
	}
	if classID != warriorOccupation || baseClass != warriorOccupation {
		t.Fatalf("saved class/base class = %d/%d, want %d/%d", classID, baseClass, warriorOccupation, warriorOccupation)
	}
}

func TestClassManagerRefusesATalkerShortOfThePrice(t *testing.T) {
	w := bootClassManagerWorld(t, classPrice-1)
	w.command(t, "1stClass")

	frames := w.command(t, "change_class "+strconv.Itoa(warriorOccupation))
	if order, messages := changeOrder(frames); string(order) != string([]byte{serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed}) || !slices.Equal(messages, []int32{smNotEnoughItems}) {
		t.Fatalf("answer = %x messages %v, want NOT_ENOUGH_ITEMS then ActionFailed", order, messages)
	}
	if adena := w.srv.PlayerInventory(t, w.player).ItemCount(item.AdenaID, -1, true); adena != classPrice-1 {
		t.Fatalf("adena = %d, want %d kept", adena, classPrice-1)
	}
	w.srv.TickAutosave(t)
	w.srv.FlushPersistence(t)
	var classID int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT classid FROM characters WHERE obj_Id = ?`, w.player).Scan(&classID); err != nil || classID != 0 {
		t.Fatalf("saved class = %d, %v; want 0", classID, err)
	}
}

func TestClassManagerChangeWithoutAutoLearnGrantsNoSkill(t *testing.T) {
	w := bootClassManagerWorld(t, classPrice)
	w.command(t, "1stClass")

	frames := w.command(t, "change_class "+strconv.Itoa(warriorOccupation))
	if firstIndex(frames, serverpackets.OpcodeSkillList) >= 0 {
		t.Fatalf("change answer = %x, want no skill list without AutoLearnSkills", opcodes(frames))
	}
	if _, html := pageOf(t, frames); !strings.Contains(html, "Warrior") {
		t.Fatalf("page = %q, want the Warrior confirmation", html)
	}
}

func TestClassManagerLearnSkillsGrantsEveryAvailableSkill(t *testing.T) {
	w := bootClassManagerWorld(t, 0)
	frames := w.command(t, "learn_skills")
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeSkillList, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("answer = %x, want SkillList then ActionFailed", got)
	}
	if skills := skillListIDs(t, frames[0]); !slices.Contains(skills, fighterSkill) || slices.Contains(skills, 900001) {
		t.Fatalf("skill list = %v, want %d and not the level 50 grant", skills, fighterSkill)
	}
	w.srv.FlushPersistence(t)
	var level int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT skill_level FROM character_skills WHERE char_obj_id = ? AND skill_id = ?`, w.player, fighterSkill).Scan(&level); err != nil || level != 1 {
		t.Fatalf("stored skill level = %d, %v; want 1", level, err)
	}
}

func TestClassManagerGrantsNoblesseOnce(t *testing.T) {
	w := bootClassManagerWorld(t, 0)

	frames := w.command(t, "become_noble")
	want := []byte{serverpackets.OpcodeSkillList, serverpackets.OpcodeUserInfo, serverpackets.OpcodeUserInfo, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}
	if got := opcodes(frames); string(got) != string(want) {
		t.Fatalf("answer = %x, want %x", got, want)
	}
	skills := skillListIDs(t, frames[0])
	for _, ref := range modelskill.NobleSkills() {
		if !slices.Contains(skills, int(ref.ID)) {
			t.Fatalf("skill list = %v, want noble skill %d", skills, ref.ID)
		}
	}
	if _, html := pageOf(t, frames); !strings.Contains(html, "Congratulations on becoming a Noble") {
		t.Fatalf("page = %q, want 50000-6", html)
	}
	// The status is stored at once, ahead of any save.
	w.srv.FlushPersistence(t)
	var noble int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT nobless FROM characters WHERE obj_Id = ?`, w.player).Scan(&noble); err != nil || noble != 1 {
		t.Fatalf("nobless = %d, %v; want 1", noble, err)
	}

	frames = w.fromFirstPage(t, "become_noble")
	if got := opcodes(frames); string(got) != string([]byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}) {
		t.Fatalf("second answer = %x, want the page then ActionFailed", got)
	}
	if _, html := pageOf(t, frames); !strings.Contains(html, "You already got noblesse status") {
		t.Fatalf("second page = %q, want 50000-5", html)
	}
}

// TestClassManagerChangeRefreshesTheClanRow pins the clan side of an
// occupation change: right after the transfer message, every online member,
// the changer included, gets the changer's roster row with its new class.
func TestClassManagerChangeRefreshesTheClanRow(t *testing.T) {
	w := bootClassManagerWorld(t, classPrice, gameservertest.WithClanSeed(func(db *sql.DB) {
		for _, q := range []string{
			`UPDATE characters SET clanid = 600 WHERE char_name = 'Talker'`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) SELECT 600, 'Changers', 1, obj_Id FROM characters WHERE char_name = 'Talker'`,
		} {
			if _, err := db.ExecContext(context.Background(), q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	}))
	w.command(t, "1stClass")

	frames := w.command(t, "change_class "+strconv.Itoa(warriorOccupation))
	transfer := slices.IndexFunc(frames, func(f []byte) bool {
		return f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(f) == smClassTransfer
	})
	if transfer < 0 || transfer+1 >= len(frames) || frames[transfer+1][0] != serverpackets.OpcodePledgeShowMemberListUpdate {
		t.Fatalf("change answer = %x, want PledgeShowMemberListUpdate right after CLASS_TRANSFER", opcodes(frames))
	}
	r := wire.NewReader(frames[transfer+1][1:])
	name, level, class := r.ReadString(), r.ReadInt32(), r.ReadInt32()
	if name != "Talker" || level != playerLevel || class != warriorOccupation {
		t.Fatalf("roster row = %s level %d class %d, want Talker level %d class %d", name, level, class, playerLevel, warriorOccupation)
	}
}
