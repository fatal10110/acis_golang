package quest

import (
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q232"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// A quest link stays usable after the page that offered it, so a player can
// give away or destroy what the page checked and still follow it. These
// events check it again when the link is followed and refuse with the page
// the NPC shows without it (owner-approved divergence, #5163).

const (
	q232Jakal         = 30558
	q232TantusCharm   = 3393
	q232NerugaBlade   = 3405
	sagaNPC2nd        = 31624
	sagaIceCrystal    = 7080
	sagaOptionalFish  = 6482
	sagaRewardItem    = 7534
	clickAdenaID      = 57
	clickItemGetSound = "S PlaySound type=0 file=ItemSound.quest_itemget bind=0 obj=0 loc=0,0,0 delay=0"
)

// destroyHeldItem destroys count units of the player's templateID through
// the client's destroy packet; quest names a script registered in w.
func (w *dialogWorld) destroyHeldItem(t *testing.T, quest string, templateID, count int32) {
	t.Helper()
	var objectID int32
	w.srv.RunScript(t, w.player, quest, func(_ *script.Script, p *script.Player) {
		for _, held := range p.HeldItems() {
			if held.ItemID == templateID {
				objectID = held.ObjectID
			}
		}
	})
	if objectID == 0 {
		t.Fatalf("the player holds no %d", templateID)
	}
	w.srv.Client.Send(encodeRequestDestroyItem(objectID, count))
	w.srv.ReadQueued(t, w.srv.Client)
}

// requireItems requires the player to hold each template of want as many
// times as it maps to.
func (w *dialogWorld) requireItems(t *testing.T, step string, want map[int32]int) {
	t.Helper()
	for id, n := range want {
		if got := w.srv.PlayerItemCount(t, w.player, id); got != n {
			t.Errorf("%s: item %d = %d, want %d", step, id, got, n)
		}
	}
}

// TestQ232JakalChecksTheAdenaWhenTheLinkIsFollowed: Jakal offers the axe
// blade for 1000 adena to a player holding Tantus' charm and the adena. One
// that spends the adena before following the link is told it is short
// (30558-03.htm), keeps what it holds and gets no blade; one that keeps it
// pays 1000 once for the blade.
func TestQ232JakalChecksTheAdenaWhenTheLinkIsFollowed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		destroy int32
		page    string
		want    map[int32]int
	}{
		{name: "adena spent", destroy: 4500, page: "30558-03.htm", want: map[int32]int{clickAdenaID: 500, q232NerugaBlade: 0, q232TantusCharm: 1}},
		{name: "adena kept", page: "30558-02.htm", want: map[int32]int{clickAdenaID: 4000, q232NerugaBlade: 1, q232TantusCharm: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pages := chatPages(questPages(q232Name, map[string][]string{
				"30558-01.htm": {"30558-02.htm"}, "30558-02.htm": nil, "30558-03.htm": nil,
			}), q232Jakal)
			kinds := map[int32]script.NPCKind{}
			for _, id := range []int32{30565, 30642, 30568, 30510, 30564, 30641, 30567, 30558, 30566, 30515, q232Martankus, q232FirstOrc} {
				kinds[id] = script.KindFolk // the quest's talkers, so its bindings register
			}
			for _, id := range []int32{20233, 20269, 20270, 20564, 20583, 20584, 20585, 20586, 20587, 20588, 20778, 20779} {
				kinds[id] = script.KindHostile
			}
			w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
				insertJournal(t, srv.DB,
					journalRow{objID, q232Name, "<state>", val("STARTED")},
					journalRow{objID, q232Name, "<cond>", val("1")})
				srv.GiveItem(t, objID, q232TantusCharm, 1)
				srv.GiveItem(t, objID, clickAdenaID, 5000)
			},
				x1Items([]int32{q232TantusCharm, q232NerugaBlade}),
				gameservertest.WithNpcSpawns(nil),
				gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q232Path}}, script.Catalog{q232Path: q232.New}),
			)
			jakal := w.spawnFolk(t, "jakal", q232Jakal, 30, 0, 0).ObjectID()
			if got := w.questWindow(t, jakal); !shows(got, "30558-01.htm") {
				t.Fatalf("Jakal's window = %q, want 30558-01.htm", got)
			}
			if tc.destroy > 0 {
				w.destroyHeldItem(t, q232Name, clickAdenaID, tc.destroy)
			}
			got := w.bypass(t, "Quest "+q232Name+" 30558-02.htm")
			if !shows(got, tc.page) {
				t.Fatalf("the blade link = %q, want %s", got, tc.page)
			}
			if sound := slices.Contains(got, clickItemGetSound); sound != (tc.destroy == 0) {
				t.Fatalf("the blade link = %q, want the item sound %t", got, tc.destroy == 0)
			}
			w.requireItems(t, "after the blade link", tc.want)
			if vars := w.srv.QuestVars(t, w.player, q232Name); vars["<cond>"] != "1" {
				t.Fatalf("Q232 after the blade link = %v, want cond 1", vars)
			}
		})
	}
}

