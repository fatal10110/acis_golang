package quest

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q422"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the Repent Your Sins (Q422) scenario cannot show, since it sets no
// PK count and holds no item: the Black Judge's offer and the errand each
// level is sent on, the manacles Pushkin forges, the PKs the grown Sin
// Eater takes away, and the quit (Q422_RepentYourSins.onTalk,
// onAdvEvent).

const (
	q422Name = "Q422_RepentYourSins"
	q422Path = "quest.Q422_RepentYourSins"

	q422BlackJudge = 30981
	q422Pushkin    = 30300

	q422QitemManacles    = 4330
	q422ManualOfManacles = 4331
	q422PenitentManacles = 4425
	q422LeftManacles     = 4426
	q422Silver           = 1873
	q422Adamantine       = 1877
	q422Frame            = 1892
	q422Cokes            = 1879
	q422Steel            = 1880
	q422VenomSac         = 4329
)

// q422Draws is the script random source of a Q422 world: every draw is
// draw, and each draw's bound is kept.
type q422Draws struct {
	draw   int
	bounds []int
}

func (d *q422Draws) rand(n int) int {
	d.bounds = append(d.bounds, n)
	return d.draw
}

// bootQ422 boots a character of level with pk PK kills, the Black Judge
// and Pushkin; before seeds what precedes the selection.
func bootQ422(t *testing.T, level, pk int, draws *q422Draws, before func(srv *gameservertest.Server, objID int32)) (w *dialogWorld, judge, pushkin int32) {
	t.Helper()
	w = bootX1Quest(t, level, q422Path, q422.New, []int32{q422BlackJudge, q422Pushkin}, nil,
		func(srv *gameservertest.Server, objID int32) {
			seedLevelExp(t, srv, objID, level)
			if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET pkkills = ? WHERE obj_Id = ?", pk, objID); err != nil {
				t.Fatalf("seed pkkills: %v", err)
			}
			if before != nil {
				before(srv, objID)
			}
		},
		x1Items([]int32{
			q422QitemManacles, q422ManualOfManacles, q422PenitentManacles, q422LeftManacles,
			q422Silver, q422Adamantine, q422Frame, q422Cokes, q422Steel, q422VenomSac,
		}),
		gameservertest.WithScriptRand(draws.rand),
	)
	return w, w.spawnFolk(t, "judge", q422BlackJudge, 10, 0, 0).ObjectID(), w.spawnFolk(t, "pushkin", q422Pushkin, -10, 0, 0).ObjectID()
}

// pkKills is the player's PK count.
func (w *dialogWorld) pkKills(t *testing.T) int32 {
	t.Helper()
	var n int32
	w.srv.RunScript(t, w.player, q422Name, func(_ *script.Script, p *script.Player) { n = p.PkKills() })
	return n
}

// TestQ422BlackJudgeSendsEachLevelOnItsErrand: a player without a PK is
// turned away with no way in; one with a PK is offered the quest, and the
// start sends it on the errand of its level: up to 20, 21 to 30, 31 to 40,
// past 40.
func TestQ422BlackJudgeSendsEachLevelOnItsErrand(t *testing.T) {
	t.Parallel()
	t.Run("no PK", func(t *testing.T) {
		t.Parallel()
		w, judge, _ := bootQ422(t, 30, 0, &q422Draws{}, nil)
		if got := w.questWindow(t, judge); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-01.htm")) {
			t.Fatalf("the Black Judge's window without a PK = %q, want 30981-01.htm", got)
		}
	})
	for _, tc := range []struct {
		level int
		page  string
		cond  string
	}{
		{20, "30981-03.htm", "2"},
		{30, "30981-04.htm", "3"},
		{40, "30981-05.htm", "4"},
		{41, "30981-06.htm", "5"},
	} {
		t.Run("level "+strconv.Itoa(tc.level), func(t *testing.T) {
			t.Parallel()
			w, judge, _ := bootQ422(t, tc.level, 1, &q422Draws{}, nil)
			if got := w.questWindow(t, judge); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-02.htm")) {
				t.Fatalf("level %d: the Black Judge's window with a PK = %q, want 30981-02.htm", tc.level, got)
			}
			got := w.bypass(t, "Quest "+q422Name+" Start")
			if !slices.Contains(got, w.page(t, "judge", q422Name, tc.page)) ||
				!slices.Contains(got, "S PlaySound type=0 file=ItemSound.quest_accept bind=0 obj=0 loc=0,0,0 delay=0") {
				t.Fatalf("level %d: the start = %q, want %s and the accept sound", tc.level, got, tc.page)
			}
			if vars := w.srv.QuestVars(t, w.player, q422Name); vars["<state>"] != "STARTED" || vars["<cond>"] != tc.cond {
				t.Fatalf("level %d: Q422 after the start = %v, want started at cond %s", tc.level, vars, tc.cond)
			}
		})
	}
}

