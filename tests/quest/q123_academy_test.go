package quest

import (
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q118"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q123"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Leader and the Follower's academy branches
// (Q123_TheLeaderAndTheFollower onTalk, onAdvEvent "31961-10.htm",
// onMyDying with a sponsor) and the reads between it and To Lead and Be
// Led. A scenario cannot give the character a clan academy sponsor or play
// its sponsor (#4175), so these Boot tests pin them. The no-sponsor
// branches are scenarios.
const (
	q123Name    = "Q123_TheLeaderAndTheFollower"
	q123Path    = "quest.Q123_TheLeaderAndTheFollower"
	q123Newyear = 31961
	q123Lizard  = 27321
	q123Araneid = 27322
	q123Blood   = 8549
	q123Leg     = 8550
)

// q123World boots Seeker at level, Chief's academy apprentice, with the
// quest rows of rows (quest name to vars; none: not taken), Newyear and
// Pinter beside it, both quests registered. Chief holds crystals D-grade
// crystals and stays out of the world. Every script draw returns roll.
func q123World(t *testing.T, level int, rows map[string]map[string]string, holds map[int32]int32, crystals int32, roll *atomic.Int32) *dialogWorld {
	t.Helper()
	pages := questPages(q123Name, map[string][]string{
		"31961-01.htm": {"31961-03.htm"}, "31961-02.htm": nil, "31961-02a.htm": nil, "31961-02b.htm": nil,
		"31961-03.htm": nil, "31961-06.htm": nil, "31961-06d.htm": nil, "31961-06e.htm": nil, "31961-07.htm": nil,
		"31961-08.htm": {"31961-10.htm"}, "31961-09.htm": nil, "31961-10.htm": nil,
		"31961-11.htm": nil, "31961-11a.htm": nil, "31961-12.htm": nil, "31961-14.htm": nil,
		"31961-15.htm": nil,
	})
	for k, v := range questPages(q118Name, map[string][]string{
		"30298-01.htm": nil, "30298-02a.htm": nil, "30298-02b.htm": nil,
	}) {
		pages[k] = v
	}
	w := bootSeamQuest(t, level, chatPages(pages, q123Newyear, q118Pinter), func(srv *gameservertest.Server, objID int32) {
		var journal []journalRow
		for quest, vars := range rows {
			for k, v := range vars {
				journal = append(journal, journalRow{objID, quest, k, val(v)})
			}
		}
		insertJournal(t, srv.DB, journal...)
		for id, n := range holds {
			srv.GiveItem(t, objID, id, n)
		}
		if crystals > 0 {
			srv.GiveItem(t, q118Chief, q118Crystal, crystals)
		}
	},
		q118Academy(t),
		x1Items([]int32{q123Blood, q123Leg, q118Blood, q118Leg, q118Crystal}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q123Lizard), proofMonster(q123Araneid)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q123Newyear: script.KindFolk, q118Pinter: script.KindFolk, q123Lizard: script.KindHostile, q123Araneid: script.KindHostile},
			[]script.Listing{{Path: q118Path}, {Path: q123Path}},
			script.Catalog{q118Path: q118.New, q123Path: q123.New},
		),
		gameservertest.WithScriptRand(func(int) int { return int(roll.Load()) }),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "newyear", q123Newyear, 40, 0, 0)
	w.spawnFolk(t, "pinter", q118Pinter, -40, 0, 0)
	return w
}

// q123Own is the rows map of Seeker's The Leader and the Follower rows vars.
func q123Own(vars map[string]string) map[string]map[string]string {
	return map[string]map[string]string{q123Name: vars}
}

// wantQ123Rows fails unless Seeker's quest rows hold want.
func (w *dialogWorld) wantQ123Rows(t *testing.T, step string, want map[string]string) {
	t.Helper()
	got := questRows(t, w.srv, w.player, q123Name)
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: rows %v, want %s=%s", step, got, k, v)
		}
	}
}

