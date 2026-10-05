package clan

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/classmaster"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// The graduation scenario plays an academy member, a level 40 Warrior that
// joined the academy at level 20, taking its second occupation, Gladiator,
// at the class manager.
const (
	classManagerID   = 50000
	warriorClassID   = 1
	gladiatorClassID = 2
	graduateID       = 0x7f100002
	graduateJoinedAt = 20
	// graduationPoints is what joining at level 20 earns: 400 - 4 * 10.
	graduationPoints = 360
	academyCircletID = 8181
)

// Graduation system messages.
const (
	smYouPickedUpS1            = 30
	smClassTransfer            = 1308
	smGraduatedFromAcademy     = 1748
	smAcademyMembershipEnded   = 1749
	smYouHaveWithdrawnFromClan = 197
	smS1HasWithdrawnFromClan   = 223
)

// graduationPages are the shipped class manager pages.
func graduationPages(t *testing.T) map[string]string {
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

// occupationTemplate is the fixture class template as class id.
func occupationTemplate(id int) *player.Template {
	tmpl := gameservertest.ClassTemplate()
	tmpl.ID = id
	tmpl.Skills = nil
	return tmpl
}

// graduationItems are the fixture items plus the Academy Circlet.
func graduationItems() *item.Table {
	templates := append([]*item.Template(nil), gameservertest.ItemTemplates().All()...)
	templates = append(templates, &item.Template{
		ID: academyCircletID, Name: "Academy Circlet", Kind: item.KindArmor, Slot: item.SlotFace,
		Duration: -1, Weight: 10, Destroyable: true, DefaultAction: item.ActionEquip,
		Armor: &item.ArmorDetail{Type: item.ArmorNone},
	})
	return item.NewTable(templates)
}

// seedGraduate stores the academy member, account player2, a level
// level character of class classID.
func seedGraduate(t *testing.T, classID, level int) gameservertest.Option {
	return gameservertest.WithSeed(func(chars *gamesql.CharacterStore, _ *gamesql.ItemStore) {
		ch, err := player.NewCharacter(graduateID, occupationTemplate(classID), "player2", "Graduate", 0, 0, 0, player.SexMale)
		if err != nil {
			t.Fatalf("new graduate: %v", err)
		}
		ch.CharLevel = level
		ctx := context.Background()
		if err := chars.Create(ctx, ch); err != nil {
			t.Fatalf("store graduate: %v", err)
		}
	})
}

// graduationWorld is the clan's founder and its academy member in the
// world, the member beside the class manager, its first page open.
type graduationWorld struct {
	srv      *gameservertest.Server
	leader   *testsupport.ScriptedClient
	graduate *testsupport.ScriptedClient
	manager  *npc.Folk
}

// bootGraduation boots the seeded clan at clanLevel with its academy
// member, a level level character of class classID that joined the
// academy at level 20, both in the world; extra statements run on the
// seeded clan last.
func bootGraduation(t *testing.T, classID, level, clanLevel int, extra ...string) *graduationWorld {
	t.Helper()
	jobs, err := classmaster.ParseJobs("1;[];[];2;[];[]")
	if err != nil {
		t.Fatal(err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(graduationPages(t)),
		gameservertest.WithClassMaster(classmaster.NewConfig(false, jobs)),
		gameservertest.WithClassTemplates(occupationTemplate(warriorClassID), occupationTemplate(gladiatorClassID)),
		gameservertest.WithItemTemplates(graduationItems()),
		seedGraduate(t, classID, level),
		seedSubunitClan(t, append([]string{
			academySeed,
			`UPDATE clan_data SET clan_level = ` + strconv.Itoa(clanLevel) + ` WHERE clan_id = ` + itoa(subunitClanID),
			`UPDATE characters SET clanid = ` + itoa(subunitClanID) + `, subpledge = -1, power_grade = 9, lvl_joined_academy = ` +
				strconv.Itoa(graduateJoinedAt) + ` WHERE obj_Id = ` + itoa(graduateID),
		}, extra...)...),
	)
	w := &graduationWorld{srv: srv, leader: srv.Client, graduate: srv.DialClient(t, "player2", 1)}
	startInWorld(t, w.leader)
	startInWorld(t, w.graduate)
	drainFrames(t, w.leader)
	x, y, z := srv.PlayerPosition(t, graduateID)
	w.manager = srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("ClassMaster", classManagerID), location.Location{X: x + 30, Y: y, Z: z})
	drainFrames(t, w.leader)
	drainFrames(t, w.graduate)
	at := location.Location{X: x, Y: y, Z: z}
	for range 2 {
		w.graduate.Send(encodeAction(w.manager.ObjectID(), at))
		drainFrames(t, w.graduate)
	}
	return w
}

// change has the academy member open the menu of the occupation tier and
// change to classID. It returns the member's and the founder's frames.
func (w *graduationWorld) change(t *testing.T, menu string, classID int) (member, leader [][]byte) {
	t.Helper()
	command := func(cmd string) [][]byte {
		w.graduate.Send(encodeBypass("npc_" + strconv.Itoa(int(w.manager.ObjectID())) + "_" + cmd))
		return drainFrames(t, w.graduate)
	}
	command(menu)
	member = command("change_class " + strconv.Itoa(classID))
	return member, drainFrames(t, w.leader)
}

// TestAcademyGraduationOnSecondOccupation has an academy member that
// joined at level 20 take its second occupation at the class manager. The
// clan earns 360 reputation; every member hears of the graduation, the
// graduate is told its membership ended and that it withdrew, the rest
// see its row deleted and its withdrawal; the graduate leaves the clan
// with no join penalty and holds the Academy Circlet, and only then is
// the occupation change shown.
func TestAcademyGraduationOnSecondOccupation(t *testing.T) {
	w := bootGraduation(t, warriorClassID, 40, 7)
	srv := w.srv
	frames, leaderFrames := w.change(t, "2ndClass", gladiatorClassID)

	// The graduate: the clan header with the new score, the three notices,
	// its clan state taken (skill list, UserInfo, roster cleared), the
	// circlet picked up, then the occupation change.
	kept := only(frames, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSkillList,
		serverpackets.OpcodePledgeShowMemberListDelAll, serverpackets.OpcodeMagicSkillUse)
	want := []byte{
		serverpackets.OpcodePledgeShowInfoUpdate,
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSkillList, serverpackets.OpcodePledgeShowMemberListDelAll,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage,
	}
	if len(kept) < len(want) || !slices.Equal(kept[:len(want)], want) {
		t.Fatalf("graduate's answer = %x, want %x first", kept, want)
	}
	var notices [][]string
	var ids []int
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			id, params := sysMsg(t, f)
			ids = append(ids, id)
			notices = append(notices, params)
		}
	}
	if wantIDs := []int{smGraduatedFromAcademy, smAcademyMembershipEnded, smYouHaveWithdrawnFromClan, smYouPickedUpS1, smClassTransfer}; !slices.Equal(ids[:min(len(ids), len(wantIDs))], wantIDs) {
		t.Fatalf("graduate's messages = %v, want %v first", ids, wantIDs)
	}
	if !slices.Equal(notices[0], []string{"Graduate", strconv.Itoa(graduationPoints)}) {
		t.Fatalf("graduation notice = %v, want Graduate and %d", notices[0], graduationPoints)
	}
	if !slices.Equal(notices[3], []string{strconv.Itoa(academyCircletID)}) {
		t.Fatalf("pickup notice = %v, want the circlet", notices[3])
	}

	// The leader: the header, the graduation notice, the row deleted, then
	// the withdrawal.
	if got := only(leaderFrames, serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListDelete); !slices.Equal(got, []byte{
		serverpackets.OpcodePledgeShowInfoUpdate, serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodePledgeShowMemberListDelete, serverpackets.OpcodeSystemMessage,
	}) {
		t.Fatalf("leader's frames = %x", got)
	}
	var leaderNotices [][]string
	for _, f := range leaderFrames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			id, params := sysMsg(t, f)
			leaderNotices = append(leaderNotices, append([]string{strconv.Itoa(id)}, params...))
		}
	}
	if !slices.EqualFunc(leaderNotices, [][]string{
		{strconv.Itoa(smGraduatedFromAcademy), "Graduate", strconv.Itoa(graduationPoints)},
		{strconv.Itoa(smS1HasWithdrawnFromClan), "Graduate"},
	}, slices.Equal) {
		t.Fatalf("leader's notices = %v", leaderNotices)
	}

	g := onlineCharacter(t, srv, graduateID)
	if g.ClanID() != 0 || g.ClanJoinExpiryTime() != 0 || g.ClassID() != gladiatorClassID {
		t.Fatalf("graduate clan %d join expiry %d class %d, want no clan, no penalty, Gladiator", g.ClanID(), g.ClanJoinExpiryTime(), g.ClassID())
	}
	if n := g.Inventory().ItemCount(academyCircletID, -1, true); n != 1 {
		t.Fatalf("circlets held = %d, want 1", n)
	}

	srv.FlushItems(t)
	cw := &clanWorld{srv: srv}
	if got := queryInt(t, cw, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, subunitClanID); got != 20000+graduationPoints {
		t.Fatalf("stored reputation = %d, want %d", got, 20000+graduationPoints)
	}
	for column, want := range map[string]int64{"clanid": 0, "subpledge": 0, "lvl_joined_academy": 0, "clan_join_expiry_time": 0} {
		if got := queryInt(t, cw, `SELECT `+column+` FROM characters WHERE obj_Id = ?`, graduateID); got != want {
			t.Fatalf("stored %s = %d, want %d", column, got, want)
		}
	}
	if got := queryInt(t, cw, `SELECT COUNT(*) FROM items WHERE owner_id = ? AND item_id = ?`, graduateID, academyCircletID); got != 1 {
		t.Fatalf("stored circlets = %d, want 1", got)
	}
}

