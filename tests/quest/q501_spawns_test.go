package quest

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q501"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Proof Of Clan Alliance (Q501_ProofOfClanAlliance) is asked by the clan's
// members for the clan's leader: the shamans' herbs, the sacrifices at the
// Statue of Offering and the Boxes of Athrea go to a member who stands near,
// whichever place the leader is in. A scenario has no line for a clan member
// whose leader is in the world, nor for the boxes the quest spawns (#4020 item
// 5), nor for a party. These Boot tests play them; the scenarios play the
// leader's dialogs, items and drops.
const (
	q501Name = "Q501_ProofOfClanAlliance"
	q501Path = "quest.Q501_ProofOfClanAlliance"

	q501Clan  = 7501
	q501Chief = 900501

	q501Athrea = 30758
	q501Statue = 30757

	q501Vanor = 20685
	q501Harit = 20644
	q501Oel   = 20576

	q501HerbOfHarit = 3832
	q501HerbOfVanor = 3833
	q501Symbol      = 3837
	q501BloodOfEva  = 3835

	q501FirstBox = 27173
	q501LastBox  = 27177
)

// q501Spawns are the places of the boxes, in the order the quest spawns them.
var q501Spawns = [16][2]int{
	{102273, 103433},
	{102190, 103379},
	{102107, 103325},
	{102024, 103271},
	{102327, 103350},
	{102244, 103296},
	{102161, 103242},
	{102078, 103188},
	{102381, 103267},
	{102298, 103213},
	{102215, 103159},
	{102132, 103105},
	{102435, 103184},
	{102352, 103130},
	{102269, 103076},
	{102186, 103022},
}

// q501World is a Q501 world: Seeker, level 40, in a clan of level 3 led by
// itself (leader) or by Chief, who then enters the world on request.
type q501World struct {
	*dialogWorld
	leaderID int32
	chief    *testsupport.ScriptedClient
	rolls    *scriptDraws
}

// q501Boot boots the world with the leader's quest STARTED at cond, over the
// quest's shipped pages. Every NPC the quest binds stands as a template of
// the world's table, so the script may spawn it.
func q501Boot(t *testing.T, leader bool, cond int, vars map[string]string, holds map[int32]int32, extra ...gameservertest.Option) *q501World {
	t.Helper()
	quest := q501.New()
	var templates []*npc.Template
	kinds := map[int32]script.NPCKind{}
	var chat []int
	for _, id := range quest.Bind[script.EventTalked] {
		templates = append(templates, gameservertest.FolkTemplate("Folk", int(id)))
		kinds[id] = script.KindFolk
		chat = append(chat, int(id))
	}
	for _, id := range quest.Bind[script.EventMyDying] {
		templates = append(templates, proofMonster(int(id)))
		kinds[id] = script.KindHostile
	}
	clan := strconv.Itoa(q501Clan)
	seed := []string{"UPDATE characters SET clanid = " + clan + " WHERE char_name = 'Seeker'"}
	if leader {
		seed = append(seed, "INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) SELECT "+clan+", 'Allied', 3, 0, obj_Id FROM characters WHERE char_name = 'Seeker'")
	} else {
		seed = append(seed,
			`DROP TEMPORARY TABLE IF EXISTS chief`,
			`CREATE TEMPORARY TABLE chief SELECT * FROM characters WHERE char_name = 'Seeker'`,
			"UPDATE chief SET obj_Id = "+strconv.Itoa(q501Chief)+", account_name = 'chief', char_name = 'Chief'",
			`INSERT INTO characters SELECT * FROM chief`,
			`DROP TEMPORARY TABLE chief`,
			"INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) VALUES ("+clan+", 'Allied', 3, 0, "+strconv.Itoa(q501Chief)+")")
	}
	rolls := &scriptDraws{}
	w := &q501World{rolls: rolls}
	w.dialogWorld = bootSeamQuest(t, 40, chatPages(shippedQuestPages(t, q501Name), chat...), func(srv *gameservertest.Server, objID int32) {
		w.leaderID = objID
		if !leader {
			w.leaderID = q501Chief
		}
		rows := []journalRow{
			{w.leaderID, q501Name, "<state>", val("STARTED")},
			{w.leaderID, q501Name, "<cond>", val(strconv.Itoa(cond))},
		}
		for k, v := range vars {
			rows = append(rows, journalRow{w.leaderID, q501Name, k, val(v)})
		}
		insertJournal(t, srv.DB, rows...)
		for id, count := range holds {
			srv.GiveItem(t, objID, id, count)
		}
	}, append([]gameservertest.Option{
		x1Items(append(quest.Items, 3874, 3889)),
		clanSeed(t, seed...),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q501Path}}, script.Catalog{q501Path: q501.New}),
		gameservertest.WithScriptRand(rolls.rand),
	}, extra...)...)
	w.roles[w.player] = "player"
	return w
}

