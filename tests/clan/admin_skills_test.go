package clan

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// clanSkillCount is how many clan skills the skill table holds: ids 370
// to 391.
const clanSkillCount = 22

// adminClanWorld is a seeded clan world, the recruit in the clan, with a
// game master of access level 7 in the world beside them, its target the
// clan's leader.
type adminClanWorld struct {
	*clanWorld
	gm   *testsupport.ScriptedClient
	gmID int32
}

// bootAdminClanWorld boots a level 5 clan with reputation, its leader
// carrying one Clan Vitality item, the shipped skill data, admin tables
// and the //clan_skill and //skill pages.
func bootAdminClanWorld(t *testing.T, reputation int, extra ...gameservertest.Option) *adminClanWorld {
	t.Helper()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	pages := map[string]string{}
	for _, name := range []string{"clan_skills.htm", "char_skills.htm"} {
		raw, err := os.ReadFile(datapack.Path(t, "data", "html", "admin", name))
		if err != nil {
			t.Fatalf("read admin page %s: %v", name, err)
		}
		pages["admin/"+name] = string(raw)
	}
	opts := append(clanSkillOptions(t),
		seedClan(t, clanSeed{level: 5, reputation: reputation}),
		gameservertest.WithAdmin(data),
		gameservertest.WithHTMLPages(pages))
	w := bootClanWorldCarrying(t, 40, 0, map[int32]int32{vitalityItem: 1}, append(opts, extra...)...)
	w.recruit(t)

	gmID := w.srv.SeedCharacterFor(t, "gm", "Admin", 10, 0).ID
	if _, err := w.srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", gmID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	gm := w.srv.DialClient(t, "gm", 1)
	startInWorld(t, gm)
	aw := &adminClanWorld{clanWorld: w, gm: gm, gmID: gmID}
	aw.selectFor(t, w.leaderID)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return aw
}

// selectFor has the game master select objectID.
func (w *adminClanWorld) selectFor(t *testing.T, objectID int32) {
	t.Helper()
	w.gm.Send(encodeAction(objectID, w.at))
	drainFrames(t, w.gm)
}

// clanSkill sends //clan_skill args from the game master and returns its
// frames and the leader's and recruit's.
func (w *adminClanWorld) clanSkill(t *testing.T, args string) (gm, leader, member [][]byte) {
	t.Helper()
	w.gm.Send(encodeBypass(strings.TrimSpace("admin_clan_skill " + args)))
	return drainFrames(t, w.gm), drainFrames(t, w.leader), drainFrames(t, w.member)
}

// gmReply decodes the game master's answer: the texts it was told, then
// the page it was shown, "" without one.
func gmReply(t *testing.T, frames [][]byte) (texts []string, page string) {
	t.Helper()
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			id, params := sysMsg(t, f)
			if id == serverpackets.SystemMessageS1 {
				texts = append(texts, params[0])
			} else {
				texts = append(texts, "sm"+itoa(int32(id))+" "+strings.Join(params, ","))
			}
		case serverpackets.OpcodeNpcHtmlMessage:
			r := wire.NewReader(f[1:])
			r.ReadInt32()
			page = r.ReadString()
		default:
			t.Fatalf("game master got opcode %#x", f[0])
		}
	}
	return texts, page
}

// clanSkillRow is the //clan_skill page row of a skill the clan knows.
func clanSkillRow(id, level int, name string) string {
	return `<tr><td><a action="bypass -h admin_clan_skill remove ` + itoa(int32(id)) + `">` + name + `</a></td><td>` + itoa(int32(level)) + `</td><td>` + itoa(int32(id)) + `</td></tr>`
}

func isSkillList(frame []byte) bool { return frame[0] == serverpackets.OpcodeSkillList }

// frameOf returns the first of frames match accepts.
func frameOf(t *testing.T, frames [][]byte, match func([]byte) bool) []byte {
	t.Helper()
	for _, f := range frames {
		if match(f) {
			return f
		}
	}
	t.Fatalf("no such frame among %x", opcodes(frames))
	return nil
}

// storedClanSkills reads the seeded clan's clan_skills rows.
func storedClanSkills(t *testing.T, w *clanWorld) map[int]int {
	t.Helper()
	w.srv.FlushPersistence(t)
	rows, err := w.srv.DB.QueryContext(context.Background(), `SELECT skill_id, skill_level FROM clan_skills WHERE clan_id = ?`, seededClanID)
	if err != nil {
		t.Fatalf("read clan skills: %v", err)
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var id, level int
		if err := rows.Scan(&id, &level); err != nil {
			t.Fatalf("scan clan skill: %v", err)
		}
		out[id] = level
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read clan skills: %v", err)
	}
	return out
}

