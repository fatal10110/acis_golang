package quest

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q235"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the Mimir's Elixir (Q235) scenarios cannot show, since they hold no
// item: Ladd's offer to a holder of the Star of Destiny, the pure silver
// Ladd and Joan wait for, the mixing at the urn and the elixir traded for
// the scroll (Q235_MimirsElixir.onTalk, onAdvEvent).

const (
	q235Name = "Q235_MimirsElixir"
	q235Path = "quest.Q235_MimirsElixir"

	q235Joan      = 30718
	q235Ladd      = 30721
	q235MixingUrn = 31149

	q235StarOfDestiny = 5011
	q235PureSilver    = 6320
	q235TrueGold      = 6321
	q235SageStone     = 6322
	q235BloodFire     = 6318
	q235MimirElixir   = 6319
	q235MixingStone   = 5905
	q235ScrollEnchant = 729
)

func bootQ235(t *testing.T, before func(srv *gameservertest.Server, objID int32)) *dialogWorld {
	t.Helper()
	return bootX1Quest(t, 75, q235Path, q235.New, []int32{q235Joan, q235Ladd, q235MixingUrn}, nil, before,
		x1Items([]int32{q235StarOfDestiny, q235PureSilver, q235TrueGold, q235SageStone, q235BloodFire, q235MimirElixir, q235MixingStone, q235ScrollEnchant}))
}

// TestQ235LaddOffersToAHolderOfTheStar: a level 75 holder of the Star of
// Destiny is offered the quest and takes it; with pure silver held, Ladd
// moves it to cond 2 and sends the player to Joan, who sends it after the
// sage stone (cond 3).
func TestQ235LaddOffersToAHolderOfTheStar(t *testing.T) {
	t.Parallel()
	w := bootQ235(t, func(srv *gameservertest.Server, objID int32) {
		seedLevelExp(t, srv, objID, 75)
		srv.GiveItem(t, objID, q235StarOfDestiny, 1)
		srv.GiveItem(t, objID, q235PureSilver, 1)
	})
	ladd := w.spawnFolk(t, "ladd", q235Ladd, 10, 0, 0).ObjectID()
	joan := w.spawnFolk(t, "joan", q235Joan, -10, 0, 0).ObjectID()

	if got := w.questWindow(t, ladd); !slices.Contains(got, w.page(t, "ladd", q235Name, "30721-01.htm")) {
		t.Fatalf("Ladd's window = %q, want 30721-01.htm", got)
	}
	for _, file := range []string{"30721-02.htm", "30721-03.htm", "30721-04.htm", "30721-05.htm"} {
		if got := w.bypass(t, "Quest "+q235Name+" "+file); !slices.Contains(got, w.page(t, "ladd", q235Name, file)) {
			t.Fatalf("link %s = %q", file, got)
		}
	}
	got := w.bypass(t, "Quest "+q235Name+" 30721-06.htm")
	if want := "S PlaySound type=0 file=ItemSound.quest_accept bind=0 obj=0 loc=0,0,0 delay=0"; !slices.Contains(got, want) {
		t.Fatalf("the accept = %q, want the accept sound", got)
	}
	if vars := w.srv.QuestVars(t, w.player, q235Name); vars["<state>"] != "STARTED" || vars["<cond>"] != "1" {
		t.Fatalf("Q235 after the accept = %v, want started at cond 1", vars)
	}

	if got := w.questWindow(t, ladd); !slices.Contains(got, w.page(t, "ladd", q235Name, "30721-08.htm")) {
		t.Fatalf("Ladd's window with pure silver = %q, want 30721-08.htm", got)
	}
	if vars := w.srv.QuestVars(t, w.player, q235Name); vars["<cond>"] != "2" {
		t.Fatalf("Q235 after the silver = %v, want cond 2", vars)
	}
	if got := w.questWindow(t, joan); !slices.Contains(got, w.page(t, "joan", q235Name, "30718-01.htm")) {
		t.Fatalf("Joan's window = %q, want 30718-01.htm", got)
	}
	w.bypass(t, "Quest "+q235Name+" 30718-02.htm")
	w.bypass(t, "Quest "+q235Name+" 30718-03.htm")
	if vars := w.srv.QuestVars(t, w.player, q235Name); vars["<cond>"] != "3" {
		t.Fatalf("Q235 after Joan = %v, want cond 3", vars)
	}
}