// enterChief brings Chief into the world beside Seeker, in Seeker's party
// when party is set.
func (w *q501World) enterChief(t *testing.T, party bool) {
	t.Helper()
	c := w.srv.DialClient(t, "chief", 1)
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	w.srv.ReadQueued(t, c)
	w.srv.ReadQueued(t, w.srv.Client)
	if party {
		w.srv.Client.Send(encodeJoinParty("Chief"))
		w.srv.ReadQueued(t, c)
		c.Send(encodeAnswerJoinParty(1))
		w.srv.ReadQueued(t, c)
		w.srv.ReadQueued(t, w.srv.Client)
	}
	w.chief = c
	w.roles[q501Chief] = "chief"
}

// moveChief puts Chief dx east of Seeker's place.
func (w *q501World) moveChief(t *testing.T, dx int) {
	t.Helper()
	w.srv.RunQuest(t, q501Chief, q501Name, func(_ *script.Quests, c *player.Character, _ *script.Script) {
		c.SetXYZ(w.at.X+dx, w.at.Y, w.at.Z)
	})
}

// event sends the quest's bypass event as Seeker and renders the answer.
func (w *q501World) event(t *testing.T, event string) []string {
	t.Helper()
	return w.send(t, encodeBypass("Quest "+q501Name+" "+event))
}

// shows fails unless got shows the shipped quest page file of the NPC role.
func (w *q501World) shows(t *testing.T, step string, got []string, role, file string) {
	t.Helper()
	wantLines(t, step, got, w.page(t, role, q501Name, file))
}

// vars returns the leader's variables of the quest.
func (w *q501World) vars(t *testing.T) map[string]string {
	t.Helper()
	return questRows(t, w.srv, w.leaderID, q501Name)
}

// held checks the count of an item Seeker holds.
func (w *q501World) held(t *testing.T, item int32, want int) {
	t.Helper()
	if n := w.srv.PlayerItemCount(t, w.player, item); n != want {
		t.Fatalf("item %d held = %d, want %d", item, n, want)
	}
}

