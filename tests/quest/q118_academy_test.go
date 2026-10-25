package quest

import (
	"context"
	"database/sql"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q118"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// To Lead and Be Led's academy branches (Q118_ToLeadAndBeLed onTalk,
// onAdvEvent "30298-10.htm", onMyDying with a sponsor). A scenario cannot
// give the character a clan academy sponsor or play its sponsor (#4175),
// so these Boot tests pin them. The no-sponsor branches are scenarios.
const (
	q118Name    = "Q118_ToLeadAndBeLed"
	q118Path    = "quest.Q118_ToLeadAndBeLed"
	q118Pinter  = 30298
	q118Lizard  = 20919
	q118Araneid = 20927
	q118Blood   = 8062
	q118Leg     = 8063
	q118Crystal = 1458
	q118Chief   = 900001
)

// q118Academy seeds clan 7001 led by Chief, Seeker's academy sponsor, a
// copy of Seeker standing where it does.
func q118Academy(t *testing.T) gameservertest.Option {
	return gameservertest.WithClanSeed(func(db *sql.DB) {
		ctx := context.Background()
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		for _, q := range []string{
			`DROP TEMPORARY TABLE IF EXISTS chief`,
			`CREATE TEMPORARY TABLE chief SELECT * FROM characters WHERE char_name = 'Seeker'`,
			`UPDATE chief SET obj_Id = 900001, account_name = 'chief', char_name = 'Chief', clanid = 7001, apprentice = (SELECT obj_Id FROM characters WHERE char_name = 'Seeker')`,
			`INSERT INTO characters SELECT * FROM chief`,
			`UPDATE characters SET clanid = 7001, sponsor = 900001 WHERE char_name = 'Seeker'`,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) VALUES (7001, 'Chiefs', 3, 100, 900001)`,
			`DROP TEMPORARY TABLE chief`,
		} {
			if _, err := conn.ExecContext(ctx, q); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	})
}

// q118World boots Seeker at level, Chief's academy apprentice, with its
// To Lead and Be Led rows vars (none: not taken) and Pinter beside it.
// Chief holds crystals D-grade crystals and stays out of the world. Every
// script draw returns roll.
func q118World(t *testing.T, level int, vars map[string]string, holds map[int32]int32, crystals int32, roll *atomic.Int32) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q118Name, map[string][]string{
		"30298-01.htm": {"30298-03.htm"}, "30298-02.htm": nil, "30298-03.htm": nil,
		"30298-06.htm": nil, "30298-06d.htm": nil, "30298-06e.htm": nil, "30298-07.htm": nil,
		"30298-08.htm": {"30298-10.htm"}, "30298-09.htm": nil, "30298-10.htm": nil,
		"30298-11.htm": nil, "30298-11a.htm": nil, "30298-12.htm": nil, "30298-14.htm": nil,
		"30298-15.htm": nil,
	}), q118Pinter)
	w := bootSeamQuest(t, level, pages, func(srv *gameservertest.Server, objID int32) {
		var rows []journalRow
		for k, v := range vars {
			rows = append(rows, journalRow{objID, q118Name, k, val(v)})
		}
		insertJournal(t, srv.DB, rows...)
		for id, n := range holds {
			srv.GiveItem(t, objID, id, n)
		}
		if crystals > 0 {
			srv.GiveItem(t, q118Chief, q118Crystal, crystals)
		}
	},
		q118Academy(t),
		x1Items([]int32{q118Blood, q118Leg, q118Crystal}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(q118Lizard), proofMonster(q118Araneid)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q118Pinter: script.KindFolk, q118Lizard: script.KindHostile, q118Araneid: script.KindHostile},
			[]script.Listing{{Path: q118Path}},
			script.Catalog{q118Path: q118.New},
		),
		gameservertest.WithScriptRand(func(int) int { return int(roll.Load()) }),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "pinter", q118Pinter, 40, 0, 0)
	return w
}

// started is the rows of a started quest at cond with vars.
func started(cond int, vars ...string) map[string]string {
	m := map[string]string{"<state>": "STARTED", "<cond>": strconv.Itoa(cond)}
	for i := 0; i+1 < len(vars); i += 2 {
		m[vars[i]] = vars[i+1]
	}
	return m
}

// wantQ118Page fails unless got shows the stand-in page file.
func wantQ118Page(t *testing.T, step string, got []string, file string) {
	t.Helper()
	if !shows(got, file) {
		t.Fatalf("%s: got %q, want the page %s", step, got, file)
	}
}

// wantQ118Rows fails unless Seeker's quest rows hold want.
func (w *dialogWorld) wantQ118Rows(t *testing.T, step string, want map[string]string) {
	t.Helper()
	got := questRows(t, w.srv, w.player, q118Name)
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: rows %v, want %s=%s", step, got, k, v)
		}
	}
}

// chiefTalk opens, as Chief, Pinter's quest window.
func (w *dialogWorld) chiefTalk(t *testing.T, c *testsupport.ScriptedClient) []string {
	t.Helper()
	return w.talkAs(t, c, "pinter")
}

// TestQ118ApprenticeTakesTheQuest: Pinter gives the quest to an academy
// apprentice above level 18 (cond 1, state 1).
func TestQ118ApprenticeTakesTheQuest(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	w := q118World(t, 19, nil, nil, 0, &roll)
	wantQ118Page(t, "level 19", w.talk(t, "pinter"), "30298-01.htm")
	wantQ118Page(t, "accept", w.send(t, encodeBypass("Quest "+q118Name+" 30298-03.htm")), "30298-03.htm")
	w.wantQ118Rows(t, "accept", started(1, "state", "1"))
}

// TestQ118ApprenticeTooLow: Pinter turns an academy apprentice of level 18
// away.
func TestQ118ApprenticeTooLow(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	w := q118World(t, 18, nil, nil, 0, &roll)
	wantQ118Page(t, "level 18", w.talk(t, "pinter"), "30298-02.htm")
}

// TestQ118ChosenArmorWaitsForTheSponsor: with its armor set chosen, an
// apprentice whose sponsor is out of the world is told to bring it; with
// the sponsor beside it, Pinter names the price of its set.
func TestQ118ChosenArmorWaitsForTheSponsor(t *testing.T) {
	t.Parallel()
	for stateEx, page := range map[int]string{1: "30298-06.htm", 2: "30298-06d.htm", 3: "30298-06e.htm"} {
		t.Run(strconv.Itoa(stateEx), func(t *testing.T) {
			t.Parallel()
			var roll atomic.Int32
			w := q118World(t, 19, started(3+stateEx-1, "state", "2", "stateEx", strconv.Itoa(stateEx)), nil, 0, &roll)
			wantQ118Page(t, "sponsor away", w.talk(t, "pinter"), "30298-07.htm")
			enterChief(t, w.srv)
			wantQ118Page(t, "sponsor beside", w.talk(t, "pinter"), page)
		})
	}
}

// TestQ118SponsorPaysForTheArmor: the sponsor pays 922 D-grade crystals
// for the heavy set and 771 for the others, which moves its apprentice's
// quest to cond 6 (state 3) with the sound played to the apprentice; one
// crystal short, Pinter refuses and takes nothing. The apprentice then
// moves its quest to cond 7, and the sponsor's view follows it.
func TestQ118SponsorPaysForTheArmor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stateEx  int
		crystals int32
		want     string
		left     int
	}{
		{1, 921, "30298-11.htm", 921},
		{1, 922, "30298-10.htm", 0},
		{2, 770, "30298-11a.htm", 770},
		{3, 772, "30298-10.htm", 1},
	} {
		t.Run(strconv.Itoa(tc.stateEx)+"/"+strconv.Itoa(int(tc.crystals)), func(t *testing.T) {
			t.Parallel()
			var roll atomic.Int32
			cond := 2 + tc.stateEx
			w := q118World(t, 19, started(cond, "state", "2", "stateEx", strconv.Itoa(tc.stateEx)), nil, tc.crystals, &roll)
			c := enterChief(t, w.srv)

			wantQ118Page(t, "sponsor's talk", w.chiefTalk(t, c), "30298-08.htm")
			c.Send(encodeBypass("Quest " + q118Name + " 30298-10.htm"))
			wantQ118Page(t, "payment", w.render(t, w.srv.ReadQueued(t, c)), tc.want)
			if n := w.srv.PlayerItemCount(t, q118Chief, q118Crystal); n != tc.left {
				t.Fatalf("sponsor's crystals %d, want %d", n, tc.left)
			}
			apprentice := w.render(t, w.srv.ReadQueued(t, w.srv.Client))
			if tc.want != "30298-10.htm" {
				w.wantQ118Rows(t, "refused", started(cond, "state", "2"))
				return
			}
			wantLines(t, "the apprentice's sound", apprentice, "S PlaySound type=0 file=ItemSound.quest_middle bind=0 obj=0 loc=0,0,0 delay=0")
			w.wantQ118Rows(t, "paid", started(6, "state", "3"))
			wantQ118Page(t, "sponsor after paying", w.chiefTalk(t, c), "30298-12.htm")

			wantQ118Page(t, "apprentice after the payment", w.talk(t, "pinter"), "30298-15.htm")
			w.wantQ118Rows(t, "apprentice after the payment", started(7, "state", "4"))
			wantQ118Page(t, "sponsor at state 4", w.chiefTalk(t, c), "30298-14.htm")
		})
	}
}

// TestQ118SponsorWithoutItsApprentice: a sponsor whose apprentice stands
// 1500 away has no news of it.
func TestQ118SponsorWithoutItsApprentice(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	w := q118World(t, 19, started(3, "state", "2", "stateEx", "1"), nil, 0, &roll)
	c := enterChief(t, w.srv)
	if !w.srv.RunScript(t, w.player, q118Name, func(_ *script.Script, p *script.Player) {
		p.InstantTeleportTo(script.Loc{X: w.at.X + 1500, Y: w.at.Y, Z: w.at.Z})
	}) {
		t.Fatal("teleport panicked")
	}
	w.srv.AdvanceUntil(t, "the apprentice stands 1500 away", func() bool {
		x, _, _ := w.srv.PlayerPosition(t, w.player)
		return x == w.at.X+1500
	})
	wantQ118Page(t, "sponsor alone", w.chiefTalk(t, c), "30298-09.htm")
}

// TestQ118LizardmenBleedForAnApprentice: with a sponsor, even out of the
// world, a Maille lizardman kill at cond 1 drops a blood on a roll under
// 700000 of 1000000, and the tenth moves the quest to cond 2.
func TestQ118LizardmenBleedForAnApprentice(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	roll.Store(700000)
	w := q118World(t, 19, started(1, "state", "1"), map[int32]int32{q118Blood: 9}, 0, &roll)

	w.killHere(t, q118Lizard)
	if n := w.srv.PlayerItemCount(t, w.player, q118Blood); n != 9 {
		t.Fatalf("bloods %d after a failed roll, want 9", n)
	}
	w.wantQ118Rows(t, "failed roll", started(1, "state", "1"))
	roll.Store(699999)
	w.killHere(t, q118Lizard)
	if n := w.srv.PlayerItemCount(t, w.player, q118Blood); n != 10 {
		t.Fatalf("bloods %d after the tenth, want 10", n)
	}
	w.wantQ118Rows(t, "tenth blood", started(2, "state", "1"))
}

// TestQ118AraneidLegsNeedTheSponsorBeside: at cond 7 the King of the
// Araneid drops no leg while the sponsor is out of the world, the quest
// kept; with the sponsor beside, the eighth leg moves the quest to cond 8.
func TestQ118AraneidLegsNeedTheSponsorBeside(t *testing.T) {
	t.Parallel()
	var roll atomic.Int32
	w := q118World(t, 19, started(7, "state", "4", "stateEx", "1"), map[int32]int32{q118Leg: 7}, 0, &roll)

	w.killHere(t, q118Araneid)
	if n := w.srv.PlayerItemCount(t, w.player, q118Leg); n != 7 {
		t.Fatalf("legs %d with the sponsor away, want 7", n)
	}
	w.wantQ118Rows(t, "sponsor away", started(7, "state", "4"))
	enterChief(t, w.srv)
	w.killHere(t, q118Araneid)
	if n := w.srv.PlayerItemCount(t, w.player, q118Leg); n != 8 {
		t.Fatalf("legs %d with the sponsor beside, want 8", n)
	}
	w.wantQ118Rows(t, "eighth leg", started(8, "state", "4"))
}

// killHere kills a fresh monster of id beside the player.
func (w *dialogWorld) killHere(t *testing.T, id int) {
	t.Helper()
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(id), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	killOnPlayerQueue(t, w.srv, w.player, h)
	w.srv.ReadQueued(t, w.srv.Client)
}
