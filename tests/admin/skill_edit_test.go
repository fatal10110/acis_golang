package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// passiveSkill is a passive skill outside the fixture class's grants.
const passiveSkill = 1004

// bootSkillEditAdmin boots the //skill fixture of bootSkillAdmin with a
// passive skill added to its table, and a second character, Learner, at
// level 20 knowing boughtSkill at level 1 and passiveSkill, stored, with
// them on shortcut slots 3 and 4. It returns the server and Learner's
// client and object id, both in the world, the GM's selection on Learner.
func bootSkillEditAdmin(t *testing.T) (*gameservertest.Server, *testsupport.ScriptedClient, int32) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: boughtSkill, Level: 1, Name: "Bought", Activation: modelskill.ActivationActive},
		{ID: boughtSkill, Level: 2, Name: "Bought", Activation: modelskill.ActivationActive},
		{ID: freeSkill, Level: 1, Name: "Free", Activation: modelskill.ActivationActive},
		{ID: laterSkill, Level: 1, Name: "Later", Activation: modelskill.ActivationActive},
		{ID: passiveSkill, Level: 1, Name: "Passive", Activation: modelskill.ActivationPassive},
	}), gamesql.NewCharacterSkillStore(db))
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithSkills(skills),
		gameservertest.WithClassTemplate(skillClassTemplate()),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "char_skills.htm")),
	)
	enterWorld(t, srv.Client)
	learner := srv.SeedCharacterFor(t, "player2", "Learner", 20, 0)
	for _, q := range []struct {
		query string
		args  []any
	}{
		{"INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES (?, ?, 1, 0), (?, ?, 1, 0)", []any{learner.ID, boughtSkill, learner.ID, passiveSkill}},
		{"INSERT INTO character_shortcuts (char_obj_id, slot, page, type, id, level, class_index) VALUES (?, 3, 0, 'SKILL', ?, 1, 0), (?, 4, 0, 'SKILL', ?, 1, 0)", []any{learner.ID, boughtSkill, learner.ID, passiveSkill}},
	} {
		if _, err := srv.DB.ExecContext(context.Background(), q.query, q.args...); err != nil {
			t.Fatalf("seed learner: %v", err)
		}
	}
	learnerClient := srv.DialClient(t, "player2", 1)
	enterWorld(t, learnerClient)
	drain(t, srv.Client)
	exchange(t, srv.Client, encodeAction(learner.ID))
	return srv, learnerClient, learner.ID
}

// storedShortcuts returns objID's character_shortcuts rows, slot to skill
// level.
func storedShortcuts(t *testing.T, srv *gameservertest.Server, objID int32) map[int]int {
	t.Helper()
	srv.FlushPersistence(t)
	rows, err := srv.DB.QueryContext(context.Background(), "SELECT slot, level FROM character_shortcuts WHERE char_obj_id = ? AND class_index = 0 AND type = 'SKILL'", objID)
	if err != nil {
		t.Fatalf("read shortcuts: %v", err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var slot, level int
		if err := rows.Scan(&slot, &level); err != nil {
			t.Fatalf("scan shortcut: %v", err)
		}
		out[slot] = level
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read shortcuts: %v", err)
	}
	return out
}

// assertReportAndPage requires frames to be the text report then the
// skill page holding marker.
func assertReportAndPage(t *testing.T, command string, frames [][]byte, report, marker string) {
	t.Helper()
	if len(frames) != 2 {
		t.Fatalf("//%s frames = %x, want the report and the skill page", command, testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], report)
	assertPage(t, frames[1:], marker)
}

// shortcutFrames returns, in order, the opcodes of frames that are a
// ShortCutRegister, ShortCutDelete or SkillList, a register as "R<slot>:
// <level>", a delete as "D<slot>" and the list as "L".
func shortcutFrames(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeShortCutRegister:
			r := wire.NewReader(f[1:])
			typ, slot, id, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
			if typ != int32(serverpackets.ShortcutSkill) {
				t.Fatalf("ShortCutRegister type %d id %d, want a skill", typ, id)
			}
			out = append(out, fmt.Sprintf("R%d:%d", slot, level))
		case serverpackets.OpcodeShortCutDelete:
			out = append(out, fmt.Sprintf("D%d", wire.NewReader(f[1:]).ReadInt32()))
		case serverpackets.OpcodeSkillList:
			out = append(out, "L")
		}
	}
	return out
}