// TestQ123ApprenticeTakesTheQuest: Newyear gives the quest to an academy
// apprentice above level 18 (cond 1, state 1).
func TestQ123ApprenticeTakesTheQuest(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	w := q123World(t, 19, nil, nil, 0, &roll)
	wantQ118Page(t, "level 19", w.talk(t, "newyear"), "31961-01.htm")
	wantQ118Page(t, "accept", w.send(t, encodeBypass("Quest "+q123Name+" 31961-03.htm")), "31961-03.htm")
	w.wantQ123Rows(t, "accept", started(1, "state", "1"))
}

// TestQ123ApprenticeTooLow: Newyear turns an academy apprentice of level
// 18 away.
func TestQ123ApprenticeTooLow(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	w := q123World(t, 18, nil, nil, 0, &roll)
	wantQ118Page(t, "level 18", w.talk(t, "newyear"), "31961-02.htm")
}

// TestQ123AndQ118ReadEachOther: an apprentice that took one of the two
// academy quests is turned away by the other's NPC, with one page while
// the quest it took runs and another once it is completed.
func TestQ123AndQ118ReadEachOther(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, took, npc string
		vars            map[string]string
		want            string
	}{
		{"q123 reads a started q118", q118Name, "newyear", started(1, "state", "1"), "31961-02b.htm"},
		{"q123 reads a completed q118", q118Name, "newyear", map[string]string{"<state>": "COMPLETED"}, "31961-02a.htm"},
		{"q118 reads a started q123", q123Name, "pinter", started(1, "state", "1"), "30298-02b.htm"},
		{"q118 reads a completed q123", q123Name, "pinter", map[string]string{"<state>": "COMPLETED"}, "30298-02a.htm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var roll atomic.Int32
			w := q123World(t, 19, map[string]map[string]string{tc.took: tc.vars}, nil, 0, &roll)
			wantQ118Page(t, "the other quest's npc", w.talk(t, tc.npc), tc.want)
		})
	}
}

// TestQ123ChosenArmorWaitsForTheSponsor: with its armor set chosen, an
// apprentice whose sponsor is out of the world is told to bring it; with
// the sponsor beside it, Newyear names the price of its set.
func TestQ123ChosenArmorWaitsForTheSponsor(t *testing.T) {
	t.Parallel()
	for stateEx, page := range map[int]string{1: "31961-06.htm", 2: "31961-06d.htm", 3: "31961-06e.htm"} {
		t.Run(strconv.Itoa(stateEx), func(t *testing.T) {
			t.Parallel()
			var roll atomic.Int32
			w := q123World(t, 19, q123Own(started(3+stateEx-1, "state", "2", "stateEx", strconv.Itoa(stateEx))), nil, 0, &roll)
			wantQ118Page(t, "sponsor away", w.talk(t, "newyear"), "31961-07.htm")
			enterChief(t, w.srv)
			wantQ118Page(t, "sponsor beside", w.talk(t, "newyear"), page)
		})
	}
}

// TestQ123SponsorPaysForTheArmor: the sponsor pays 922 D-grade crystals
// for the heavy set and 771 for the others, which moves its apprentice's
// quest to cond 6 (state 3) with the sound played to the apprentice; one
// crystal short, Newyear refuses and takes nothing. The apprentice then
// moves its quest to cond 7, and the sponsor's view follows it.
func TestQ123SponsorPaysForTheArmor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stateEx  int
		crystals int32
		want     string
		left     int
	}{
		{1, 921, "31961-11.htm", 921},
		{1, 922, "31961-10.htm", 0},
		{2, 770, "31961-11a.htm", 770},
		{3, 772, "31961-10.htm", 1},
	} {
		t.Run(strconv.Itoa(tc.stateEx)+"/"+strconv.Itoa(int(tc.crystals)), func(t *testing.T) {
			t.Parallel()
			var roll atomic.Int32
			cond := 2 + tc.stateEx
			w := q123World(t, 19, q123Own(started(cond, "state", "2", "stateEx", strconv.Itoa(tc.stateEx))), nil, tc.crystals, &roll)
			c := enterChief(t, w.srv)

			wantQ118Page(t, "sponsor's talk", w.talkAs(t, c, "newyear"), "31961-08.htm")
			c.Send(encodeBypass("Quest " + q123Name + " 31961-10.htm"))
			wantQ118Page(t, "payment", w.render(t, w.srv.ReadQueued(t, c)), tc.want)
			if n := w.srv.PlayerItemCount(t, q118Chief, q118Crystal); n != tc.left {
				t.Fatalf("sponsor's crystals %d, want %d", n, tc.left)
			}
			apprentice := w.render(t, w.srv.ReadQueued(t, w.srv.Client))
			if tc.want != "31961-10.htm" {
				w.wantQ123Rows(t, "refused", started(cond, "state", "2"))
				return
			}
			wantLines(t, "the apprentice's sound", apprentice, "S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0")
			w.wantQ123Rows(t, "paid", started(6, "state", "3"))
			wantQ118Page(t, "sponsor after paying", w.talkAs(t, c, "newyear"), "31961-12.htm")

			wantQ118Page(t, "apprentice after the payment", w.talk(t, "newyear"), "31961-15.htm")
			w.wantQ123Rows(t, "apprentice after the payment", started(7, "state", "4"))
			wantQ118Page(t, "sponsor at state 4", w.talkAs(t, c, "newyear"), "31961-14.htm")
		})
	}
}

