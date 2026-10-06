package quest

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// renderTail renders frames as the goldens write packets, leaving out
// those the goldens do not render (selection, approach, animation).
func (w *dialogWorld) renderTail(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		if !slices.Contains(traceRenders, f[0]) {
			continue
		}
		line, err := scriptcontract.Packet(f, w.roles)
		if err != nil {
			t.Fatalf("frame %x: %v", f, err)
		}
		out = append(out, line)
	}
	return out
}

// TestFirstTalkAnswersInsteadOfTheChatWindow: a civilian NPC whose first
// talk one script holds answers an interact with that script's page, its
// object id filled in, then ActionFailed, and no chat window; an empty
// answer only releases the client. Either way the NPC becomes the last
// quest NPC.
func TestFirstTalkAnswersInsteadOfTheChatWindow(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, map[string]string{"default/30981.htm": "<html><body>chat</body></html>"}, nil)
	judge := w.spawnFolk(t, "judge", judgeID, 20, 0, 0)
	silent := w.spawnFolk(t, "silent", silentID, 0, 20, 0)

	got := w.renderTail(t, w.interact(t, judge.ObjectID()))
	want := []string{
		"S ActionFailed",
		`S NpcHtmlMessage obj=judge item=0 html="<html><body>Judge {judge}</body></html>"`,
		"S ActionFailed",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("judge interact:\n got %q\nwant %q", got, want)
	}
	if got := w.lastRole(t); got != "judge" {
		t.Fatalf("last quest NPC = %s, want judge", got)
	}

	got = w.renderTail(t, w.interact(t, silent.ObjectID()))
	if want := []string{"S ActionFailed", "S ActionFailed"}; !slices.Equal(got, want) {
		t.Fatalf("silent interact:\n got %q\nwant %q", got, want)
	}
	if got := w.lastRole(t); got != "silent" {
		t.Fatalf("last quest NPC = %s, want silent", got)
	}
}

// spawnHostile places a hostile NPC of kind under role, dx east of the
// player.
func (w *dialogWorld) spawnHostile(t *testing.T, role string, npcID int, kind string, dx int) *npc.Hostile {
	t.Helper()
	tmpl := &npc.Template{
		ID: npcID, TemplateID: npcID, Type: kind, Level: 20, HPMax: 1000, AtkSpd: 300,
		RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	h := w.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: w.at.X + dx, Y: w.at.Y, Z: w.at.Z})
	w.roles[h.ObjectID()], w.npcs[role] = role, h.ObjectID()
	w.srv.ReadQueued(t, w.srv.Client)
	return h
}

// TestGuardTalksOnInteract: a plain click on a selected town guard is an
// interact, not an attack. The guard opens its chat window, whose quest
// link reaches the guard's quest window; a guard whose first talk a script
// holds answers with that script; a starting village's guard answers
// nothing. Each talking guard becomes the last quest NPC.
func TestGuardTalksOnInteract(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, map[string]string{
		"guard/30039.htm": `<html><body>Guard %objectId% <a action="bypass -h npc_%objectId%_Quest">Quest</a></body></html>`,
	}, nil)
	guard := w.spawnHostile(t, "guard", guardID, "Guard", 20)
	judge := w.spawnHostile(t, "judgeguard", judgeGuardID, "Guard", 40)
	silent := w.spawnHostile(t, "silentguard", silentGuardID, "Guard", 60)

	got := w.renderTail(t, w.interact(t, guard.ObjectID()))
	want := []string{
		"S ActionFailed",
		`S NpcHtmlMessage obj=guard item=0 html="<html><body>Guard {guard} <a action=\"bypass -h npc_{guard}_Quest\">Quest</a></body></html>\n"`,
		"S ActionFailed",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("guard interact:\n got %q\nwant %q", got, want)
	}
	if got := w.lastRole(t); got != "guard" {
		t.Fatalf("last quest NPC = %s, want guard", got)
	}
	if got, want := w.bypass(t, w.expand("npc_{guard}_Quest")), []string{`S NpcHtmlMessage obj=guard item=0 html="<html><body>guard {guard}</body></html>"`, "S ActionFailed", "S ActionFailed"}; !slices.Equal(got, want) {
		t.Fatalf("guard chat window's quest link:\n got %q\nwant %q", got, want)
	}

	got = w.renderTail(t, w.interact(t, judge.ObjectID()))
	if want := []string{"S ActionFailed", `S NpcHtmlMessage obj=judgeguard item=0 html="<html><body>Judge {judgeguard}</body></html>"`, "S ActionFailed"}; !slices.Equal(got, want) {
		t.Fatalf("judging guard interact:\n got %q\nwant %q", got, want)
	}
	if got := w.lastRole(t); got != "judgeguard" {
		t.Fatalf("last quest NPC = %s, want judgeguard", got)
	}

	if got := w.renderTail(t, w.interact(t, silent.ObjectID())); !slices.Equal(got, []string{"S ActionFailed"}) {
		t.Fatalf("starting village guard interact = %q, want only the interact's ActionFailed", got)
	}
	if got := w.lastRole(t); got != "judgeguard" {
		t.Fatalf("last quest NPC after the silent guard = %s, want judgeguard kept", got)
	}
}