// TestQ422JudgeChecksTheLeftoverManaclesWhenTheLinkIsFollowed: at cond 16
// without the Sin Eater's manacles, the Black Judge offers them back for the
// leftover manacles. A player that destroys the leftover ones before
// following the link is sent back to bring them (30981-18.htm), keeps cond
// 16 and its kept level unset, and gets no manacles; one that keeps them
// trades them for the manacles as before.
func TestQ422JudgeChecksTheLeftoverManaclesWhenTheLinkIsFollowed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		destroy bool
		page    string
		level   string
		want    map[int32]int
	}{
		{name: "leftover destroyed", destroy: true, page: "30981-18.htm", want: map[int32]int{q422LeftManacles: 0, q422PenitentManacles: 0}},
		{name: "leftover kept", page: "30981-11.htm", level: "45", want: map[int32]int{q422LeftManacles: 0, q422PenitentManacles: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, judge, _ := bootQ422(t, 45, 3, &q422Draws{}, func(srv *gameservertest.Server, objID int32) {
				insertJournal(t, srv.DB,
					journalRow{objID, q422Name, "<state>", val("STARTED")},
					journalRow{objID, q422Name, "<cond>", val("16")},
				)
				srv.GiveItem(t, objID, q422LeftManacles, 1)
			})
			if got := w.questWindow(t, judge); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-18.htm")) {
				t.Fatalf("the Black Judge's window = %q, want 30981-18.htm", got)
			}
			if got := w.bypass(t, "Quest "+q422Name+" 30981-19.htm"); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-19.htm")) {
				t.Fatalf("the leftover link = %q, want 30981-19.htm", got)
			}
			if tc.destroy {
				w.destroyHeldItem(t, q422Name, q422LeftManacles, 1)
			}
			got := w.bypass(t, "Quest "+q422Name+" 30981-11.htm")
			if !slices.Contains(got, w.page(t, "judge", q422Name, tc.page)) {
				t.Fatalf("the manacles link = %q, want %s", got, tc.page)
			}
			if sound := slices.Contains(got, clickItemGetSound); sound == tc.destroy {
				t.Fatalf("the manacles link = %q, want the item sound %t", got, !tc.destroy)
			}
			if vars := w.srv.QuestVars(t, w.player, q422Name); vars["<cond>"] != "16" || vars["level"] != tc.level {
				t.Fatalf("Q422 after the manacles link = %v, want cond 16 and level %q", vars, tc.level)
			}
			w.requireItems(t, "after the manacles link", tc.want)
		})
	}
}

