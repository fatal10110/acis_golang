package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q503"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// Pursuit Of Clan Ambition (Q503_PursuitOfClanAmbition) spawns NPCs the
// scenarios cannot show nor act on (#4020 item 5): the Blitz Wyrms a dwarf
// corpse calls, the witches Cleo calls, the Grave Keymaster and the
// Imperial Coffer. It also reads the clan's leader, who may be another
// character of the clan, and a scenario has no line for a clan member whose
// leader is in the world. These Boot tests play both; the scenarios play the
// leader's dialogs, items and drops.
const (
	q503Name = "Q503_PursuitOfClanAmbition"
	q503Path = "quest.Q503_PursuitOfClanAmbition"

	q503Clan  = 7503
	q503Chief = 900503

	q503Gustav    = 30760
	q503Martien   = 30645
	q503Kusto     = 30512
	q503Balthazar = 30764
	q503Eric      = 30868
	q503Cleo      = 30766
	q503Coffer    = 30765
	q503Athrea    = 30758
	q503Kalis     = 30759
	q503Fritz     = 30761
	q503Lutz      = 30762
	q503Kurtz     = 30763

	q503ThunderWyrm = 20243
	q503BlitzWyrm   = 27178
	q503GraveGuard  = 20668
	q503Keymaster   = 27179
	q503Gravekeeper = 27181

	q503MistDrakeEgg = 3839
	q503BlitzWyrmEgg = 3840
	q503ThunderEgg   = 3842
	q503Brooch       = 3843
	q503ImperialKey  = 3847
	q503Scepter      = 3869
	q503RecipeTitan  = 3838
	q503BrokenTitan  = 3845
	q503Seal         = 3870
)

// q503World is a Q503 world: Seeker, level 40, in a clan of level 4 led by
// itself (leader) or by Chief, who then enters the world on request.
type q503World struct {
	*dialogWorld
	leader   bool
	leaderID int32
	chief    *testsupport.ScriptedClient
	rolls    *scriptDraws
}

// q503Boot boots the world with the leader's quest STARTED at cond, holding
// holds, over the quest's shipped pages. Every NPC the quest binds stands
// as a template of the world's table, so the script may spawn it.
func q503Boot(t *testing.T, leader bool, cond int, vars map[string]string, holds map[int32]int32) *q503World {
	t.Helper()
	quest := q503.New()
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
	clan := strconv.Itoa(q503Clan)
	seed := []string{"UPDATE characters SET clanid = " + clan + " WHERE char_name = 'Seeker'"}
	if leader {
		seed = append(seed, "INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) SELECT "+clan+", 'Ambitious', 4, 0, obj_Id FROM characters WHERE char_name = 'Seeker'")
	} else {
		seed = append(seed,
			`DROP TEMPORARY TABLE IF EXISTS chief`,
			`CREATE TEMPORARY TABLE chief SELECT * FROM characters WHERE char_name = 'Seeker'`,
			"UPDATE chief SET obj_Id = "+strconv.Itoa(q503Chief)+", account_name = 'chief', char_name = 'Chief'",
			`INSERT INTO characters SELECT * FROM chief`,
			`DROP TEMPORARY TABLE chief`,
			"INSERT INTO clan_data (clan_id, clan_name, clan_level, reputation_score, leader_id) VALUES ("+clan+", 'Ambitious', 4, 0, "+strconv.Itoa(q503Chief)+")")
	}
	rolls := &scriptDraws{}
	w := &q503World{leader: leader, rolls: rolls}
	w.dialogWorld = bootSeamQuest(t, 40, chatPages(shippedQuestPages(t, q503Name), chat...), func(srv *gameservertest.Server, objID int32) {
		w.leaderID = objID
		if !leader {
			w.leaderID = q503Chief
		}
		rows := []journalRow{
			{w.leaderID, q503Name, "<state>", val("STARTED")},
			{w.leaderID, q503Name, "<cond>", val(strconv.Itoa(cond))},
		}
		for k, v := range vars {
			rows = append(rows, journalRow{w.leaderID, q503Name, k, val(v)})
		}
		insertJournal(t, srv.DB, rows...)
		for id, count := range holds {
			srv.GiveItem(t, w.leaderID, id, count)
		}
	},
		x1Items(append(quest.Items, q503RecipeTitan, q503BrokenTitan, q503Seal)),
		clanSeed(t, seed...),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q503Path}}, script.Catalog{q503Path: q503.New}),
		gameservertest.WithScriptRand(rolls.rand),
	)
	w.roles[w.player] = "player"
	return w
}