// TestAcademyGraduationInLowLevelClan graduates an academy member from a
// level 4 clan, which earns no reputation: no header goes out and the
// score stays, the graduation notice still naming the points.
func TestAcademyGraduationInLowLevelClan(t *testing.T) {
	w := bootGraduation(t, warriorClassID, 40, 4)
	frames, leaderFrames := w.change(t, "2ndClass", gladiatorClassID)
	if got := only(append(frames, leaderFrames...), serverpackets.OpcodePledgeShowInfoUpdate); len(got) != 0 {
		t.Fatalf("headers = %d, want none", len(got))
	}
	if got := messages(t, leaderFrames); !slices.Equal(got, []int{smGraduatedFromAcademy, smS1HasWithdrawnFromClan}) {
		t.Fatalf("leader's messages = %v", got)
	}
	if g := onlineCharacter(t, w.srv, graduateID); g.ClanID() != 0 || g.Inventory().ItemCount(academyCircletID, -1, true) != 1 {
		t.Fatalf("graduate clan %d, circlets %d, want no clan and one circlet", g.ClanID(), g.Inventory().ItemCount(academyCircletID, -1, true))
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, &clanWorld{srv: w.srv}, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, subunitClanID); got != 20000 {
		t.Fatalf("stored reputation = %d, want 20000", got)
	}
}

