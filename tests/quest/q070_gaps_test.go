package quest

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q076"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q077"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The saga evidence the traced scenarios cannot give: the sixth tablet's
// defender answering from every state its fight can be in, the defender
// running out after the player's sixteen hits, and the Archon of Halisha two
// sagas share.

// sagaState runs fn on the saga state of objID, on its queue.
func sagaState(t *testing.T, w *dialogWorld, objID int32, fn func(st *script.QuestState)) {
	t.Helper()
	if !w.srv.RunScript(t, objID, q070Name, func(sc *script.Script, p *script.Player) { fn(sc.QuestState(p, q070Name)) }) {
		t.Fatal("the saga state hook did not return")
	}
	w.srv.ReadQueued(t, w.srv.Client)
}

// setSagaState puts the saga of objID at cond with attacks hits counted.
func setSagaState(t *testing.T, w *dialogWorld, objID int32, cond, attacks int) {
	t.Helper()
	sagaState(t, w, objID, func(st *script.QuestState) {
		st.SetCond(int32(cond))
		if attacks == 0 {
			st.Unset("attacks")
		} else {
			st.Set("attacks", strconv.Itoa(attacks))
		}
	})
}

// firstTalkAs clicks the NPC obj twice as the client c (the first selects,
// the second talks) and renders the answer.
func firstTalkAs(t *testing.T, w *dialogWorld, c *testsupport.ScriptedClient, obj int32) []string {
	t.Helper()
	var frames [][]byte
	for range 2 {
		c.Send(encodeTutorialAction(obj, w.at))
		frames = w.srv.ReadQueued(t, c)
	}
	return w.render(t, frames)
}

// TestQ070DefenderFirstTalkPages: another player's call of a busy sixth
// tablet gets 10-04; the defender of the sixth tablet answers
// its first talk from the cond of the talker's saga, whether its attacker
// still stands, whether the fight is the talker's own and whether the
// talker landed sixteen hits: 4-01 to 4-05 with the attacker standing (4-01
// the own fight so far, 4-02 another's, 4-04 and 4-05 once sixteen hits
// were landed), 4-03 and 4-06 / 4-07 once it left, 4-08 / 4-09 at cond 18.
// Another player's 4-07 offers 4-2, which hands over the sixth amulet with
// 4-011, takes the attacks and moves to cond 18, and leaves the defender.
func TestQ070DefenderFirstTalkPages(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 17, nil, gameservertest.WithScriptRand(func(int) int { return 0 }))
	w.spawnFolk(t, "tablet6", tablet6ID, -40, 0, 0)
	w.talk(t, "tablet6")
	wantLines(t, "10-1", w.sagaEvent(t, "10-1"), w.sagaPage(t, "tablet6", "10-02.htm"))
	defender := w.spawned(t, "defender", defenderID)
	attacker := w.spawned(t, "attacker", attackerID)
	mate, mateClient := sagaMate(t, w, 17, false)
	obj := defender.ObjectID()

	// The tablet is busy with the player's fight: another player's call
	// gets 10-04 and no second fight.
	mateWorld := &dialogWorld{srv: w.srv, player: mate, at: w.at, roles: w.roles, npcs: w.npcs}
	wantLines(t, "the mate's window", mateWorld.talkAs(t, mateClient, "tablet6"), w.sagaPage(t, "tablet6", "10-01.htm"))
	mateClient.Send(encodeBypass("Quest " + q070Name + " 10-1"))
	wantLines(t, "the mate's 10-1", w.render(t, w.srv.ReadQueued(t, mateClient)), w.sagaPage(t, "tablet6", "10-04.htm"))
	if n := len(npcsOf(w.srv, defenderID)) + len(npcsOf(w.srv, attackerID)); n != 2 {
		t.Fatalf("%d fight NPCs after the mate's 10-1, want the player's two", n)
	}

	own := func(step, page string) {
		t.Helper()
		wantLines(t, step, w.render(t, w.interact(t, obj)), w.sagaPage(t, "defender", page))
	}
	other := func(step, page string) {
		t.Helper()
		wantLines(t, step, firstTalkAs(t, w, mateClient, obj), w.sagaPage(t, "defender", page))
	}

	// The attacker stands.
	own("own fight", "4-01.htm")
	other("another's fight", "4-02.htm")
	setSagaState(t, w, w.player, 17, 16)
	setSagaState(t, w, mate, 17, 16)
	own("own fight, sixteen hits", "4-04.htm")
	other("another's fight, sixteen hits", "4-05.htm")
	setSagaState(t, w, w.player, 18, 0)
	setSagaState(t, w, mate, 18, 0)
	own("own fight, cond 18", "4-08.htm")
	other("another's fight, cond 18", "4-08.htm")

	// The player lands sixteen hits and the attacker leaves.
	setSagaState(t, w, w.player, 17, 0)
	setSagaState(t, w, mate, 17, 0)
	h := hostileNPC(t, w.srv, attacker)
	for range 16 {
		hit(t, w.srv, w.player, h, 10)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	if n := len(npcsOf(w.srv, attackerID)); n != 0 {
		t.Fatalf("%d attackers after sixteen hits, want 0", n)
	}
	own("own fight won", "4-06.htm")
	other("another's fight, no hits", "4-03.htm")
	setSagaState(t, w, mate, 17, 16)
	other("another's fight won", "4-07.htm")

	// 4-07 offers 4-2.
	mateClient.Send(encodeBypass("Quest " + q070Name + " 4-2"))
	wantLines(t, "4-2", w.render(t, w.srv.ReadQueued(t, mateClient)),
		"S SystemMessage id=54 3:7423", w.sagaPage(t, "defender", "4-011.htm"))
	if vars := w.srv.QuestVars(t, mate, q070Name); vars["<cond>"] != "18" || vars["attacks"] != "" {
		t.Fatalf("after 4-2: vars %v, want cond 18 and no attacks", vars)
	}
	if n := w.srv.PlayerItemCount(t, mate, amulet6th); n != 1 {
		t.Fatalf("sixth amulets %d after 4-2, want 1", n)
	}
	if n := len(npcsOf(w.srv, defenderID)); n != 1 {
		t.Fatalf("%d defenders after 4-2, want it to stay", n)
	}

	// Cond 18, the attacker gone.
	other("cond 18, the attacker gone", "4-09.htm")
	setSagaState(t, w, w.player, 18, 0)
	own("own fight, cond 18, the attacker gone", "4-09.htm")
}