// TestAdminClanSkillSetAndRemoveOne pins //clan_skill set <id> <level> and
// remove <id> (AdminSkill.java admin_clan_skill, Clan.addClanSkill and
// removeClanSkill): setting Clan Vitality 2 gives it to every online member
// whose rank reaches it, with its skill list, then the clan skill list's
// addition and the notice naming it; the game master is told and shown
// the page with the skill linked for removal. The row is stored. Removing
// it takes it from every member, with its skill list and the full clan
// skill list, now empty; the row is deleted.
func TestAdminClanSkillSetAndRemoveOne(t *testing.T) {
	w := bootAdminClanWorld(t, 1000)

	gm, leader, member := w.clanSkill(t, "set 370 2")
	texts, page := gmReply(t, gm)
	if !slices.Equal(texts, []string{"You gave Clan Vitality skill to Seeded clan."}) {
		t.Fatalf("game master told %q", texts)
	}
	for _, want := range []string{"<title>Seeded clan skills</title>", clanSkillRow(370, 2, "Clan Vitality"), "<td>Clan Spirituality</td><td>3</td><td>371</td>"} {
		if !strings.Contains(page, want) {
			t.Fatalf("clan skill page lacks %q:\n%s", want, page)
		}
	}
	added := []string{"SkillList", "PledgeSkillListAdd", "sm" + itoa(serverpackets.SystemMessageClanSkillS1Added)}
	for who, frames := range map[string][][]byte{"leader": leader, "recruit": member} {
		if view := clanSkillView(t, frames); !slices.Equal(view, added) {
			t.Fatalf("%s's view = %v, want %v", who, view, added)
		}
		if level := skillListLevel(t, frameOf(t, frames, isSkillList), vitalityID); level != 2 {
			t.Fatalf("%s's SkillList has Clan Vitality at %d, want 2", who, level)
		}
	}
	if got := storedClanSkills(t, w.clanWorld); len(got) != 1 || got[vitalityID] != 2 {
		t.Fatalf("stored clan skills = %v, want Clan Vitality 2", got)
	}

	gm, leader, member = w.clanSkill(t, "remove 370")
	if texts, page := gmReply(t, gm); !slices.Equal(texts, []string{"You removed 370 skillId from Seeded clan."}) || strings.Contains(page, "admin_clan_skill remove 3") {
		t.Fatalf("game master told %q, page still links a removal:\n%s", texts, page)
	}
	for who, frames := range map[string][][]byte{"leader": leader, "recruit": member} {
		if view := clanSkillView(t, frames); !slices.Equal(view, []string{"SkillList", "PledgeSkillList"}) {
			t.Fatalf("%s's view = %v, want SkillList, PledgeSkillList", who, view)
		}
		if level := skillListLevel(t, frameOf(t, frames, isSkillList), vitalityID); level != 0 {
			t.Fatalf("%s still holds Clan Vitality %d", who, level)
		}
		if got := pledgeSkills(t, frameOf(t, frames, isPledgeSkillList)); len(got) != 0 {
			t.Fatalf("%s's PledgeSkillList = %v, want empty", who, got)
		}
	}
	if got := storedClanSkills(t, w.clanWorld); len(got) != 0 {
		t.Fatalf("stored clan skills = %v, want none", got)
	}
}