// TestQ422PushkinForgesAndTheJudgeGivesTheManacles: at cond 14 with the
// manual and the materials, Pushkin takes them for the penitent's manacles
// (cond 15); the Black Judge takes those for the Sin Eater's manacles and
// keeps the player's level (cond 16). Pushkin then only says the manacles
// are made.
func TestQ422PushkinForgesAndTheJudgeGivesTheManacles(t *testing.T) {
	t.Parallel()
	w, judge, pushkin := bootQ422(t, 45, 3, &q422Draws{}, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q422Name, "<state>", val("STARTED")},
			journalRow{objID, q422Name, "<cond>", val("14")},
		)
		for id, n := range map[int32]int32{q422ManualOfManacles: 1, q422Silver: 10, q422Steel: 5, q422Adamantine: 2, q422Cokes: 10, q422Frame: 1} {
			srv.GiveItem(t, objID, id, n)
		}
	})
	if got := w.questWindow(t, pushkin); !slices.Contains(got, w.page(t, "pushkin", q422Name, "30300-01.htm")) {
		t.Fatalf("Pushkin's window with the materials = %q, want 30300-01.htm", got)
	}
	for id, n := range map[int32]int{q422ManualOfManacles: 0, q422Silver: 0, q422Steel: 0, q422Adamantine: 0, q422Cokes: 0, q422Frame: 0, q422QitemManacles: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the forging = %d, want %d", id, c, n)
		}
	}
	if got := w.questWindow(t, judge); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-10.htm")) {
		t.Fatalf("the Black Judge's window with the manacles = %q, want 30981-10.htm", got)
	}
	w.bypass(t, "Quest "+q422Name+" 30981-11.htm")
	if vars := w.srv.QuestVars(t, w.player, q422Name); vars["<cond>"] != "16" || vars["level"] != "45" {
		t.Fatalf("Q422 after the Black Judge = %v, want cond 16 and level 45", vars)
	}
	for id, n := range map[int32]int{q422QitemManacles: 0, q422PenitentManacles: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the Black Judge = %d, want %d", id, c, n)
		}
	}
	// The Sin Eater has not grown: the manacles' enchant is not past 45.
	if got := w.questWindow(t, judge); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-12.htm")) {
		t.Fatalf("the Black Judge's window before the Sin Eater grows = %q, want 30981-12.htm", got)
	}
	if got := w.questWindow(t, pushkin); !slices.Contains(got, w.page(t, "pushkin", q422Name, "30300-03.htm")) {
		t.Fatalf("Pushkin's window with the manacles = %q, want 30300-03.htm", got)
	}
}