// enterChief brings Chief into the world beside Seeker.
func (w *q503World) enterChief(t *testing.T) {
	t.Helper()
	c := w.srv.DialClient(t, "chief", 1)
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	w.srv.ReadQueued(t, c)
	w.srv.ReadQueued(t, w.srv.Client)
	w.chief = c
	w.roles[q503Chief] = "chief"
}

// event sends the quest's bypass event as Seeker and renders the answer.
func (w *q503World) event(t *testing.T, event string) []string {
	t.Helper()
	return w.send(t, encodeBypass("Quest "+q503Name+" "+event))
}

// shows fails unless got shows the shipped quest page file of the NPC role.
func (w *q503World) shows(t *testing.T, step string, got []string, role, file string) {
	t.Helper()
	wantLines(t, step, got, w.page(t, role, q503Name, file))
}

// vars returns the leader's variables of the quest.
func (w *q503World) vars(t *testing.T) map[string]string {
	t.Helper()
	return questRows(t, w.srv, w.leaderID, q503Name)
}

// held checks the count of an item the leader holds.
func (w *q503World) held(t *testing.T, item int32, want int) {
	t.Helper()
	if n := w.srv.PlayerItemCount(t, w.leaderID, item); n != want {
		t.Fatalf("item %d held = %d, want %d", item, n, want)
	}
}

// count checks that count NPCs of id stand in the world and returns them.
func (w *q503World) count(t *testing.T, step string, id int32, count int) []*script.NPC {
	t.Helper()
	got := npcsOf(w.srv, id)
	if len(got) != count {
		t.Fatalf("%s: %d NPCs %d, want %d", step, len(got), id, count)
	}
	return got
}

// kill spawns a monster of id beside Seeker and has Seeker kill it, letting
// the dying hooks run.
func (w *q503World) kill(t *testing.T, id int32) []string {
	t.Helper()
	h := w.spawnHostile(t, "monster", int(id), "Monster", 20)
	killOnPlayerQueue(t, w.srv, w.player, h)
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// TestQ503CorpsesRewardOnceCallBlitzWyrmsAndLeave: the leader's asking of a
// dwarf corpse gives its eggs once and spends its flag; the flag spent, the
// corpse answers with Fritz's page and gives nothing (Lutz's flagless page
// is Fritz's too). Fritz and Lutz call two Blitz Wyrms of 180 s and leave 10 s
// later; Kurtz gives the brooch and the eggs and leaves at once, calling
// none.
func TestQ503CorpsesRewardOnceCallBlitzWyrmsAndLeave(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		id      int32
		flagged bool
		talk    string
		event   string
		page    string
		items   map[int32]int
		wyrms   int
	}{
		{"fritz", q503Fritz, true, "30761-01.htm", "30761-02.htm", "30761-02.htm", map[int32]int{q503BlitzWyrmEgg: 3}, 2},
		{"fritz flag spent", q503Fritz, false, "30761-01.htm", "30761-02.htm", "30761-03.htm", map[int32]int{q503BlitzWyrmEgg: 0}, 2},
		{"lutz", q503Lutz, true, "30762-01.htm", "30762-02.htm", "30762-02.htm", map[int32]int{q503BlitzWyrmEgg: 3, q503MistDrakeEgg: 4}, 2},
		{"lutz flag spent", q503Lutz, false, "30762-01.htm", "30762-02.htm", "30761-03.htm", map[int32]int{q503BlitzWyrmEgg: 0, q503MistDrakeEgg: 0}, 2},
		{"kurtz", q503Kurtz, true, "30763-01.htm", "30763-02.htm", "30763-02.htm", map[int32]int{q503Brooch: 1, q503MistDrakeEgg: 6}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vars := map[string]string{}
			if tc.flagged {
				vars[strconv.Itoa(int(tc.id))] = "true"
			}
			vars["30769"] = "true" // another flag, which stays
			w := q503Boot(t, true, 2, vars, nil)
			w.spawnFolk(t, "corpse", int(tc.id), 40, 0, 0)

			w.shows(t, "talk", w.talk(t, "corpse"), "corpse", tc.talk)
			got := w.event(t, tc.event)
			w.shows(t, "event", got, "corpse", tc.page)
			for item, n := range tc.items {
				w.held(t, item, n)
			}
			if _, ok := w.vars(t)[strconv.Itoa(int(tc.id))]; ok {
				t.Fatalf("the corpse's flag stays: %v", w.vars(t))
			}
			if w.vars(t)["30769"] != "true" {
				t.Fatalf("another corpse's flag changed: %v", w.vars(t))
			}

			w.count(t, "wyrms called", q503BlitzWyrm, tc.wyrms)
			if tc.wyrms == 0 {
				w.count(t, "Kurtz at once", tc.id, 0)
				return
			}
			w.count(t, "corpse after the call", tc.id, 1)
			w.advance(t, 9*time.Second)
			w.count(t, "corpse 9 s on", tc.id, 1)
			w.advance(t, 2*time.Second)
			w.count(t, "corpse 11 s on", tc.id, 0)
			w.count(t, "wyrms 11 s on", q503BlitzWyrm, tc.wyrms)
			w.advance(t, 170*time.Second)
			w.count(t, "wyrms 181 s on", q503BlitzWyrm, 0)
		})
	}
}