// TestHostileNPCReachesQuestWindowsAndEvents: a hostile guard a quest
// starts and talks through answers its npc_ Quest command with the quest's
// window, creating the quest and becoming the last quest NPC; the quest's
// event links then run through it. A page answer is a page through the
// guard, any other text a chat line.
func TestHostileNPCReachesQuestWindowsAndEvents(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, map[string]string{"script/quest/Q950_GuardQuest/next.htm": "<html><body>next %objectId%</body></html>"}, nil)
	w.spawnHostile(t, "guard", guardID, "Guard", 20)
	w.offerAll(t)
	got := w.bypass(t, w.expand("npc_{guard}_Quest"))
	want := []string{`S NpcHtmlMessage obj=guard item=0 html="<html><body>guard {guard}</body></html>"`, "S ActionFailed", "S ActionFailed"}
	if !slices.Equal(got, want) {
		t.Fatalf("guard quest window:\n got %q\nwant %q", got, want)
	}
	if got := w.lastRole(t); got != "guard" {
		t.Fatalf("last quest NPC = %s, want guard", got)
	}
	var created bool
	w.onCharacter(t, func(c *player.Character) { created = c.Quests().State(q950) != nil })
	if !created {
		t.Fatal("the guard's quest window created no state")
	}

	w.offerAll(t)
	if got, want := w.bypass(t, "Quest Q950_GuardQuest next.htm"), []string{`S NpcHtmlMessage obj=guard item=0 html="<html><body>next {guard}</body></html>\n"`, "S ActionFailed"}; !slices.Equal(got, want) {
		t.Fatalf("guard page event:\n got %q\nwant %q", got, want)
	}
	w.offerAll(t)
	if got, want := w.bypass(t, "Quest Q950_GuardQuest hello there"), []string{`S SystemMessage id=1987 0:"hello there"`}; !slices.Equal(got, want) {
		t.Fatalf("guard chat event:\n got %q\nwant %q", got, want)
	}
}

// TestQuestEventRefusedWhileTrading: a player running a private store, or
// setting one up, has a quest event link refused with ActionFailed, and
// the event does not run.
func TestQuestEventRefusedWhileTrading(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, q001Pages(loadContractPages(t)), nil)
	w.spawnGoldenNPCs(t, 0)
	w.onCharacter(t, func(c *player.Character) {
		c.SetLastQuestNPC(w.npcs["darin"])
		c.SetOperateType(privatestore.OperateSellManage)
	})
	w.offerAll(t)
	if got, want := w.bypass(t, "Quest Q001_LettersOfLove 30048-02.htm"), []string{"S ActionFailed"}; !slices.Equal(got, want) {
		t.Fatalf("event while trading:\n got %q\nwant %q", got, want)
	}
	w.onCharacter(t, func(c *player.Character) { c.SetOperateType(privatestore.OperateNone) })
	w.offerAll(t)
	if got := w.bypass(t, "Quest Q001_LettersOfLove 30048-02.htm"); len(got) != 2 || !strings.HasPrefix(got[0], "S NpcHtmlMessage obj=darin") {
		t.Fatalf("event after the store closed = %q, want Darin's page", got)
	}
}

