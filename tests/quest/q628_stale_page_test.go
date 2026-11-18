package quest

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q628"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestQ628HandInWithAStaleChitinCount: Kahman's page 31554-03 offers the
// hand-in of the splinters to a character who holds 100; one moved away
// before the click leaves 99, so the hand-in event takes nothing and gives
// no badge, answers the page it names (the reference keeps the event as the
// answer when the count is short), and the quest stays at cond 1. A client
// cannot reach this from a page it was shown with fewer splinters, so no
// scenario can (the page 31554-03a of few_splinters is Kahman's talk).
func TestQ628HandInWithAStaleChitinCount(t *testing.T) {
	t.Parallel()
	const kahman = 31554
	_, itemOpt := q628Items(t)
	w := bootSeamQuest(t, 66, chatPages(shippedQuestPages(t, q628Name), kahman), func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q628Name + ":STARTED"})
		srv.GiveItem(t, objID, q628Splinter, 100)
	},
		itemOpt,
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{kahman: script.KindFolk},
			[]script.Listing{{Path: q628Path}}, script.Catalog{q628Path: q628.New}),
	)
	npcID := w.spawnFolk(t, "kahman", kahman, 0, 30, 0).ObjectID()
	if got := w.questWindow(t, npcID); !slices.ContainsFunc(got, func(l string) bool {
		return strings.HasPrefix(l, "S NpcHtmlMessage ") && strings.Contains(l, "Quest "+q628Name+" 31554-03a.htm")
	}) {
		t.Fatalf("Kahman's window with 100 splinters = %q, want the page that links 31554-03a.htm", got)
	}
	c := onlineCharacter(t, w.srv, w.player)
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player).Post(func() { defer close(done); c.ConsumeItem(q628Splinter, 1) }) {
		t.Fatal("the player's queue is closed")
	}
	<-done
	if got := w.bypass(t, "Quest "+q628Name+" 31554-03a.htm"); !slices.ContainsFunc(got, func(l string) bool {
		return strings.HasPrefix(l, "S NpcHtmlMessage ") && strings.Contains(l, "collect <font color=\\\"LEVEL\\\">100 Chitins")
	}) {
		t.Fatalf("hand-in with 99 splinters = %q, want the page 31554-03a that asks for 100 chitins", got)
	}
	if n, b := w.srv.PlayerItemCount(t, w.player, q628Splinter), w.srv.PlayerItemCount(t, w.player, q628Badge); n != 99 || b != 0 {
		t.Fatalf("splinters and badges = %d, %d; want 99 and 0", n, b)
	}
	if vars := w.srv.QuestVars(t, w.player, q628Name); vars["<cond>"] != "1" {
		t.Fatalf("Q628 = %v, want cond 1", vars)
	}
}