func assertShortcutFrames(t *testing.T, step string, frames [][]byte, want ...string) {
	t.Helper()
	if got := shortcutFrames(t, frames); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("%s: learner frames %x read as %q, want %q", step, testsupport.FrameOpcodes(frames), got, want)
	}
}

// TestAdminSkillSetOne pins //skill set <id> <level> (AdminSkill.java
// admin_skill "set", Player.addSkill(skill, true, true)): the selected
// player learns that level, stored; a changed level, up or down, re-points
// the skill's shortcuts ahead of the new SkillList, the same level sends
// the list alone; the GM is told then shown the player's page. A level the
// skill table has no entry for answers the usage alone; a malformed id or
// level answers the usage then the page.
func TestAdminSkillSetOne(t *testing.T) {
	t.Parallel()
	srv, learnerClient, learnerID := bootSkillEditAdmin(t)
	gm := srv.Client

	assertReportAndPage(t, "skill set 1001 2", exchange(t, gm, encodeBuildCmd("skill set 1001 2")),
		"You gave Bought skill to Learner.", skillRow(boughtSkill, "Bought", 2))
	assertShortcutFrames(t, "raise", settle(t, learnerClient), "R3:2", "L")
	if stored := storedSkills(t, srv, learnerID); stored[boughtSkill] != 2 {
		t.Fatalf("stored skills = %v, want %d at level 2", stored, boughtSkill)
	}
	if sc := storedShortcuts(t, srv, learnerID); sc[3] != 2 {
		t.Fatalf("stored shortcuts = %v, want slot 3 at level 2", sc)
	}

	// The table's combined key is id*256+level: 1000/257 is 1001/1.
	assertReportAndPage(t, "skill set 1000 257", exchange(t, gm, encodeBuildCmd("skill set 1000 257 1")),
		"You gave Bought skill to Learner.", skillRow(boughtSkill, "Bought", 1))
	assertShortcutFrames(t, "lower", settle(t, learnerClient), "R3:1", "L")
	if stored := storedSkills(t, srv, learnerID); stored[boughtSkill] != 1 {
		t.Fatalf("stored skills = %v, want %d at level 1", stored, boughtSkill)
	}

	assertReportAndPage(t, "skill set 1001 1", exchange(t, gm, encodeBuildCmd("skill set 1001 1")),
		"You gave Bought skill to Learner.", skillRow(boughtSkill, "Bought", 1))
	assertShortcutFrames(t, "same level", settle(t, learnerClient), "L")

	assertTexts(t, exchange(t, gm, encodeBuildCmd("skill set 1001 9")), "Usage: //skill set id level [page]")
	for _, command := range []string{"skill set abc 1", "skill set 1001", "skill set 1001 x"} {
		assertReportAndPage(t, command, exchange(t, gm, encodeBuildCmd(command)),
			"Usage: //skill set id level [page]", skillRow(boughtSkill, "Bought", 1))
	}
	assertShortcutFrames(t, "rejected", settle(t, learnerClient))
}