// TestAdminClanSkillSetAllAndRemoveAll pins //clan_skill set all and
// remove all (Clan.addAllClanSkills and removeAllClanSkills): every clan
// skill is stored at its highest level, and each member is given those
// its rank reaches with its skill list, then the full clan skill list.
// Setting all again changes nothing and shows the page alone. Removing all
// takes every one from each member, with its skill list and the empty clan
// skill list, and deletes the rows; the next login lists no clan skill.
func TestAdminClanSkillSetAllAndRemoveAll(t *testing.T) {
	w := bootAdminClanWorld(t, 1000)

	gm, leader, member := w.clanSkill(t, "set all")
	texts, page := gmReply(t, gm)
	if !slices.Equal(texts, []string{"You gave all available skills to Seeded clan."}) || strings.Count(page, "admin_clan_skill remove 3") != 15 {
		t.Fatalf("game master told %q, page:\n%s", texts, page)
	}
	for who, frames := range map[string][][]byte{"leader": leader, "recruit": member} {
		if view := clanSkillView(t, frames); !slices.Equal(view, []string{"SkillList", "PledgeSkillList"}) {
			t.Fatalf("%s's view = %v, want SkillList, PledgeSkillList", who, view)
		}
		if level := skillListLevel(t, frameOf(t, frames, isSkillList), vitalityID); level != 3 {
			t.Fatalf("%s's SkillList has Clan Vitality at %d, want 3", who, level)
		}
		listed := pledgeSkills(t, frameOf(t, frames, isPledgeSkillList))
		if len(listed) != clanSkillCount || listed[0] != [2]int32{vitalityID, 3} {
			t.Fatalf("%s's PledgeSkillList = %v, want the %d clan skills from Clan Vitality 3", who, listed, clanSkillCount)
		}
	}
	stored := storedClanSkills(t, w.clanWorld)
	if len(stored) != clanSkillCount || stored[vitalityID] != 3 || stored[391] != 1 {
		t.Fatalf("stored clan skills = %v, want all %d at their highest level", stored, clanSkillCount)
	}

	gm, leader, member = w.clanSkill(t, "set all 2")
	if texts, page := gmReply(t, gm); len(texts) != 0 || strings.Count(page, "admin_clan_skill remove 3") != clanSkillCount-15 {
		t.Fatalf("second set all told %q, page 2:\n%s", texts, page)
	}
	if len(leader) != 0 || len(member) != 0 {
		t.Fatalf("second set all sent the members %x and %x, want nothing", opcodes(leader), opcodes(member))
	}

	gm, leader, member = w.clanSkill(t, "remove all")
	if texts, page := gmReply(t, gm); !slices.Equal(texts, []string{"You removed all skills from Seeded clan."}) || strings.Contains(page, "admin_clan_skill remove 3") {
		t.Fatalf("game master told %q, page:\n%s", texts, page)
	}
	for who, frames := range map[string][][]byte{"leader": leader, "recruit": member} {
		if view := clanSkillView(t, frames); !slices.Equal(view, []string{"SkillList", "PledgeSkillList"}) {
			t.Fatalf("%s's view = %v, want SkillList, PledgeSkillList", who, view)
		}
		if level := skillListLevel(t, frameOf(t, frames, isSkillList), vitalityID); level != 0 {
			t.Fatalf("%s still holds Clan Vitality %d", who, level)
		}
		if got := pledgeSkills(t, frameOf(t, frames, isPledgeSkillList)); len(got) != 0 {
			t.Fatalf("%s's PledgeSkillList = %v, want empty", who, got)
		}
	}
	if got := storedClanSkills(t, w.clanWorld); len(got) != 0 {
		t.Fatalf("stored clan skills = %v, want none", got)
	}

	w.leaveWorld(t, w.member)
	drainFrames(t, w.leader)
	for _, f := range startInWorld(t, w.srv.DialClient(t, "player2", 1)) {
		if isPledgeSkillList(f) {
			if got := pledgeSkills(t, f); len(got) != 0 {
				t.Fatalf("login PledgeSkillList = %v, want empty", got)
			}
		}
	}
}