// TestQ123SponsorWithoutItsApprentice: a sponsor whose apprentice stands
// 1500 away has no news of it.
func TestQ123SponsorWithoutItsApprentice(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	w := q123World(t, 19, q123Own(started(3, "state", "2", "stateEx", "1")), nil, 0, &roll)
	c := enterChief(t, w.srv)
	if !w.srv.RunScript(t, w.player, q123Name, func(_ *script.Script, p *script.Player) {
		p.InstantTeleportTo(script.Loc{X: w.at.X + 1500, Y: w.at.Y, Z: w.at.Z})
	}) {
		t.Fatal("teleport panicked")
	}
	w.srv.AdvanceUntil(t, "the apprentice stands 1500 away", func() bool {
		x, _, _ := w.srv.PlayerPosition(t, w.player)
		return x == w.at.X+1500
	})
	wantQ118Page(t, "sponsor alone", w.talkAs(t, c, "newyear"), "31961-09.htm")
}

// TestQ123LizardmenBleedForAnApprentice: with a sponsor, even out of the
// world, a Bruin lizardman kill at cond 1 drops a blood on a roll under
// 700000 of 1000000, and the tenth moves the quest to cond 2.
func TestQ123LizardmenBleedForAnApprentice(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	roll.Store(700000)
	w := q123World(t, 19, q123Own(started(1, "state", "1")), map[int32]int32{q123Blood: 9}, 0, &roll)

	w.killHere(t, q123Lizard)
	if n := w.srv.PlayerItemCount(t, w.player, q123Blood); n != 9 {
		t.Fatalf("bloods %d after a failed roll, want 9", n)
	}
	w.wantQ123Rows(t, "failed roll", started(1, "state", "1"))
	roll.Store(699999)
	w.killHere(t, q123Lizard)
	if n := w.srv.PlayerItemCount(t, w.player, q123Blood); n != 10 {
		t.Fatalf("bloods %d after the tenth, want 10", n)
	}
	w.wantQ123Rows(t, "tenth blood", started(2, "state", "1"))
}

// TestQ123AraneidLegsNeedTheSponsorBeside: at cond 7 the Picot araneid
// drops no leg while the sponsor is out of the world, the quest kept; with
// the sponsor beside, the eighth leg moves the quest to cond 8.
func TestQ123AraneidLegsNeedTheSponsorBeside(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	w := q123World(t, 19, q123Own(started(7, "state", "4", "stateEx", "1")), map[int32]int32{q123Leg: 7}, 0, &roll)

	w.killHere(t, q123Araneid)
	if n := w.srv.PlayerItemCount(t, w.player, q123Leg); n != 7 {
		t.Fatalf("legs %d with the sponsor away, want 7", n)
	}
	w.wantQ123Rows(t, "sponsor away", started(7, "state", "4"))
	enterChief(t, w.srv)
	w.killHere(t, q123Araneid)
	if n := w.srv.PlayerItemCount(t, w.player, q123Leg); n != 8 {
		t.Fatalf("legs %d with the sponsor beside, want 8", n)
	}
	w.wantQ123Rows(t, "eighth leg", started(8, "state", "4"))
}