// TestQ422GrownSinEaterTakesPKsAway: at cond 16 with manacles whose
// Sin Eater (enchant 46) has grown past the kept level 45, the Black
// Judge takes the manacles for the leftover ones and 1 + the draw (4) PKs
// away, showing the player its new count alone; the level kept becomes
// the player's. With the leftover manacles the player takes the
// manacles back; once the PK count is no more than what is taken away it
// is cleared and the repeatable quest ends.
func TestQ422GrownSinEaterTakesPKsAway(t *testing.T) {
	t.Parallel()
	draws := &q422Draws{draw: 4}
	w, judge, _ := bootQ422(t, 47, 7, draws, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q422Name, "<state>", val("STARTED")},
			journalRow{objID, q422Name, "<cond>", val("16")},
			journalRow{objID, q422Name, "level", val("45")},
		)
	})
	grow := func(enchant int32) {
		w.srv.RunScript(t, w.player, q422Name, func(s *script.Script, p *script.Player) {
			s.TakeItems(p, q422PenitentManacles, -1)
			s.GiveItemsEnchanted(p, q422PenitentManacles, 1, enchant)
		})
		w.srv.ReadQueued(t, w.srv.Client)
	}
	pk := func() []string {
		w.srv.Client.Send(encodeBypass("Quest " + q422Name + " Pk"))
		frames := w.srv.ReadQueued(t, w.srv.Client)
		if n := len(framesOf(frames, serverpackets.OpcodeUserInfo)); n != 1 {
			t.Fatalf("the Pk link sent %d UserInfo, want 1", n)
		}
		return scriptLines(t, frames)
	}

	grow(46)
	if got := w.questWindow(t, judge); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-13.htm")) {
		t.Fatalf("the Black Judge's window with the grown Sin Eater = %q, want 30981-13.htm", got)
	}
	if got, want := pk(), []string{
		itemMessage(302, q422PenitentManacles),
		itemMessage(54, q422LeftManacles),
		"S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0",
	}; !slices.Equal(got, want) {
		t.Fatalf("the first relief = %q, want %q", got, want)
	}
	if got := w.pkKills(t); got != 2 {
		t.Fatalf("PK kills after the first relief = %d, want 7 - 5", got)
	}
	if vars := w.srv.QuestVars(t, w.player, q422Name); vars["level"] != "47" {
		t.Fatalf("Q422 after the first relief = %v, want level 47", vars)
	}
	if !slices.Equal(draws.bounds, []int{10}) {
		t.Fatalf("draw bounds = %v, want [10]", draws.bounds)
	}

	// The leftover manacles: the player asks for the manacles again.
	w.bypass(t, "Quest "+q422Name+" 30981-17.htm")
	w.bypass(t, "Quest "+q422Name+" 30981-19.htm")
	w.bypass(t, "Quest "+q422Name+" 30981-11.htm")
	for id, n := range map[int32]int{q422LeftManacles: 0, q422PenitentManacles: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the manacles are taken back = %d, want %d", id, c, n)
		}
	}

	grow(48)
	w.questWindow(t, judge)
	if got := pk(); !slices.Contains(got, "S PlaySound type=0 file=ItemSound.quest_finish bind=0 obj=0 loc=0,0,0 delay=0") {
		t.Fatalf("the last relief = %q, want the finish sound", got)
	}
	if got := w.pkKills(t); got != 0 {
		t.Fatalf("PK kills after the last relief = %d, want 0", got)
	}
	if vars := w.srv.QuestVars(t, w.player, q422Name); vars != nil {
		t.Fatalf("Q422 after the last relief = %v, want no state", vars)
	}
	if c := w.srv.PlayerItemCount(t, w.player, q422LeftManacles); c != 1 {
		t.Fatalf("leftover manacles after the end = %d, want 1 (not a quest item)", c)
	}
}

// TestQ422QuitTakesTheQuestItems: at cond 16 without the manacles the
// Black Judge offers the quit, which takes every quest item but the
// leftover manacles and forgets the quest.
func TestQ422QuitTakesTheQuestItems(t *testing.T) {
	t.Parallel()
	w, judge, _ := bootQ422(t, 45, 3, &q422Draws{}, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q422Name, "<state>", val("STARTED")},
			journalRow{objID, q422Name, "<cond>", val("16")},
			journalRow{objID, q422Name, "level", val("45")},
		)
		srv.GiveItem(t, objID, q422VenomSac, 2)
		srv.GiveItem(t, objID, q422LeftManacles, 1)
	})
	if got := w.questWindow(t, judge); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-18.htm")) {
		t.Fatalf("the Black Judge's window without the manacles = %q, want 30981-18.htm", got)
	}
	got := w.bypass(t, "Quest "+q422Name+" Quit")
	if !slices.Contains(got, w.page(t, "judge", q422Name, "30981-20.htm")) {
		t.Fatalf("the quit = %q, want 30981-20.htm", got)
	}
	if vars := w.srv.QuestVars(t, w.player, q422Name); vars != nil {
		t.Fatalf("Q422 after the quit = %v, want no state", vars)
	}
	for id, n := range map[int32]int{q422VenomSac: 0, q422LeftManacles: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the quit = %d, want %d", id, c, n)
		}
	}
	// The leftover manacles let the player in again at cond 16.
	if got := w.questWindow(t, judge); !slices.Contains(got, w.page(t, "judge", q422Name, "30981-18.htm")) {
		t.Fatalf("the Black Judge's window after the quit = %q, want 30981-18.htm", got)
	}
	w.bypass(t, "Quest "+q422Name+" 30981-19.htm")
	if vars := w.srv.QuestVars(t, w.player, q422Name); vars["<state>"] != "STARTED" || vars["<cond>"] != "16" {
		t.Fatalf("Q422 after the return = %v, want started at cond 16", vars)
	}
}
