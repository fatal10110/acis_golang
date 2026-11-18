package quest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q620"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The no-page refusals of Four Goblets (Q620) that no scenario can play,
// since they need an item taken away between the page that offers the link
// and the click: the Nameless Spirit's hand-over of the goblets (31453-16),
// the second Ghost of Wigoth's sealed box (31454-14) and his recipe for
// broken relic parts (a recipe id, with 1000 parts). Each answers nothing,
// as the reference answers null, and gives and takes nothing.
// A digit event that is no recipe id is never offered by a page.
const (
	q620StaleName    = "Q620_FourGoblets"
	q620StaleRelic   = 7254
	q620StaleBox     = 7255
	q620StaleGoblet1 = 7256
	q620StaleBrooch  = 7262
	namelessSpirit   = 31453
	wigoth2          = 31454
	recipeLink       = "6881"
)

// q620StaleWorld is a level 74 character on Four Goblets at cond 1 (with
// completed set when asked) beside the NPC, holding items.
func q620StaleWorld(t *testing.T, npcID int, completed bool, held map[int32]int32) (*dialogWorld, int32) {
	t.Helper()
	links := map[string][]string{}
	if npcID == namelessSpirit {
		links["31453-15.htm"] = []string{"31453-16.htm"}
	} else {
		for i := 1; i <= 12; i++ {
			links[fmt.Sprintf("31454-%02d.htm", i)] = []string{"31454-14.htm", recipeLink}
		}
	}
	items := []int32{q620StaleRelic, q620StaleBox, q620StaleGoblet1, q620StaleGoblet1 + 1, q620StaleGoblet1 + 2, q620StaleGoblet1 + 3, q620StaleBrooch}
	w := bootSeamQuest(t, 74, chatPages(questPages(q620StaleName, links), npcID), func(srv *gameservertest.Server, objID int32) {
		seedJournal(t, srv, objID, []string{q620StaleName + ":STARTED"})
		if completed {
			insertJournal(t, srv.DB, journalRow{objID, q620StaleName, "completed", val("true")})
		}
		for id, n := range held {
			srv.GiveItem(t, objID, id, n)
		}
	},
		varkaItemTemplates(items...),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{int32(npcID): script.KindFolk},
			[]script.Listing{{Path: q620Path}},
			script.Catalog{q620Path: q620.New},
		),
	)
	return w, w.spawnFolk(t, "ghost", npcID, 0, 30, 0).ObjectID()
}

// TestQ620StaleLinkAnswersNothing shows each page, takes the item its link
// needs away, and clicks the link: nothing is sent, the quest does not
// move and nothing is given or taken.
func TestQ620StaleLinkAnswersNothing(t *testing.T) {
	t.Parallel()
	goblets := map[int32]int32{q620StaleGoblet1: 1, q620StaleGoblet1 + 1: 1, q620StaleGoblet1 + 2: 1, q620StaleGoblet1 + 3: 1}
	for _, tc := range []struct {
		name      string
		npc       int
		completed bool
		held      map[int32]int32
		page      string
		take      func(c *player.Character)
		link      string
		want      map[int32]int
	}{
		{
			"goblets", namelessSpirit, false, goblets, "31453-15.htm",
			func(c *player.Character) { c.ConsumeItem(q620StaleGoblet1, 1) }, "31453-16.htm",
			map[int32]int{q620StaleGoblet1: 0, q620StaleGoblet1 + 1: 1, q620StaleGoblet1 + 2: 1, q620StaleGoblet1 + 3: 1, q620StaleBrooch: 0},
		},
		{
			"sealed box", wigoth2, true,
			map[int32]int32{q620StaleBox: 1},
			"",
			func(c *player.Character) { c.ConsumeItem(q620StaleBox, 1) }, "31454-14.htm",
			map[int32]int{q620StaleBox: 0, q620StaleRelic: 0},
		},
		{
			"recipe with fewer than 1000 parts", wigoth2, true,
			map[int32]int32{q620StaleRelic: 1000},
			"",
			func(c *player.Character) { c.ConsumeItem(q620StaleRelic, 1) }, recipeLink,
			map[int32]int{q620StaleRelic: 999, 6881: 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, ghost := q620StaleWorld(t, tc.npc, tc.completed, tc.held)
			got := w.questWindow(t, ghost)
			if tc.page != "" && !shows(got, tc.page) {
				t.Fatalf("window = %q, want %s", got, tc.page)
			}
			if tc.page == "" && !slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, "S NpcHtmlMessage ") && strings.Contains(l, "31454-") }) {
				t.Fatalf("window = %q, want one of Wigoth's pages", got)
			}
			c := onlineCharacter(t, w.srv, w.player)
			done := make(chan struct{})
			if !w.srv.PlayerQueue(t, w.player).Post(func() { defer close(done); tc.take(c) }) {
				t.Fatal("the player's queue is closed")
			}
			<-done
			got = w.bypass(t, "Quest "+q620StaleName+" "+tc.link)
			if slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, "S NpcHtmlMessage ") }) {
				t.Fatalf("%s sent %q, want no page", tc.link, got)
			}
			for id, n := range tc.want {
				if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
					t.Errorf("item %d held = %d, want %d", id, c, n)
				}
			}
			if vars := w.srv.QuestVars(t, w.player, q620StaleName); vars["<cond>"] != "1" || vars["<state>"] != "STARTED" {
				t.Errorf("Q620 = %v, want started at cond 1", vars)
			}
		})
	}
}