// TestQ503KurtzFlagSpentAnswersWithItsThirdPage: a spent flag shows Kurtz's
// third page, and the corpse stays.
func TestQ503KurtzFlagSpentAnswersWithItsThirdPage(t *testing.T) {
	t.Parallel()
	w := q503Boot(t, true, 2, nil, nil)
	w.spawnFolk(t, "corpse", q503Kurtz, 40, 0, 0)
	w.shows(t, "talk", w.talk(t, "corpse"), "corpse", "30763-03.htm")
	w.count(t, "corpse", q503Kurtz, 1)
}

// TestQ503CleoCallsTwoWitchesForFiveSeconds: Cleo's last page says her line,
// calls Athrea and Kalis at their spots for 5 s, and moves the quest to cond
// 9.
func TestQ503CleoCallsTwoWitchesForFiveSeconds(t *testing.T) {
	t.Parallel()
	w := q503Boot(t, true, 8, nil, nil)
	w.spawnFolk(t, "cleo", q503Cleo, 40, 0, 0)
	w.shows(t, "talk", w.talk(t, "cleo"), "cleo", "30766-02.htm")
	w.event(t, "30766-03.htm")
	got := w.event(t, "30766-04.htm")
	wantLines(t, "Cleo's line", got, say("cleo", q503Cleo, 50338))
	if c := w.vars(t)["<cond>"]; c != "9" {
		t.Fatalf("cond = %q, want 9", c)
	}
	for id, at := range map[int32]script.Loc{
		q503Athrea: {X: 160688, Y: 21296, Z: -3714, Heading: 32768},
		q503Kalis:  {X: 160690, Y: 21176, Z: -3712, Heading: 32768},
	} {
		loc := w.count(t, "witch", id, 1)[0].SpawnLocation()
		if loc.X != at.X || loc.Y != at.Y || loc.Heading != at.Heading {
			t.Fatalf("witch %d stands at %+v, want %+v", id, loc, at)
		}
	}
	w.advance(t, 4*time.Second)
	w.count(t, "Athrea 4 s on", q503Athrea, 1)
	w.advance(t, 2*time.Second)
	w.count(t, "Athrea 6 s on", q503Athrea, 0)
	w.count(t, "Kalis 6 s on", q503Kalis, 0)
}