// kill spawns a monster of id dx east of Seeker and has Seeker kill it, letting
// the dying hooks run.
func (w *q501World) kill(t *testing.T, id int32, dx int) []string {
	t.Helper()
	h := w.spawnHostile(t, "monster", int(id), "Monster", dx)
	killOnPlayerQueue(t, w.srv, w.player, h)
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// TestQ501HerbsDropToTheMemberWhoStandsNear: a clan member's kill of a shaman
// the leader waits for drops the herb to the member and spends the leader's
// flag, wherever the leader stands (even in another place of the world, or in
// the member's party); it drops nothing when the leader is out of the world,
// the member is out of the party range of the kill, or the flag is spent.
func TestQ501HerbsDropToTheMemberWhoStandsNear(t *testing.T) {
	t.Parallel()
	flags := map[string]string{"3832": "true", "3833": "true"}
	for _, tc := range []struct {
		name    string
		party   bool
		chief   bool // Chief is in the world
		far     bool // Chief stands out of the party range
		killAt  int
		vars    map[string]string
		dropped bool
	}{
		{"leader near", false, true, false, 20, flags, true},
		{"leader far", false, true, true, 20, flags, true},
		{"leader far in the party", true, true, true, 20, flags, true},
		{"leader offline", false, false, false, 20, flags, false},
		{"kill out of the member's range", false, true, false, 3000, flags, false},
		{"flag spent", false, true, false, 20, map[string]string{"3833": "true"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q501Boot(t, false, 3, tc.vars, nil)
			if tc.chief {
				w.enterChief(t, tc.party)
				if tc.far {
					w.moveChief(t, 3000)
				}
			}
			w.rolls.taken()
			w.kill(t, q501Harit, tc.killAt)

			want := 0
			if tc.dropped {
				want = 1
			}
			w.held(t, q501HerbOfHarit, want)
			if tc.chief {
				if n := w.srv.PlayerItemCount(t, w.leaderID, q501HerbOfHarit); n != 0 {
					t.Fatalf("the leader holds %d herbs", n)
				}
			}
			_, kept := w.vars(t)["3832"]
			if wantKept := tc.vars["3832"] != "" && !tc.dropped; kept != wantKept {
				t.Fatalf("the leader's flag after the kill: %v", w.vars(t))
			}
			if w.vars(t)["3833"] != tc.vars["3833"] {
				t.Fatalf("another herb's flag changed: %v", w.vars(t))
			}
			if tc.party {
				// Only Seeker stands near: a draw among one.
				if got := w.rolls.taken(); !slices.Contains(got, 1) {
					t.Fatalf("draw bounds %v, want the party draw among 1", got)
				}
			}
		})
	}
}

// TestQ501SacrificesPayTheMemberOnceAndTheDeadToo: a member's sacrifice at the
// Statue of Offering that survives (a draw below 40) pays a Symbol of Loyalty
// and counts for the leader once; the member who did not survive and is dead
// by the fourth second is paid the same, and the one still alive is paid
// nothing.
func TestQ501SacrificesPayTheMemberOnceAndTheDeadToo(t *testing.T) {
	t.Parallel()
	setup := func(t *testing.T) *q501World {
		w := q501Boot(t, false, 2, nil, nil)
		w.enterChief(t, false)
		w.spawnFolk(t, "statue", q501Statue, 40, 0, 0)
		w.shows(t, "talk", w.talk(t, "statue"), "statue", "30757-01.htm")
		return w
	}
	t.Run("survives", func(t *testing.T) {
		t.Parallel()
		w := setup(t)
		w.rolls.queue(0)
		w.shows(t, "sacrifice", w.event(t, "30757-03.htm"), "statue", "30757-04.htm")
		w.held(t, q501Symbol, 1)
		vars := w.vars(t)
		if vars["sacrificed"] != "1" || vars[strconv.Itoa(int(w.player))] != "true" {
			t.Fatalf("leader's rows after the sacrifice: %v", vars)
		}
		w.shows(t, "talk again", w.talk(t, "statue"), "statue", "30757-01b.htm")
	})
	t.Run("dies", func(t *testing.T) {
		t.Parallel()
		w := setup(t)
		w.rolls.queue(99)
		w.shows(t, "sacrifice", w.event(t, "30757-03.htm"), "statue", "30757-03.htm")
		c := onlineCharacter(t, w.srv, w.player)
		onPlayerQueue(t, w.srv, w.player, func() { c.Die(nil) })
		w.srv.Advance(t, 4*time.Second)
		w.held(t, q501Symbol, 1)
		w.srv.Advance(t, time.Second) // the timer's writes are stored on the next tick
		if vars := w.vars(t); vars["sacrificed"] != "1" || vars[strconv.Itoa(int(w.player))] != "true" {
			t.Fatalf("leader's rows after the death: %v", vars)
		}
	})
	t.Run("lives", func(t *testing.T) {
		t.Parallel()
		w := setup(t)
		w.rolls.queue(99)
		w.event(t, "30757-03.htm")
		w.srv.Advance(t, 4*time.Second)
		w.srv.Advance(t, time.Second)
		w.held(t, q501Symbol, 0)
		if _, ok := w.vars(t)["sacrificed"]; ok {
			t.Fatalf("leader's rows after the survivor: %v", w.vars(t))
		}
	})
}

// boxes returns the live boxes of Athrea by the x of their place.
func (w *q501World) boxes(t *testing.T) map[int]*script.NPC {
	t.Helper()
	got := map[int]*script.NPC{}
	for id := int32(q501FirstBox); id <= q501LastBox; id++ {
		for _, n := range npcsOf(w.srv, id) {
			x, _, _ := hostileNPC(t, w.srv, n).Position()
			got[x] = n
			w.roles[n.ObjectID()] = "box" + strconv.Itoa(x)
		}
	}
	return got
}

// askBoxes has the member ask Athrea (already standing) for the boxes, the
// draws picking the places of the special boxes (the first pass picks a place
// twice) and the kinds of the boxes (by position, cycling).
func (w *q501World) askBoxes(t *testing.T) {
	t.Helper()
	w.shows(t, "first request", w.talk(t, "athrea"), "athrea", "30758-01.htm")
	w.shows(t, "page 2", w.event(t, "30758-02.htm"), "athrea", "30758-02.htm")
	w.rolls.queue(0, 0, 1, 2, 0, 1, 2, 3)
	w.rolls.queue(0, 1, 2, 3, 4, 0, 1, 2, 3, 4, 0, 1, 2, 3, 4, 0)
	w.rolls.taken()
	w.shows(t, "boxes", w.event(t, "30758-03.htm"), "athrea", "30758-03.htm")
}

// atBoxes puts Seeker at the first place of the boxes, or back home.
func (w *q501World) atBoxes(t *testing.T, there bool) {
	t.Helper()
	x, y, z := w.at.X, w.at.Y, w.at.Z
	if there {
		x, y, z = q501Spawns[0][0], q501Spawns[0][1], -3512
	}
	w.srv.RunQuest(t, w.player, q501Name, func(_ *script.Quests, c *player.Character, _ *script.Script) {
		c.SetXYZ(x, y, z)
	})
}

// killBox has Seeker kill the box at place i and renders what it was sent.
func (w *q501World) killBox(t *testing.T, i int) []string {
	t.Helper()
	h := hostileNPC(t, w.srv, w.boxes(t)[q501Spawns[i][0]])
	killOnPlayerQueue(t, w.srv, w.player, h)
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// boxSays is the line of the box at place i that was opened.
func boxSays(i int) string {
	return say("box"+strconv.Itoa(q501Spawns[i][0]), int32(q501FirstBox+i%5), 50110)
}

// TestQ501AthreaSpawnsSixteenBoxesFourSpecial: asking Athrea for the boxes
// spawns one box of a drawn kind at each of the 16 places, marks four places
// drawn apart with the clan; an ask while boxes stand spawns none.
func TestQ501AthreaSpawnsSixteenBoxesFourSpecial(t *testing.T) {
	t.Parallel()
	w := q501Boot(t, false, 3, map[string]string{"3835": "true"}, nil)
	w.enterChief(t, false)
	w.spawnFolk(t, "athrea", q501Athrea, 40, 0, 0)
	w.askBoxes(t)

	want := slices.Repeat([]int{16}, 8)
	want = append(want, slices.Repeat([]int{5}, 16)...)
	if got := w.rolls.taken(); !slices.Equal(got, want) {
		t.Fatalf("draw bounds %v, want %v", got, want)
	}
	if vars := w.vars(t); vars["spawn"] != "true" || vars["boxes"] != "0" {
		t.Fatalf("leader's rows: %v", vars)
	}

	boxes := w.boxes(t)
	if len(boxes) != 16 {
		t.Fatalf("%d boxes stand, want 16", len(boxes))
	}
	for i, at := range q501Spawns {
		box, ok := boxes[at[0]]
		if !ok {
			t.Fatalf("no box at place %d", i)
		}
		if id, want := box.NpcID(), int32(q501FirstBox+i%5); id != want {
			t.Errorf("box %d is NPC %d, want %d", i, id, want)
		}
		wantValue := int32(0)
		if i < 4 {
			wantValue = q501Clan
		}
		if box.ScriptValue() != wantValue {
			t.Errorf("box %d script value = %d, want %d", i, box.ScriptValue(), wantValue)
		}
	}

	// A second ask, while boxes stand, spawns none and draws nothing.
	w.shows(t, "talk", w.talk(t, "athrea"), "athrea", "30758-05.htm")
	w.shows(t, "ask", w.event(t, "30758-07.htm"), "athrea", "30758-03a.htm")
	if got := w.rolls.taken(); len(got) != 0 {
		t.Fatalf("draws of the second ask: %v", got)
	}
	if n := len(w.boxes(t)); n != 16 {
		t.Fatalf("%d boxes stand after the second ask, want 16", n)
	}
}

// TestQ501SpecialBoxesCountForTheClanOnly: a member who stands near counts
// the special boxes of its clan as they die, each with a line of the box; the
// other boxes count nothing, and a member out of the party range counts
// nothing. Short of four counted, Athrea gives no Blood of Eva.
func TestQ501SpecialBoxesCountForTheClanOnly(t *testing.T) {
	t.Parallel()
	w := q501Boot(t, false, 3, map[string]string{"3835": "true"}, nil)
	w.enterChief(t, false)
	w.spawnFolk(t, "athrea", q501Athrea, 40, 0, 0)
	w.askBoxes(t)

	// Seeker stands where the world put it, out of the range of the boxes.
	if got := w.killBox(t, 3); slices.Contains(got, boxSays(3)) || w.vars(t)["boxes"] != "0" {
		t.Fatalf("a kill out of range counted: %v %v", got, w.vars(t))
	}

	w.atBoxes(t, true)
	for i, count := range []string{"1", "2"} {
		if got := w.killBox(t, i); !slices.Contains(got, boxSays(i)) || w.vars(t)["boxes"] != count {
			t.Fatalf("special box %d: %v, rows %v", i, got, w.vars(t))
		}
	}
	// A plain box counts nothing and says nothing.
	if got := w.killBox(t, 5); slices.Contains(got, boxSays(5)) || w.vars(t)["boxes"] != "2" {
		t.Fatalf("plain box: %v, rows %v", got, w.vars(t))
	}
	w.atBoxes(t, false)
	w.shows(t, "two counted", w.talk(t, "athrea"), "athrea", "30758-05.htm")
	w.held(t, q501BloodOfEva, 0)
}

// TestQ501FourSpecialBoxesPayTheBloodOfEvaOnce: with the four special boxes
// counted Athrea gives the Blood of Eva to the member and spends the request.
func TestQ501FourSpecialBoxesPayTheBloodOfEvaOnce(t *testing.T) {
	t.Parallel()
	w := q501Boot(t, false, 3, map[string]string{"3835": "true"}, nil)
	w.enterChief(t, false)
	w.spawnFolk(t, "athrea", q501Athrea, 40, 0, 0)
	w.askBoxes(t)

	w.atBoxes(t, true)
	for i := range 4 {
		w.killBox(t, i)
	}
	if w.vars(t)["boxes"] != "4" {
		t.Fatalf("rows: %v", w.vars(t))
	}
	w.atBoxes(t, false)
	w.shows(t, "four counted", w.talk(t, "athrea"), "athrea", "30758-08.htm")
	w.held(t, q501BloodOfEva, 1)
	vars := w.vars(t)
	for _, k := range []string{"3835", "boxes", "spawn"} {
		if _, ok := vars[k]; ok {
			t.Fatalf("leader's rows after the Blood of Eva: %v", vars)
		}
	}
	w.shows(t, "request spent", w.talk(t, "athrea"), "athrea", "30758-09.htm")
	w.held(t, q501BloodOfEva, 1)
}

// TestQ501DespawnedBoxesLetAthreaSpawnThemAgain: the boxes leave the world
// after 300 s; with none left Athrea takes 10000 adena for another try (a
// member short of it is turned away) and the next ask spawns the boxes anew.
func TestQ501DespawnedBoxesLetAthreaSpawnThemAgain(t *testing.T) {
	t.Parallel()
	w := q501Boot(t, false, 3, map[string]string{"3835": "true"}, map[int32]int32{57: 10000})
	w.enterChief(t, false)
	w.spawnFolk(t, "athrea", q501Athrea, 40, 0, 0)
	w.askBoxes(t)

	w.srv.Advance(t, 299*time.Second)
	if n := len(w.boxes(t)); n != 16 {
		t.Fatalf("%d boxes stand after 299 s, want 16", n)
	}
	w.srv.Advance(t, 2*time.Second)
	if n := len(w.boxes(t)); n != 0 {
		t.Fatalf("%d boxes stand after 301 s, want 0", n)
	}

	w.shows(t, "talk", w.talk(t, "athrea"), "athrea", "30758-05.htm")
	w.held(t, 57, 10000)
	w.shows(t, "talk again", w.talk(t, "athrea"), "athrea", "30758-05.htm")
	w.shows(t, "adena", w.event(t, "30758-07.htm"), "athrea", "30758-07.htm")
	w.held(t, 57, 0)

	w.rolls.queue(0, 1, 2, 3)
	w.rolls.taken()
	w.shows(t, "boxes", w.event(t, "30758-03.htm"), "athrea", "30758-03.htm")
	if n := len(w.boxes(t)); n != 16 {
		t.Fatalf("%d boxes stand after the new ask, want 16", n)
	}
}

// TestQ501AthreaTurnsAwayAMemberShortOfAdena: with no box standing, a member
// short of the 10000 adena of another try is turned away and keeps what it has.
func TestQ501AthreaTurnsAwayAMemberShortOfAdena(t *testing.T) {
	t.Parallel()
	w := q501Boot(t, false, 3, map[string]string{"3835": "true", "spawn": "true", "boxes": "0"}, map[int32]int32{57: 9999})
	w.enterChief(t, false)
	w.spawnFolk(t, "athrea", q501Athrea, 40, 0, 0)
	w.shows(t, "talk", w.talk(t, "athrea"), "athrea", "30758-05.htm")
	w.shows(t, "short", w.event(t, "30758-07.htm"), "athrea", "30758-06.htm")
	w.held(t, 57, 9999)
}