// TestAcademyMemberStaysOnFirstOccupation keeps an academy member taking
// its first occupation in the academy: no graduation notice, no circlet.
func TestAcademyMemberStaysOnFirstOccupation(t *testing.T) {
	w := bootGraduation(t, 0, 20, 7)
	frames, leaderFrames := w.change(t, "1stClass", warriorClassID)
	if got := messages(t, frames); !slices.Equal(got, []int{smClassTransfer}) {
		t.Fatalf("member's messages = %v, want only CLASS_TRANSFER", got)
	}
	if got := only(leaderFrames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePledgeShowMemberListDelete); len(got) != 0 {
		t.Fatalf("leader's frames = %x, want no notice nor row deleted", got)
	}
	g := onlineCharacter(t, w.srv, graduateID)
	if g.ClanID() != subunitClanID || g.ClassID() != warriorClassID || g.Inventory().ItemCount(academyCircletID, -1, true) != 0 {
		t.Fatalf("member clan %d class %d circlets %d, want still in the clan, Warrior, no circlet", g.ClanID(), g.ClassID(), g.Inventory().ItemCount(academyCircletID, -1, true))
	}
}

// TestAcademyGraduationLiftsReputationAboveZero graduates an academy
// member from a clan at -100 reputation: the score rising above 0, every
// member, the graduate included as it was still one, first learns the clan
// skills are back on.
func TestAcademyGraduationLiftsReputationAboveZero(t *testing.T) {
	w := bootGraduation(t, warriorClassID, 40, 7, `UPDATE clan_data SET reputation_score = -100 WHERE clan_id = `+itoa(subunitClanID))
	frames, leaderFrames := w.change(t, "2ndClass", gladiatorClassID)
	if got := messages(t, frames); len(got) < 2 || !slices.Equal(got[:2], []int{serverpackets.SystemMessageClanSkillsActivatedReputation, smGraduatedFromAcademy}) {
		t.Fatalf("graduate's messages = %v, want the skills back on, then the graduation", got)
	}
	if got := messages(t, leaderFrames); !slices.Equal(got, []int{serverpackets.SystemMessageClanSkillsActivatedReputation, smGraduatedFromAcademy, smS1HasWithdrawnFromClan}) {
		t.Fatalf("leader's messages = %v", got)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, &clanWorld{srv: w.srv}, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, subunitClanID); got != graduationPoints-100 {
		t.Fatalf("stored reputation = %d, want %d", got, graduationPoints-100)
	}
}