// TestAdminSkillRemove pins //skill remove (AdminSkill.java admin_skill
// "remove", Player.removeSkill(id, true)): one skill goes with its row and
// the shortcuts bound to it, the GM is told, and no SkillList follows;
// "all" takes every skill away — a passive skill's shortcut stays — and
// the player gets its skill list after the GM's report. A missing or
// malformed id answers the usage then the page.
func TestAdminSkillRemove(t *testing.T) {
	t.Parallel()
	srv, learnerClient, learnerID := bootSkillEditAdmin(t)
	gm := srv.Client

	assertReportAndPage(t, "skill remove 1001", exchange(t, gm, encodeBuildCmd("skill remove 1001")),
		"You removed 1001 skillId from Learner.", skillRow(freeSkill, "Free", 1))
	assertShortcutFrames(t, "remove one", settle(t, learnerClient), "D3")
	if stored := storedSkills(t, srv, learnerID); len(stored) != 1 || stored[passiveSkill] != 1 {
		t.Fatalf("stored skills = %v, want only %d", stored, passiveSkill)
	}
	if sc := storedShortcuts(t, srv, learnerID); len(sc) != 1 || sc[4] != 1 {
		t.Fatalf("stored shortcuts = %v, want only slot 4", sc)
	}

	for _, command := range []string{"skill remove", "skill remove abc"} {
		assertReportAndPage(t, command, exchange(t, gm, encodeBuildCmd(command)),
			"Usage: //skill remove id [page]", skillRow(freeSkill, "Free", 1))
	}

	frames := exchange(t, gm, encodeBuildCmd("skill remove all"))
	if len(frames) != 2 {
		t.Fatalf("//skill remove all frames = %x, want the report and the skill page", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "You removed all skills from Learner.")
	if page := htmlBody(t, frames[1]); strings.Contains(page, "admin_skill remove 1") {
		t.Fatalf("skill page = %q, want no skill left", page)
	}
	got := settle(t, learnerClient)
	assertShortcutFrames(t, "remove all", got, "L")
	for _, f := range got {
		if f[0] == serverpackets.OpcodeSkillList {
			if n := wire.NewReader(f[1:]).ReadInt32(); n != 0 {
				t.Fatalf("SkillList holds %d skills, want none", n)
			}
		}
	}
	if stored := storedSkills(t, srv, learnerID); len(stored) != 0 {
		t.Fatalf("stored skills = %v, want none", stored)
	}
	if sc := storedShortcuts(t, srv, learnerID); len(sc) != 1 || sc[4] != 1 {
		t.Fatalf("stored shortcuts = %v, want the passive skill's slot 4 kept", sc)
	}
}

// listRow is the //skill list page row of one skill.
func listRow(id int, name string, level int) string {
	return `<td width=35>` + strconv.Itoa(id) + `</td><td width=220>` + name + `</td><td width=25>` + strconv.Itoa(level) + `</td>`
}

// TestAdminSkillList pins //skill list [page] (AdminSkill.java
// showSkillList): every skill of the table at its highest level below 99,
// by id, 12 to a page, under the selected player's name with the
// "admin_skill list" page bar, and nothing else.
func TestAdminSkillList(t *testing.T) {
	t.Parallel()
	defs := make([]modelskill.Definition, 0, 16)
	for id := 1; id <= 14; id++ {
		defs = append(defs, modelskill.Definition{ID: modelskill.ID(id), Level: 1, Name: "Skill" + strconv.Itoa(id)})
	}
	// Skill 13's top regular level is 2; its enchant level is left out.
	defs = append(defs,
		modelskill.Definition{ID: 13, Level: 2, Name: "Skill13"},
		modelskill.Definition{ID: 13, Level: 101, Name: "Skill13"},
	)
	db := sqltest.SharedDB(t)
	srv, _ := bootAdmin(t, adminLevel,
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable(defs), gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "char_skills.htm", "char_skills_list.htm")),
	)
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("skill list"))
	if len(frames) != 1 {
		t.Fatalf("//skill list frames = %x, want the list page alone", testsupport.FrameOpcodes(frames))
	}
	page := htmlBody(t, frames[0])
	for _, want := range []string{"Targetting Admin", listRow(1, "Skill1", 1), listRow(12, "Skill12", 1), `<button action="bypass admin_skill list 1"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("list page 1 = %q, want it to contain %q", page, want)
		}
	}
	if strings.Contains(page, "Skill13") {
		t.Fatalf("list page 1 = %q, want skill 13 on page 2", page)
	}

	frames = exchange(t, gm, encodeBypass("admin_skill list 2"))
	if len(frames) != 1 {
		t.Fatalf("//skill list 2 frames = %x, want the list page alone", testsupport.FrameOpcodes(frames))
	}
	page = htmlBody(t, frames[0])
	for _, want := range []string{listRow(13, "Skill13", 2), listRow(14, "Skill14", 1)} {
		if !strings.Contains(page, want) {
			t.Fatalf("list page 2 = %q, want it to contain %q", page, want)
		}
	}
	if strings.Contains(page, "Skill1<") || strings.Contains(page, ">101<") {
		t.Fatalf("list page 2 = %q, want neither page 1's skills nor the enchant level", page)
	}
	if frames := exchange(t, gm, encodeBuildCmd("skill list 0")); len(frames) != 0 {
		t.Fatalf("//skill list 0 frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
}