// TestQ503GraveGuardsCallTheKeymasterByTheKilledCount: the leader's kills of
// Grave Guards at cond 10 with no scepter count in "killed"; from the 5th a
// roll under 50 calls the Keymaster and resets the count, the 10th calls it
// whatever the roll; a leader with the scepter counts none.
func TestQ503GraveGuardsCallTheKeymasterByTheKilledCount(t *testing.T) {
	t.Parallel()
	t.Run("rolls pass", func(t *testing.T) {
		t.Parallel()
		w := q503Boot(t, true, 10, nil, nil)
		for kill := 1; kill <= 4; kill++ {
			w.kill(t, q503GraveGuard)
			w.count(t, "before the 5th kill", q503Keymaster, 0)
			if got := w.vars(t)["killed"]; got != strconv.Itoa(kill) {
				t.Fatalf("killed after kill %d = %q", kill, got)
			}
		}
		w.kill(t, q503GraveGuard)
		w.count(t, "5th kill", q503Keymaster, 1)
		if got := w.vars(t)["killed"]; got != "0" {
			t.Fatalf("killed after the call = %q, want 0", got)
		}
	})
	t.Run("rolls fail until the 10th", func(t *testing.T) {
		t.Parallel()
		w := q503Boot(t, true, 10, map[string]string{"killed": "4"}, nil)
		for kill := 5; kill <= 9; kill++ {
			w.rolls.queue(50)
			w.kill(t, q503GraveGuard)
			w.count(t, "failed roll", q503Keymaster, 0)
			if got := w.vars(t)["killed"]; got != strconv.Itoa(kill) {
				t.Fatalf("killed after kill %d = %q", kill, got)
			}
		}
		w.rolls.queue(99)
		w.kill(t, q503GraveGuard)
		w.count(t, "10th kill", q503Keymaster, 1)
		if got := w.vars(t)["killed"]; got != "0" {
			t.Fatalf("killed after the call = %q, want 0", got)
		}
	})
	t.Run("scepter held", func(t *testing.T) {
		t.Parallel()
		w := q503Boot(t, true, 10, map[string]string{"killed": "9"}, map[int32]int32{q503Scepter: 1})
		w.kill(t, q503GraveGuard)
		w.count(t, "kill with the scepter", q503Keymaster, 0)
		if got := w.vars(t)["killed"]; got != "9" {
			t.Fatalf("killed = %q, want 9 still", got)
		}
	})
}

// TestQ503KeymasterGivesKeysAndGravekeeperCallsTheCoffer: the Keymaster's
// death gives the leader an Imperial key, up to six; the Gravekeeper's death
// calls the Imperial Coffer for 180 s where it fell, with the coffer's line;
// a leader with the scepter gets neither.
func TestQ503KeymasterGivesKeysAndGravekeeperCallsTheCoffer(t *testing.T) {
	t.Parallel()
	w := q503Boot(t, true, 10, nil, map[int32]int32{q503ImperialKey: 5})
	w.kill(t, q503Keymaster)
	w.held(t, q503ImperialKey, 6)
	w.kill(t, q503Keymaster)
	w.held(t, q503ImperialKey, 6)

	h := w.spawnHostile(t, "monster", q503Gravekeeper, "Monster", 20)
	killOnPlayerQueue(t, w.srv, w.player, h)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	coffer := w.count(t, "coffer", q503Coffer, 1)[0]
	w.roles[coffer.ObjectID()] = "coffer"
	wantLines(t, "coffer's line", w.render(t, frames), say("coffer", q503Coffer, 50339))
	if loc := coffer.SpawnLocation(); loc.X != w.at.X+20 || loc.Y != w.at.Y {
		t.Fatalf("coffer at %+v, want where the Gravekeeper fell, 20 east of %+v", loc, w.at)
	}
	w.advance(t, 170*time.Second)
	w.count(t, "coffer 170 s on", q503Coffer, 1)
	w.advance(t, 11*time.Second)
	w.count(t, "coffer 181 s on", q503Coffer, 0)
}

// TestQ503ScepterHolderGetsNeitherKeyNorCoffer: a leader holding the
// scepter is given no key by the Keymaster's death and no coffer by the
// Gravekeeper's.
func TestQ503ScepterHolderGetsNeitherKeyNorCoffer(t *testing.T) {
	t.Parallel()
	w := q503Boot(t, true, 10, nil, map[int32]int32{q503Scepter: 1})
	w.kill(t, q503Keymaster)
	w.held(t, q503ImperialKey, 0)
	w.kill(t, q503Gravekeeper)
	w.count(t, "no coffer", q503Coffer, 0)
}

