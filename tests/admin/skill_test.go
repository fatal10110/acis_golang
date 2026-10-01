package admin

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The skills the //skill fixture's class grants.
const (
	boughtSkill = 1001 // level 1 at 5 and level 2 at 10, bought with SP
	freeSkill   = 1002 // level 1 at 1, given for free
	laterSkill  = 1003 // level 1 at 30, out of reach
)

// skillClassTemplate is the fixture class with the grants above.
func skillClassTemplate() *player.Template {
	tmpl := gameservertest.ClassTemplate()
	tmpl.Skills = []player.SkillGrant{
		{SkillID: boughtSkill, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: boughtSkill, Level: 2, MinLevel: 10, Cost: 100},
		{SkillID: freeSkill, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: laterSkill, Level: 1, MinLevel: 30, Cost: 10},
	}
	return tmpl
}

// bootSkillAdmin boots the GM with the //skill fixture: the class above,
// its skills' definitions stored through the real character_skills
// tables, and the skill list page.
func bootSkillAdmin(t *testing.T) (*gameservertest.Server, int32) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{ID: boughtSkill, Level: 1, Name: "Bought"},
		{ID: boughtSkill, Level: 2, Name: "Bought"},
		{ID: freeSkill, Level: 1, Name: "Free"},
		{ID: laterSkill, Level: 1, Name: "Later"},
	}), gamesql.NewCharacterSkillStore(db))
	return bootAdmin(t, adminLevel,
		gameservertest.WithSkills(skills),
		gameservertest.WithClassTemplate(skillClassTemplate()),
		gameservertest.WithHTMLPages(shippedAdminPages(t, "char_skills.htm")),
	)
}

// storedSkills returns objID's character_skills rows, skill id to level.
func storedSkills(t *testing.T, srv *gameservertest.Server, objID int32) map[int]int {
	t.Helper()
	srv.FlushPersistence(t)
	rows, err := srv.DB.QueryContext(context.Background(), "SELECT skill_id, skill_level FROM character_skills WHERE char_obj_id = ? AND class_index = 0", objID)
	if err != nil {
		t.Fatalf("read skills: %v", err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id, level int
		if err := rows.Scan(&id, &level); err != nil {
			t.Fatalf("scan skill: %v", err)
		}
		out[id] = level
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read skills: %v", err)
	}
	return out
}

// skillRow is the //skill page row of one skill.
func skillRow(id int, name string, level int) string {
	return `<td width=35>` + strconv.Itoa(id) + `</td><td width=220><a action="bypass -h admin_skill remove ` + strconv.Itoa(id) + `">` + name + `</a></td><td width=25>` + strconv.Itoa(level) + `</td>`
}

// TestAdminSkillSetAll pins //skill set all (AdminSkill.java admin_skill
// "set all", Player.rewardSkills): the selected player is granted every
// skill its level makes available — the bought ones stored, the free one
// kept in memory, the one above its level left out — a shortcut to a skill
// that went up follows its new level ahead of the new SkillList, and the GM
// is told, then shown the player's skill page.
func TestAdminSkillSetAll(t *testing.T) {
	t.Parallel()
	srv, _ := bootSkillAdmin(t)
	gm := srv.Client
	enterWorld(t, gm)
	learner := srv.SeedCharacterFor(t, "player2", "Learner", 20, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index) VALUES (?, ?, 1, 0)", learner.ID, boughtSkill); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	if _, err := srv.DB.ExecContext(context.Background(), "INSERT INTO character_shortcuts (char_obj_id, slot, page, type, id, level, class_index) VALUES (?, 3, 0, 'SKILL', ?, 1, 0)", learner.ID, boughtSkill); err != nil {
		t.Fatalf("seed shortcut: %v", err)
	}
	learnerClient := srv.DialClient(t, "player2", 1)
	enterWorld(t, learnerClient)
	drain(t, gm)

	exchange(t, gm, encodeAction(learner.ID))
	frames := exchange(t, gm, encodeBuildCmd("skill set all"))
	if len(frames) != 2 {
		t.Fatalf("//skill set all frames = %x, want the report and the skill page", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "You gave all available skills to Learner.")
	page := htmlBody(t, frames[1])
	for _, want := range []string{
		"Learner",
		skillRow(boughtSkill, "Bought", 2),
		skillRow(freeSkill, "Free", 1),
		`<button action="bypass admin_skill 1"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("skill page = %q, want it to contain %q", page, want)
		}
	}
	if strings.Contains(page, "Later") || strings.Contains(page, "%content%") {
		t.Fatalf("skill page = %q, want neither the unreachable skill nor the placeholder", page)
	}

	got := settle(t, learnerClient)
	register, list := -1, -1
	for i, f := range got {
		switch f[0] {
		case serverpackets.OpcodeShortCutRegister:
			r := wire.NewReader(f[1:])
			if typ, slot, id, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); typ != int32(serverpackets.ShortcutSkill) || slot != 3 || id != boughtSkill || level != 2 {
				t.Fatalf("ShortCutRegister = type %d slot %d id %d level %d, want skill %d level 2 at slot 3", typ, slot, id, level, boughtSkill)
			}
			register = i
		case serverpackets.OpcodeSkillList:
			list = i
		}
	}
	if register < 0 || list < register {
		t.Fatalf("learner frames %x (ShortCutRegister at %d, SkillList at %d), want the shortcut refreshed then the skill list", testsupport.FrameOpcodes(got), register, list)
	}
	stored := storedSkills(t, srv, learner.ID)
	if len(stored) != 1 || stored[boughtSkill] != 2 {
		t.Fatalf("stored skills = %v, want only %d at level 2", stored, boughtSkill)
	}
}

// TestAdminSkillPage pins //skill's other ported forms: no argument or a
// page number opens the GM's own skill page, a page below 1 opens nothing,
// "set" alone answers the usage then the page, and the sub-commands still
// to port release the client.
func TestAdminSkillPage(t *testing.T) {
	t.Parallel()
	srv, _ := bootSkillAdmin(t)
	gm := srv.Client
	enterWorld(t, gm)
	drain(t, gm)

	assertPage(t, exchange(t, gm, encodeBuildCmd("skill")), skillRow(freeSkill, "Free", 1))
	assertPage(t, exchange(t, gm, encodeBypass("admin_skill 2")), skillRow(freeSkill, "Free", 1))
	if frames := exchange(t, gm, encodeBuildCmd("skill 0")); len(frames) != 0 {
		t.Fatalf("//skill 0 frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	frames := exchange(t, gm, encodeBuildCmd("skill set"))
	if len(frames) != 2 {
		t.Fatalf("//skill set frames = %x, want the usage and the page", testsupport.FrameOpcodes(frames))
	}
	assertTexts(t, frames[:1], "Usage: //skill set id level [page]")
	assertPage(t, frames[1:], skillRow(freeSkill, "Free", 1))
	for _, command := range []string{"skill list", "skill set 1001 1", "skill remove all"} {
		frames := exchange(t, gm, encodeBuildCmd(command))
		if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
			t.Fatalf("//%s frames = %x, want ActionFailed", command, testsupport.FrameOpcodes(frames))
		}
	}
}