// TestQ070SixthTabletFightWonRunsOut: a defender nobody sends away says its
// "won" line when its minute is up and the player it fought for landed more
// than fifteen hits at cond 17, not the "lost" one.
func TestQ070SixthTabletFightWonRunsOut(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 17, nil, gameservertest.WithScriptRand(func(int) int { return 0 }))
	w.spawnFolk(t, "tablet6", tablet6ID, -40, 0, 0)
	w.talk(t, "tablet6")
	wantLines(t, "10-1", w.sagaEvent(t, "10-1"), w.sagaPage(t, "tablet6", "10-02.htm"))
	w.spawned(t, "defender", defenderID)
	attacker := w.spawned(t, "attacker", attackerID)

	h := hostileNPC(t, w.srv, attacker)
	for range 16 {
		hit(t, w.srv, w.player, h, 10)
	}
	w.srv.ReadQueued(t, w.srv.Client)

	w.advance(t, 59*time.Second)
	got := w.advance(t, time.Second)
	wantLines(t, "60 s", got, say("defender", defenderID, 7061))
	if slices.Contains(got, say("defender", defenderID, 7062)) {
		t.Fatalf("60 s: %q holds the lost line", got)
	}
	if n := len(npcsOf(w.srv, defenderID)); n != 0 {
		t.Fatalf("%d defenders after 60 s, want 0", n)
	}
}

// TestSagasShareTheArchonOfHalisha: the Archon of Halisha 27226 belongs to
// the Saga of the Grand Khavatari and the Saga of the Dominator both; one
// killed by a player it was not woken for makes each saga say its own
// kill-other line, once each.
func TestSagasShareTheArchonOfHalisha(t *testing.T) {
	t.Parallel()
	const archonID = 27226
	family := thirdclass.NewFamily()
	paths := []string{"quest." + q076.Saga().Name, "quest." + q077.Saga().Name}
	w := bootSeamQuest(t, 76, nil, nil,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(archonID)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{archonID: script.KindHostile},
			[]script.Listing{{Path: paths[0]}, {Path: paths[1]}},
			script.Catalog{
				paths[0]: func() script.Script { return thirdclass.New(family, q076.Saga()) },
				paths[1]: func() script.Script { return thirdclass.New(family, q077.Saga()) },
			}))
	w.roles[w.player] = "player"
	archon := w.spawnHostile(t, "archon", archonID, "Monster", 40)

	killOnPlayerQueue(t, w.srv, w.player, archon)
	got := keep(w.render(t, w.srv.ReadQueued(t, w.srv.Client)), "S NpcSay ")
	want := []string{say("archon", archonID, 7655), say("archon", archonID, 7755)}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("lines %q, want one kill-other line per saga %q", got, want)
	}
}