// bootSagaTrade boots a level 76 Paladin at cond 3 of the test saga, made
// with optional as its optional item, holding items, beside the saga's
// second NPC.
func bootSagaTrade(t *testing.T, optional int32, items map[int32]int32) (*dialogWorld, int32) {
	t.Helper()
	family := thirdclass.NewFamily()
	sg := testSaga()
	sg.ItemOptional = optional
	kinds := map[int32]script.NPCKind{
		sagaNPC2nd: script.KindFolk, tablet3ID: script.KindFolk, tablet6ID: script.KindFolk, sedrickID: script.KindFolk, defenderID: script.KindFolk,
		corruptedID: script.KindHostile, attackerID: script.KindHostile, halishaID: script.KindHostile, scarabID: script.KindHostile,
		sepulchersArchonID: script.KindHostile,
	}
	w := bootSeamQuest(t, 76, chatPages(shippedQuestPages(t, q070Name), sagaNPC2nd),
		func(srv *gameservertest.Server, objID int32) {
			sagaPlayer(t, srv, objID, 3)
			for id, n := range items {
				srv.GiveItem(t, objID, id, n)
			}
		},
		gameservertest.WithClassTemplates(sagaClassTemplates()...),
		x1Items([]int32{sagaIceCrystal, sagaOptionalFish, sagaRewardItem}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(corruptedID), proofMonster(attackerID), proofMonster(halishaID), proofMonster(scarabID),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds,
			[]script.Listing{{Path: q070Path}, {Path: sagaCorePath}},
			script.Catalog{
				q070Path:     func() script.Script { return thirdclass.New(family, sg) },
				sagaCorePath: func() script.Script { return thirdclass.NewCore(family) },
			}),
	)
	w.roles[w.player] = "player"
	return w, w.spawnFolk(t, "npc2nd", sagaNPC2nd, 30, 0, 0).ObjectID()
}

// TestSagaSecondNPCChecksTheTradeWhenTheLinkIsFollowed: at cond 3 the
// saga's second NPC offers the reward item for the Ice Crystal and, when the
// saga has one, its optional item. A player that destroys either before
// following the link is told to bring them (1-02.htm), stays at cond 3,
// keeps the rest and gets no reward; one that keeps them trades them as
// before (cond 4).
func TestSagaSecondNPCChecksTheTradeWhenTheLinkIsFollowed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		optional int32
		destroy  int32
		page     string
		cond     string
		want     map[int32]int
	}{
		{
			name: "optional destroyed", optional: sagaOptionalFish, destroy: sagaOptionalFish, page: "1-02.htm", cond: "3",
			want: map[int32]int{sagaIceCrystal: 1, sagaOptionalFish: 0, sagaRewardItem: 0},
		},
		{
			name: "crystal destroyed", optional: sagaOptionalFish, destroy: sagaIceCrystal, page: "1-02.htm", cond: "3",
			want: map[int32]int{sagaIceCrystal: 0, sagaOptionalFish: 1, sagaRewardItem: 0},
		},
		{
			name: "both kept", optional: sagaOptionalFish, page: "1-06.htm", cond: "4",
			want: map[int32]int{sagaIceCrystal: 0, sagaOptionalFish: 0, sagaRewardItem: 1},
		},
		{
			name: "no optional, crystal destroyed", destroy: sagaIceCrystal, page: "1-02.htm", cond: "3",
			want: map[int32]int{sagaIceCrystal: 0, sagaRewardItem: 0},
		},
		{
			name: "no optional, crystal kept", page: "1-06.htm", cond: "4",
			want: map[int32]int{sagaIceCrystal: 0, sagaRewardItem: 1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			items := map[int32]int32{sagaIceCrystal: 1}
			if tc.optional != 0 {
				items[tc.optional] = 1
			}
			w, npc2nd := bootSagaTrade(t, tc.optional, items)
			if got := w.questWindow(t, npc2nd); !slices.Contains(got, w.page(t, "npc2nd", q070Name, "1-03.htm")) {
				t.Fatalf("the second NPC's window = %q, want 1-03.htm", got)
			}
			if tc.destroy != 0 {
				w.destroyHeldItem(t, q070Name, tc.destroy, 1)
			}
			got := w.bypass(t, "Quest "+q070Name+" 1-4")
			if !slices.Contains(got, w.page(t, "npc2nd", q070Name, tc.page)) {
				t.Fatalf("the trade link = %q, want %s", got, tc.page)
			}
			if sound := slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, "S PlaySound ") }); sound != (tc.cond == "4") {
				t.Fatalf("the trade link = %q, want a sound %t", got, tc.cond == "4")
			}
			if vars := w.srv.QuestVars(t, w.player, q070Name); vars["<cond>"] != tc.cond {
				t.Fatalf("the saga after the trade link = %v, want cond %s", vars, tc.cond)
			}
			w.requireItems(t, "after the trade link", tc.want)
		})
	}
}