// TestQ503MemberKillsPayTheLeaderWhoStandsNear: a clan member's kill of a
// Thunder Wyrm at cond 2 drops the egg to the clan leader, never to the
// member; it drops none when the leader is out of the NPC's range or out of
// the world.
func TestQ503MemberKillsPayTheLeaderWhoStandsNear(t *testing.T) {
	t.Parallel()
	t.Run("leader near", func(t *testing.T) {
		t.Parallel()
		w := q503Boot(t, false, 2, nil, nil)
		w.enterChief(t)
		w.kill(t, q503ThunderWyrm)
		w.held(t, q503ThunderEgg, 1)
		if n := w.srv.PlayerItemCount(t, w.player, q503ThunderEgg); n != 0 {
			t.Fatalf("the member holds %d eggs", n)
		}
	})
	t.Run("leader far", func(t *testing.T) {
		t.Parallel()
		w := q503Boot(t, false, 2, nil, nil)
		w.enterChief(t)
		w.srv.RunQuest(t, q503Chief, q503Name, func(_ *script.Quests, c *player.Character, _ *script.Script) {
			c.SetXYZ(w.at.X+3000, w.at.Y, w.at.Z)
		})
		w.kill(t, q503ThunderWyrm)
		w.held(t, q503ThunderEgg, 0)
	})
	t.Run("leader offline", func(t *testing.T) {
		t.Parallel()
		w := q503Boot(t, false, 2, nil, nil)
		w.kill(t, q503ThunderWyrm)
		w.enterChief(t)
		w.held(t, q503ThunderEgg, 0)
	})
}

// TestQ503MemberTalksAnswerByTheLeadersCond: a clan member whose leader is
// in the world is answered by the leader's cond; one whose leader is not
// gets Gustav's member page and the quest's no-quest page elsewhere.
func TestQ503MemberTalksAnswerByTheLeadersCond(t *testing.T) {
	t.Parallel()
	t.Run("leader in the world", func(t *testing.T) {
		t.Parallel()
		w := q503Boot(t, false, 1, nil, nil)
		w.enterChief(t)
		for role, id := range map[string]int{
			"gustav": q503Gustav, "martien": q503Martien, "kusto": q503Kusto, "balthazar": q503Balthazar,
			"eric": q503Eric, "cleo": q503Cleo, "coffer": q503Coffer, "athrea": q503Athrea, "kalis": q503Kalis,
		} {
			w.spawnFolk(t, role, id, 40, 0, 0)
		}
		for _, tc := range []struct {
			cond int
			role string
			page string
		}{
			{1, "martien", "30645-01.htm"},
			{2, "kusto", "30512-01a.htm"},
			{3, "gustav", "30760-11t.htm"},
			{4, "balthazar", "30764-01.htm"},
			{6, "gustav", "30760-15t.htm"},
			{7, "eric", "30868-01.htm"},
			{8, "cleo", "30766-01.htm"},
			{9, "eric", "30868-07.htm"},
			{9, "athrea", "30758-01.htm"},
			{9, "kalis", "30759-01.htm"},
			{10, "coffer", "30765-01.htm"},
			{11, "gustav", "30760-19t.htm"},
			{12, "gustav", "30760-24t.htm"},
		} {
			w.srv.RunQuest(t, q503Chief, q503Name, func(q *script.Quests, c *player.Character, sc *script.Script) {
				q.State(c, sc).SetCond(int32(tc.cond))
			})
			w.shows(t, tc.role+" at cond "+strconv.Itoa(tc.cond), w.talk(t, tc.role), tc.role, tc.page)
		}
	})
	t.Run("leader out of the world", func(t *testing.T) {
		t.Parallel()
		w := q503Boot(t, false, 3, nil, nil)
		w.spawnFolk(t, "gustav", q503Gustav, 40, 0, 0)
		w.spawnFolk(t, "kusto", q503Kusto, 60, 0, 0)
		w.shows(t, "Gustav", w.talk(t, "gustav"), "gustav", "30760-04t.htm")
		wantLines(t, "Kusto", w.talk(t, "kusto"),
			"S NpcHtmlMessage obj=kusto item=0 html="+scriptcontract.Quote(script.NoQuestMsg()))
	})
}