// TestAdminClanSkillRefusals covers what changes nothing: a selected
// player who does not lead a clan is named in the refusal and its own
// skill page shown; an id or level out of the clan skill range answers the
// usage and the first page; a malformed set or remove answers the usage
// and the page after it; removing a skill the clan lacks shows the page
// alone, as does a page number; page 0 shows nothing.
func TestAdminClanSkillRefusals(t *testing.T) {
	w := bootAdminClanWorld(t, 1000)
	const (
		setUsage    = "Usage: //clan_skill set id level [page]"
		removeUsage = "Usage: //clan_skill remove id|all [page]"
	)
	cases := []struct {
		args  string
		texts []string
		page  string // a text the page shows, "" for no page
	}{
		{args: "set 392 1", texts: []string{setUsage}, page: "<font color=LEVEL>01</font>"},
		{args: "set 370 4 2", texts: []string{setUsage}, page: "<font color=LEVEL>01</font>"},
		{args: "set 370", texts: []string{setUsage}, page: "<font color=LEVEL>01</font>"},
		{args: "set x 2", texts: []string{setUsage}, page: "<font color=LEVEL>02</font>"},
		{args: "remove", texts: []string{removeUsage}, page: "<font color=LEVEL>01</font>"},
		{args: "remove 370 2", page: "<font color=LEVEL>02</font>"},
		{args: "2", page: "<td>Clan Imperium</td><td>1</td><td>391</td>"},
		{args: "", page: "<td>Clan Vitality</td><td>3</td><td>370</td>"},
		{args: "0"},
	}
	for _, tc := range cases {
		gm, leader, member := w.clanSkill(t, tc.args)
		texts, page := gmReply(t, gm)
		if !slices.Equal(texts, tc.texts) || (tc.page == "") != (page == "") || !strings.Contains(page, tc.page) {
			t.Fatalf("//clan_skill %s told %q, page:\n%s\nwant %q and a page showing %q", tc.args, texts, page, tc.texts, tc.page)
		}
		if len(leader) != 0 || len(member) != 0 {
			t.Fatalf("//clan_skill %s sent the members %x and %x", tc.args, opcodes(leader), opcodes(member))
		}
	}

	w.selectFor(t, w.memberID)
	drainFrames(t, w.member)
	gm, _, member := w.clanSkill(t, "set all")
	if len(gm) != 2 || len(member) != 0 {
		t.Fatalf("//clan_skill on the recruit = %x, recruit got %x", opcodes(gm), opcodes(member))
	}
	if id, params := sysMsg(t, gm[0]); id != serverpackets.SystemMessageS1IsNotAClanLeader || !slices.Equal(params, []string{"Recruit"}) {
		t.Fatalf("refusal = %d %v, want S1_IS_NOT_A_CLAN_LEADER naming Recruit", id, params)
	}
	if _, page := gmReply(t, gm[1:]); !strings.Contains(page, "admin_skill") {
		t.Fatalf("refusal page is not the player's skill page:\n%s", page)
	}
	if got := storedClanSkills(t, w.clanWorld); len(got) != 0 {
		t.Fatalf("stored clan skills = %v, want none", got)
	}
}

// TestAdminClanSkillWaitsForLeaderQueue holds the leader's queue, then
// has the game master set Clan Vitality 1: the change waits on the
// leader's queue, so the game master gets nothing while it is held, and
// the leader's learn of that very skill, sent meanwhile, runs after it,
// never between its payment and its check under the clan's lock. Once
// released, the change teaches the skill to the members and the learn is
// refused before anything is paid: the leader keeps its item and the clan
// its reputation.
func TestAdminClanSkillWaitsForLeaderQueue(t *testing.T) {
	w := bootAdminClanWorld(t, 1000, gameservertest.WithRealPool())
	w.talkToMaster(t)
	drainFrames(t, w.gm)
	drainFrames(t, w.member)

	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	letGo := func() { once.Do(func() { close(release) }) }
	t.Cleanup(letGo)
	if !w.srv.PlayerQueue(t, w.leaderID).Post(func() { close(held); <-release }) {
		t.Fatal("post to the leader's queue: queue closed")
	}
	<-held
	w.gm.Send(encodeBypass("admin_clan_skill set 370 1"))
	w.srv.AwaitHandled(t)
	ran := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.gmID).Post(func() { close(ran) }) {
		t.Fatal("post to the game master's queue: queue closed")
	}
	<-ran
	if frames := drainFrames(t, w.gm); len(frames) != 0 {
		t.Fatalf("game master answered %x while the leader is held, want nothing", opcodes(frames))
	}
	w.leader.Send(encodeRequestAcquireSkill(vitalityID, 1, acquireSkillReq))

	letGo()
	w.srv.AwaitHandled(t)
	added := []string{"SkillList", "PledgeSkillListAdd", "sm" + itoa(serverpackets.SystemMessageClanSkillS1Added)}
	if view := clanSkillView(t, drainFrames(t, w.leader)); !slices.Equal(view, added) {
		t.Fatalf("leader's view = %v, want the change alone %v", view, added)
	}
	if texts, page := gmReply(t, drainFrames(t, w.gm)); !slices.Equal(texts, []string{"You gave Clan Vitality skill to Seeded clan."}) || !strings.Contains(page, clanSkillRow(370, 1, "Clan Vitality")) {
		t.Fatalf("game master told %q, page:\n%s", texts, page)
	}
	if n := w.srv.PlayerInventory(t, w.leaderID).ItemCount(vitalityItem, -1, false); n != 1 {
		t.Fatalf("leader carries %d of the item, want it kept", n)
	}
	if _, rep := storedClan(t, w.clanWorld); rep != 1000 {
		t.Fatalf("stored reputation = %d, want 1000", rep)
	}
	if got := storedClanSkills(t, w.clanWorld); len(got) != 1 || got[vitalityID] != 1 {
		t.Fatalf("stored clan skills = %v, want Clan Vitality 1", got)
	}
}