// TestQ235UrnMixesAndLaddTradesTheElixir: at cond 7 with the mixing
// stone, pure silver, true gold and blood fire, the urn's links pass each
// step and the success takes the three ingredients for the elixir (cond
// 8). Ladd then casts on the player, takes the stone, the elixir and the
// star, gives the scroll, and the quest is completed for good.
func TestQ235UrnMixesAndLaddTradesTheElixir(t *testing.T) {
	t.Parallel()
	w := bootQ235(t, func(srv *gameservertest.Server, objID int32) {
		seedLevelExp(t, srv, objID, 75)
		insertJournal(t, srv.DB,
			journalRow{objID, q235Name, "<state>", val("STARTED")},
			journalRow{objID, q235Name, "<cond>", val("7")},
		)
		for _, id := range []int32{q235StarOfDestiny, q235MixingStone, q235PureSilver, q235TrueGold, q235BloodFire} {
			srv.GiveItem(t, objID, id, 1)
		}
	})
	urn := w.spawnFolk(t, "urn", q235MixingUrn, 0, 10, 0).ObjectID()
	ladd := w.spawnFolk(t, "ladd", q235Ladd, 10, 0, 0).ObjectID()

	if got := w.questWindow(t, urn); !slices.Contains(got, w.page(t, "urn", q235Name, "31149-01.htm")) {
		t.Fatalf("the urn's window = %q, want 31149-01.htm", got)
	}
	for _, file := range []string{"31149-02.htm", "31149-03.htm", "31149-04.htm", "31149-05.htm", "31149-06.htm", "31149-07.htm"} {
		if got := w.bypass(t, "Quest "+q235Name+" "+file); !slices.Contains(got, w.page(t, "urn", q235Name, file)) {
			t.Fatalf("link %s = %q", file, got)
		}
	}
	got := keep(w.bypass(t, "Quest "+q235Name+" 31149-success.htm"), "S SystemMessage", "S PlaySound", "S NpcHtmlMessage")
	want := []string{
		"S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0",
		itemMessage(302, q235PureSilver),
		itemMessage(302, q235TrueGold),
		itemMessage(302, q235BloodFire),
		itemMessage(54, q235MimirElixir),
		w.page(t, "urn", q235Name, "31149-success.htm"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the mixing = %q, want %q", got, want)
	}
	if vars := w.srv.QuestVars(t, w.player, q235Name); vars["<cond>"] != "8" {
		t.Fatalf("Q235 after the mixing = %v, want cond 8", vars)
	}

	if got := w.questWindow(t, ladd); !slices.Contains(got, w.page(t, "ladd", q235Name, "30721-14.htm")) {
		t.Fatalf("Ladd's window with the elixir = %q, want 30721-14.htm", got)
	}
	w.bypass(t, "Quest "+q235Name+" 30721-15.htm")
	got = keep(w.bypassAll(t, "Quest "+q235Name+" 30721-16.htm"), "S MagicSkillUse", "S SystemMessage", "S SocialAction", "S PlaySound", "S QuestList", "S NpcHtmlMessage")
	want = []string{
		"S MagicSkillUse caster=player target=player skill=4339-1 hit=1 reuse=1 success=0",
		itemMessage(302, q235MixingStone),
		itemMessage(302, q235MimirElixir),
		itemMessage(302, q235StarOfDestiny),
		itemMessage(54, q235ScrollEnchant),
		"S SocialAction obj=player action=3",
		"S PlaySound type=0 file=ItemSound.quest_finish bind=0 obj=0 loc=0,0,0 delay=0",
		"S QuestList 235:0x00000000",
		w.page(t, "ladd", q235Name, "30721-16.htm"),
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the trade = %q, want %q", got, want)
	}
	for id, n := range map[int32]int{q235ScrollEnchant: 1, q235StarOfDestiny: 0, q235MixingStone: 0, q235MimirElixir: 0} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the trade = %d, want %d", id, c, n)
		}
	}
	if vars := w.srv.QuestVars(t, w.player, q235Name); vars["<state>"] != "COMPLETED" {
		t.Fatalf("Q235 after the trade = %v, want completed", vars)
	}
	if got := w.questWindow(t, ladd); !slices.Contains(got, `S NpcHtmlMessage obj=ladd item=0 html="<html><body>This quest has already been completed.</body></html>"`) {
		t.Fatalf("Ladd's window after the trade = %q, want the completed page", got)
	}
}
