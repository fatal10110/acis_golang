package quest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// q001FlowPages stand in for the datapack's pages: each names its file
// and carries the reference page's links, and each NPC's chat window links
// to its quests.
func q001FlowPages() map[string]string {
	link := func(event string) string {
		return `<a action="bypass -h Quest Q001_LettersOfLove ` + event + `">go</a>`
	}
	pages := map[string]string{}
	for _, id := range []int{darinID, roxxyID, baulroID} {
		pages[fmt.Sprintf("default/%d.htm", id)] = `<html><body>chat <a action="bypass -h npc_%objectId%_Quest">Quest</a></body></html>`
	}
	for file, links := range map[string]string{
		"30048-01.htm": "", "30048-02.htm": link("30048-03.htm"),
		"30048-03.htm": link("30048-04.htm") + link("30048-05.htm"),
		"30048-04.htm": link("30048-06.htm"), "30048-05.htm": link("30048-06.htm"),
		"30048-06.htm": "", "30048-07.htm": "", "30048-08.htm": "", "30048-09.htm": "", "30048-10.htm": "",
		"30006-01.htm": "", "30006-02.htm": "", "30006-03.htm": "", "30033-01.htm": "", "30033-02.htm": "",
	} {
		pages["script/quest/Q001_LettersOfLove/"+file] = "<html><body>" + file + links + "</body></html>"
	}
	return pages
}

// TestQ001FromFirstTalkToCompletionAndRelog plays Letters of Love through
// client packets with stand-in pages: Darin's quest window creates the
// quest, its accept link starts it with Darin's letter, each delivery
// moves the condition on and swaps the quest item, and Darin's last answer
// completes the quest for the necklace, leaving one row. A relog keeps the
// necklace and the completed quest.
func TestQ001FromFirstTalkToCompletionAndRelog(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, q001FlowPages(), nil)
	darin := w.spawnFolk(t, "darin", darinID, 10, 0, 0).ObjectID()
	roxxy := w.spawnFolk(t, "roxxy", roxxyID, 20, 0, 0).ObjectID()
	baulro := w.spawnFolk(t, "baulro", baulroID, 30, 0, 0).ObjectID()
	w.srv.TakeJournalWrites()

	// page asserts that the answer showed file through role and ended
	// with ActionFailed.
	page := func(t *testing.T, got []string, role, file string) {
		t.Helper()
		if !slices.ContainsFunc(got, func(l string) bool {
			return strings.HasPrefix(l, "S NpcHtmlMessage obj="+role+" ") && strings.Contains(l, "<html><body>"+file)
		}) || got[len(got)-1] != "S ActionFailed" {
			t.Fatalf("answer %q does not show %s through %s", got, file, role)
		}
	}
	quest := func(t *testing.T, obj int32) []string {
		t.Helper()
		w.interact(t, obj)
		return w.bypass(t, fmt.Sprintf("npc_%d_Quest", obj))
	}
	rows := func(t *testing.T, want string) {
		t.Helper()
		w.srv.FlushPersistence(t)
		if got := fmt.Sprint(questRows(t, w.srv, w.player, q001)); got != want {
			t.Fatalf("rows = %s, want %s", got, want)
		}
	}
	items := func(t *testing.T, want map[int32]int) {
		t.Helper()
		for _, id := range append(slices.Clone(q001Items), necklace) {
			if got := w.srv.PlayerItemCount(t, w.player, id); got != want[id] {
				t.Fatalf("item %d count = %d, want %d", id, got, want[id])
			}
		}
	}

	page(t, quest(t, darin), "darin", "30048-02.htm")
	if got := w.states(t); got != q001+":CREATED" {
		t.Fatalf("states after Darin's window = %s, want Q001 created", got)
	}
	rows(t, "map[]")
	page(t, w.bypass(t, "Quest Q001_LettersOfLove 30048-03.htm"), "darin", "30048-03.htm")
	page(t, w.bypass(t, "Quest Q001_LettersOfLove 30048-04.htm"), "darin", "30048-04.htm")
	accept := w.bypass(t, "Quest Q001_LettersOfLove 30048-06.htm")
	want := []string{
		"S QuestList 1:0x80000001", "S ExShowQuestMark quest=1",
		"S PlaySound type=0 file=ItemSound.quest_accept bind=0 obj=0 loc=0,0,0 delay=0",
		"S SystemMessage id=54 3:687",
	}
	if len(accept) < len(want) || !slices.Equal(accept[:len(want)], want) {
		t.Fatalf("accept = %q, want it to start %q", accept, want)
	}
	page(t, accept, "darin", "30048-06.htm")
	rows(t, "map[<cond>:1 <state>:STARTED]")
	items(t, map[int32]int{687: 1})

	page(t, quest(t, darin), "darin", "30048-07.htm")

	choice := quest(t, roxxy)
	if !strings.Contains(choice[0], "[Letters of Love (In Progress)]") {
		t.Fatalf("Roxxy's window = %q, want the quest choice list", choice)
	}
	page(t, w.bypass(t, fmt.Sprintf("npc_%d_Quest Q001_LettersOfLove", roxxy)), "roxxy", "30006-01.htm")
	rows(t, "map[<cond>:2 <state>:STARTED]")
	items(t, map[int32]int{688: 1})

	page(t, quest(t, darin), "darin", "30048-08.htm")
	items(t, map[int32]int{1079: 1})
	page(t, quest(t, baulro), "baulro", "30033-01.htm")
	rows(t, "map[<cond>:4 <state>:STARTED]")
	items(t, map[int32]int{1080: 1})
	page(t, quest(t, darin), "darin", "30048-10.htm")
	rows(t, "map[<state>:COMPLETED]")
	items(t, map[int32]int{necklace: 1})

	restart(t, w.srv)
	enterWorld(t, w.srv)
	x, y, z := w.srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	if got := w.states(t); got != q001+":COMPLETED" {
		t.Fatalf("states after relog = %s, want Q001 completed", got)
	}
	items(t, map[int32]int{necklace: 1})
	completed := quest(t, darin)
	if want := `S NpcHtmlMessage obj=darin item=0 html="<html><body>This quest has already been completed.</body></html>"`; completed[0] != want {
		t.Fatalf("Darin after relog = %q, want %q", completed, want)
	}
}