// TestTutorialEventAnswerIsShown: what the tutorial quest answers a
// tutorial request is shown with no NPC: a page file of its directory, a
// whole page, or a chat line.
func TestTutorialEventAnswerIsShown(t *testing.T) {
	t.Parallel()
	srv, _, _ := bootTutorial(t, true, gameservertest.WithHTMLPages(map[string]string{
		"script/feature/Tutorial/answer.htm": "<html><body>answer %objectId%</body></html>",
	}))
	enterWorld(t, srv)
	w := &dialogWorld{srv: srv}
	for _, tc := range []struct {
		answer string
		want   []string
	}{
		{"answer.htm", []string{`S NpcHtmlMessage obj=0 item=0 html="<html><body>answer %objectId%</body></html>\n"`, "S ActionFailed"}},
		{"gone.htm", []string{`S NpcHtmlMessage obj=0 item=0 html="<html><body>My html is missing:<br>./data/html/script/feature/Tutorial/gone.htm</body></html>"`, "S ActionFailed"}},
		{"<html><body>whole</body></html>", []string{`S NpcHtmlMessage obj=0 item=0 html="<html><body>whole</body></html>"`, "S ActionFailed"}},
		{"a line", []string{`S SystemMessage id=1987 0:"a line"`}},
	} {
		srv.Client.Send(encodeTutorialString(clientpackets.OpcodeRequestTutorialPassCmdToServer, "answer "+tc.answer))
		if got := w.lines(t); !slices.Equal(got, tc.want) {
			t.Fatalf("answer %q:\n got %q\nwant %q", tc.answer, got, tc.want)
		}
	}
}

// TestLidiasDiaryHelpPageMarksTheDiaryRead: opening the diary's last help
// page for its item sets diary=1 in Lidia's Heart at condition 5 when the
// diary is not read yet; at another condition, already read, or on another
// page, nothing changes. The help page is shown each time.
func TestLidiasDiaryHelpPageMarksTheDiaryRead(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, cond, diary, help string
		want                    string
	}{
		{"marks", "5", "", "player_help lidias_diary/7064-16.htm#7064", "1"},
		{"any case", "5", "0", "player_help LIDIAS_DIARY/7064-16.htm#7064", "1"},
		{"other condition", "4", "", "player_help lidias_diary/7064-16.htm#7064", ""},
		{"already read", "5", "1", "player_help lidias_diary/7064-16.htm#7064", "1"},
		{"other page", "5", "", "player_help lidias_diary/7064-15.htm#7064", ""},
		{"no item", "5", "", "player_help lidias_diary/7064-16.htm", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootDialog(t, map[string]string{
				"help/lidias_diary/7064-16.htm": "<html><body>last</body></html>",
				"help/lidias_diary/7064-15.htm": "<html><body>before</body></html>",
			}, func(srv *gameservertest.Server, objID int32) {
				insertJournal(t, srv.DB, journalRow{objID, q023, "<state>", val("STARTED")}, journalRow{objID, q023, "<cond>", val(tc.cond)})
				if tc.diary != "" {
					insertJournal(t, srv.DB, journalRow{objID, q023, "diary", val(tc.diary)})
				}
			})
			got := w.bypass(t, tc.help)
			if len(got) != 1 || !strings.HasPrefix(got[0], "S NpcHtmlMessage obj=0") {
				t.Fatalf("help page = %q, want the page", got)
			}
			if got := w.srv.QuestVars(t, w.player, q023)["diary"]; got != tc.want {
				t.Fatalf("diary = %q, want %q", got, tc.want)
			}
			w.srv.FlushPersistence(t)
			if got := questRows(t, w.srv, w.player, q023)["diary"]; got != tc.want {
				t.Fatalf("diary row = %q, want %q", got, tc.want)
			}
		})
	}
}
