package quest

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q070"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// What the Saga of the Phoenix Knight scenarios cannot show: the NPCs the
// saga core spawns (the corrupted knight at the third tablet, the Archon
// of Halisha in the Shrine of Loyalty, the defender and its attacker at
// the sixth tablet), their lines, the fights they keep, the party credit
// of the shrine and the class the saga ends in (ThirdClassQuest.java,
// Q070_SagaOfThePhoenixKnight.java). The saga's spawn spots are moved
// beside the player, so its client sees the spawned NPCs speak.

const (
	q070Name      = "Q070_SagaOfThePhoenixKnight"
	q070Path      = "quest." + q070Name
	sagaCorePath  = "quest.ThirdClassQuest"
	knightClass   = 4
	paladinClass  = 5
	phoenixKnight = 90

	tablet3ID   = 31650
	tablet6ID   = 31657
	sedrickID   = 30849
	defenderID  = 31631
	corruptedID = 27286
	attackerID  = 27278
	halishaID   = 27219
	scarabID    = 21646

	amulet3rd   = 7330
	amulet5th   = 7392
	amulet6th   = 7423
	halishaMark = 7485
)

// sagaAt is a spawn spot of the test saga: dx, dy from where the fixture
// character enters the world.
func sagaAt(dx, dy int) script.Loc {
	return script.Loc{X: 10 + dx, Y: 20 + dy, Z: gameservertest.SpawnZ, Heading: -1}
}

// testSaga is the Phoenix Knight saga with its spawn spots beside the
// player.
func testSaga() thirdclass.Saga {
	sg := q070.Saga()
	sg.LocCorrupted = sagaAt(60, 0)
	sg.LocDefender = sagaAt(0, 60)
	sg.LocAttacker = sagaAt(0, 90)
	return sg
}

// sagaClassTemplates are the class templates of a Paladin's saga: its
// parent's, its own and the Phoenix Knight's.
func sagaClassTemplates() []*player.Template {
	var out []*player.Template
	for _, id := range []int{knightClass, paladinClass, phoenixKnight} {
		tmpl := gameservertest.ClassTemplate()
		tmpl.ID = id
		out = append(out, tmpl)
	}
	return out
}

// sagaPlayer makes objID a Paladin whose saga is started at cond, before
// it is selected.
func sagaPlayer(t *testing.T, srv *gameservertest.Server, objID int32, cond int) {
	t.Helper()
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET classid = ?, base_class = ? WHERE obj_Id = ?",
		paladinClass, paladinClass, objID); err != nil {
		t.Fatal(err)
	}
	insertJournal(t, srv.DB,
		journalRow{objID, q070Name, "<state>", val("STARTED")},
		journalRow{objID, q070Name, "<cond>", val(strconv.Itoa(cond))},
	)
}

// bootSaga boots a level 76 Paladin whose saga is started at cond, holding
// items (id to count), with the test saga and the saga core registered.
func bootSaga(t *testing.T, cond int, items map[int32]int32, opts ...gameservertest.Option) *dialogWorld {
	t.Helper()
	family := thirdclass.NewFamily()
	sg := testSaga()
	kinds := map[int32]script.NPCKind{
		tablet3ID: script.KindFolk, tablet6ID: script.KindFolk, sedrickID: script.KindFolk, defenderID: script.KindFolk,
		corruptedID: script.KindHostile, attackerID: script.KindHostile, halishaID: script.KindHostile, scarabID: script.KindHostile,
	}
	w := bootSeamQuest(t, 76, chatPages(shippedQuestPages(t, q070Name), tablet3ID, tablet6ID, sedrickID, defenderID),
		func(srv *gameservertest.Server, objID int32) {
			sagaPlayer(t, srv, objID, cond)
			for id, n := range items {
				srv.GiveItem(t, objID, id, n)
			}
		},
		append([]gameservertest.Option{
			gameservertest.WithClassTemplates(sagaClassTemplates()...),
			x1Items([]int32{7093, amulet3rd, amulet5th, amulet6th, halishaMark, 6622}),
			gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
				proofMonster(corruptedID), proofMonster(attackerID), proofMonster(halishaID), proofMonster(scarabID),
				gameservertest.FolkTemplate("Folk", tablet3ID), gameservertest.FolkTemplate("Folk", tablet6ID),
				gameservertest.FolkTemplate("Folk", sedrickID), gameservertest.FolkTemplate("Folk", defenderID),
			})),
			gameservertest.WithNpcSpawns(nil),
			gameservertest.WithNPCScripts(kinds,
				[]script.Listing{{Path: q070Path}, {Path: sagaCorePath}},
				script.Catalog{
					q070Path:     func() script.Script { return thirdclass.New(family, sg) },
					sagaCorePath: func() script.Script { return thirdclass.NewCore(family) },
				}),
		}, opts...)...)
	w.roles[w.player] = "player"
	return w
}

// sagaMate brings a second Paladin into the world beside the player, its
// saga started at cond; with party, it joins the player's party. It
// returns the mate's object id and client.
func sagaMate(t *testing.T, w *dialogWorld, cond int, party bool) (int32, *testsupport.ScriptedClient) {
	t.Helper()
	srv := w.srv
	mate := srv.SeedCharacterFor(t, "mate", "Mate", 76, 0).ID
	sagaPlayer(t, srv, mate, cond)
	c := srv.DialClient(t, "mate", 1)
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, c)
	srv.ReadQueued(t, srv.Client)
	if party {
		srv.Client.Send(encodeJoinParty("Mate"))
		srv.ReadQueued(t, c)
		c.Send(encodeAnswerJoinParty(1))
		srv.ReadQueued(t, c)
		srv.ReadQueued(t, srv.Client)
	}
	w.roles[mate] = "mate"
	return mate, c
}

// render renders the frames of interest among frames as the goldens write
// packets: pages, messages, sounds, quest windows, casts and NPC lines.
func (w *dialogWorld) render(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound,
			serverpackets.OpcodeQuestList, serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeNpcSay:
			line, err := scriptcontract.Packet(f, w.roles)
			if err != nil {
				t.Fatalf("frame %x: %v", f, err)
			}
			out = append(out, line)
		}
	}
	return out
}

// send sends a client packet and renders what the player is answered.
func (w *dialogWorld) send(t *testing.T, packet []byte) []string {
	t.Helper()
	w.srv.Client.Send(packet)
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// talk opens the quest window of the NPC role and renders it.
func (w *dialogWorld) talk(t *testing.T, role string) []string {
	t.Helper()
	obj := w.npcs[role]
	w.interact(t, obj)
	return w.send(t, encodeBypass("npc_"+strconv.Itoa(int(obj))+"_Quest"))
}

// talkAs opens, as the client c, the quest window of the NPC role and
// renders it.
func (w *dialogWorld) talkAs(t *testing.T, c *testsupport.ScriptedClient, role string) []string {
	t.Helper()
	obj := w.npcs[role]
	for range 2 {
		c.Send(encodeTutorialAction(obj, w.at))
		w.srv.ReadQueued(t, c)
	}
	c.Send(encodeBypass("npc_" + strconv.Itoa(int(obj)) + "_Quest"))
	return w.render(t, w.srv.ReadQueued(t, c))
}

// sagaEvent sends the saga's bypass event and renders the answer.
func (w *dialogWorld) sagaEvent(t *testing.T, event string) []string {
	t.Helper()
	return w.send(t, encodeBypass("Quest "+q070Name+" "+event))
}

// advance lets d pass and renders what the player was sent meanwhile.
func (w *dialogWorld) advance(t *testing.T, d time.Duration) []string {
	t.Helper()
	w.srv.Advance(t, d)
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// say is the rendered NpcSay of the NPC role, of template npcID, saying
// the NpcString id with params.
func say(role string, npcID int32, id int32, params ...any) string {
	return "S NpcSay obj=" + role + " type=0 npc=" + strconv.Itoa(int(1000000+npcID)) + " text=" + scriptcontract.Quote(script.NpcString(id, params...))
}

// sagaPage is the rendered saga page file shown by the NPC role.
func (w *dialogWorld) sagaPage(t *testing.T, role, file string) string {
	t.Helper()
	return w.page(t, role, q070Name, file)
}

// spawned returns the one NPC of id in the world, named role.
func (w *dialogWorld) spawned(t *testing.T, role string, id int32) *script.NPC {
	t.Helper()
	found := npcsOf(w.srv, id)
	if len(found) != 1 {
		t.Fatalf("%d NPCs %d in the world, want 1", len(found), id)
	}
	n := found[0]
	w.roles[n.ObjectID()], w.npcs[role] = role, n.ObjectID()
	return n
}

// hostileNPC returns the hostile NPC behind n.
func hostileNPC(t *testing.T, srv *gameservertest.Server, n *script.NPC) *npc.Hostile {
	t.Helper()
	for _, obj := range srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.ObjectID() == n.ObjectID() {
			return h
		}
	}
	t.Fatalf("no hostile NPC %d in the world", n.ObjectID())
	return nil
}

// hit lands a hit of dmg of the player objID on h.
func hit(t *testing.T, srv *gameservertest.Server, objID int32, h *npc.Hostile, dmg int) {
	t.Helper()
	p := playerCombatant(t, srv, objID)
	onPlayerQueue(t, srv, objID, func() { h.TakeDamage(dmg, p) })
}

// wantLines fails unless got holds want, in order, among other lines.
func wantLines(t *testing.T, step string, got []string, want ...string) {
	t.Helper()
	i := 0
	for _, l := range got {
		if i < len(want) && l == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Fatalf("%s: got %q, want in order %q", step, got, want)
	}
}

// saysNone fails when got holds a line of an NPC.
func saysNone(t *testing.T, step string, got []string) {
	t.Helper()
	if slices.ContainsFunc(got, func(l string) bool { return len(l) > 9 && l[:9] == "S NpcSay " }) {
		t.Fatalf("%s: got %q, want no NPC line", step, got)
	}
}

// TestQ070CorruptedKnightIsTheTabletsPrivate: the third tablet wakes the
// corrupted knight at its spot as the tablet's private, its spawn
// parameters the player, the player and the tablet; half a second later
// it calls the player out. While it stands the tablet tells the player the
// fight is on. The player's kill gives the third amulet and the knight's
// last words.
func TestQ070CorruptedKnightIsTheTabletsPrivate(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 8, nil)
	tablet := w.spawnFolk(t, "tablet3", tablet3ID, -40, 0, 0)

	wantLines(t, "window", w.talk(t, "tablet3"), w.sagaPage(t, "tablet3", "7-01.htm"))
	wantLines(t, "7-1", w.sagaEvent(t, "7-1"), w.sagaPage(t, "tablet3", "7-02.htm"))
	knight := w.spawned(t, "corrupted", corruptedID)
	if m := knight.Master(); m == nil || m.ObjectID() != tablet.ObjectID() {
		t.Fatalf("the knight's master is %v, want the tablet %d", m, tablet.ObjectID())
	}
	params := []int32{knight.Int(script.SlotParam1), knight.Int(script.SlotParam2), knight.Int(script.SlotParam3)}
	if want := []int32{w.player, w.player, tablet.ObjectID()}; !slices.Equal(params, want) {
		t.Fatalf("the knight's spawn parameters %v, want %v", params, want)
	}
	if knight.ScriptValue() != w.player {
		t.Fatalf("the knight's script value %d, want the player %d", knight.ScriptValue(), w.player)
	}
	if at := script.LocOf(knight); at.X != 70 || at.Y != 20 {
		t.Fatalf("the knight stands at (%d, %d), want its spot (70, 20)", at.X, at.Y)
	}
	wantLines(t, "spawn line", w.advance(t, 500*time.Millisecond),
		say("corrupted", corruptedID, 7050, "Seeker"))

	wantLines(t, "window again", w.talk(t, "tablet3"), w.sagaPage(t, "tablet3", "7-01.htm"))
	wantLines(t, "7-1 again", w.sagaEvent(t, "7-1"), w.sagaPage(t, "tablet3", "7-03.htm"))
	if n := len(npcsOf(w.srv, corruptedID)); n != 1 {
		t.Fatalf("%d knights after the second 7-1, want 1", n)
	}

	killOnPlayerQueue(t, w.srv, w.player, hostileNPC(t, w.srv, knight))
	wantLines(t, "kill", w.render(t, w.srv.ReadQueued(t, w.srv.Client)),
		say("corrupted", corruptedID, 7052))
	if vars := w.srv.QuestVars(t, w.player, q070Name); vars["<cond>"] != "9" {
		t.Fatalf("after the kill: vars %v, want cond 9", vars)
	}
	if n := w.srv.PlayerItemCount(t, w.player, amulet3rd); n != 1 {
		t.Fatalf("third amulets %d, want 1", n)
	}
}

// TestQ070CorruptedKnightLeavesWhenAnotherStrikesIt: another player of the
// saga finds the tablet busy with a fight not its own; its hit sends the
// knight away at once, with no line, and frees the tablet, which wakes a
// new knight for the player.
func TestQ070CorruptedKnightLeavesWhenAnotherStrikesIt(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 8, nil)
	w.spawnFolk(t, "tablet3", tablet3ID, -40, 0, 0)
	mate, mateClient := sagaMate(t, w, 8, false)

	w.talk(t, "tablet3")
	wantLines(t, "7-1", w.sagaEvent(t, "7-1"), w.sagaPage(t, "tablet3", "7-02.htm"))
	knight := w.spawned(t, "corrupted", corruptedID)
	w.advance(t, time.Second)

	mateWorld := &dialogWorld{srv: w.srv, player: mate, at: w.at, roles: w.roles, npcs: w.npcs}
	wantLines(t, "the mate's window", mateWorld.talkAs(t, mateClient, "tablet3"), w.sagaPage(t, "tablet3", "7-01.htm"))
	mateClient.Send(encodeBypass("Quest " + q070Name + " 7-1"))
	wantLines(t, "the mate's 7-1", w.render(t, w.srv.ReadQueued(t, mateClient)), w.sagaPage(t, "tablet3", "7-04.htm"))

	hit(t, w.srv, mate, hostileNPC(t, w.srv, knight), 10)
	if n := len(npcsOf(w.srv, corruptedID)); n != 0 {
		t.Fatalf("%d knights after the mate's hit, want 0", n)
	}
	saysNone(t, "the mate's hit", w.advance(t, time.Second))

	w.talk(t, "tablet3")
	wantLines(t, "7-1 after", w.sagaEvent(t, "7-1"), w.sagaPage(t, "tablet3", "7-02.htm"))
	if n := len(npcsOf(w.srv, corruptedID)); n != 1 {
		t.Fatalf("%d knights after the tablet was freed, want 1", n)
	}
}

// TestQ070ArchonOfHalishaWakesAtSevenHundredMarks: with 700 Marks of
// Halisha, a Shrine of Loyalty death takes 20 of them and wakes the
// player's Archon of Halisha where the monster fell, hating the player
// and calling it out by name. The player's kill of it gives the fifth
// amulet, takes every mark and frees its spirit.
func TestQ070ArchonOfHalishaWakesAtSevenHundredMarks(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 15, map[int32]int32{halishaMark: 700})
	scarab := w.spawnHostile(t, "scarab", scarabID, "Monster", 40)
	sx, sy, _ := scarab.Position()

	killOnPlayerQueue(t, w.srv, w.player, scarab)
	archon := w.spawned(t, "archon", halishaID)
	wantLines(t, "the shrine kill", w.render(t, w.srv.ReadQueued(t, w.srv.Client)),
		"S SystemMessage id=301 3:7485 6:20", say("archon", halishaID, 7053, "Seeker"))
	if n := w.srv.PlayerItemCount(t, w.player, halishaMark); n != 680 {
		t.Fatalf("marks %d after the archon woke, want 680", n)
	}
	if at := archon.SpawnLocation(); at.X != sx || at.Y != sy {
		t.Fatalf("the archon stands at (%d, %d), want where the monster fell (%d, %d)", at.X, at.Y, sx, sy)
	}
	if archon.ScriptValue() != w.player {
		t.Fatalf("the archon's script value %d, want the player %d", archon.ScriptValue(), w.player)
	}
	if h := archon.MostHated(); h == nil || h.ObjectID() != w.player {
		t.Fatalf("the archon hates %v most, want the player", h)
	}

	killOnPlayerQueue(t, w.srv, w.player, hostileNPC(t, w.srv, archon))
	wantLines(t, "the archon's kill", w.render(t, w.srv.ReadQueued(t, w.srv.Client)),
		"S SystemMessage id=301 3:7485 6:680", "S SystemMessage id=54 3:7392", say("archon", halishaID, 7054))
	if vars := w.srv.QuestVars(t, w.player, q070Name); vars["<cond>"] != "16" {
		t.Fatalf("after the archon's kill: vars %v, want cond 16", vars)
	}
	if n := w.srv.PlayerItemCount(t, w.player, amulet5th); n != 1 {
		t.Fatalf("fifth amulets %d, want 1", n)
	}
}

// TestQ070ArchonKilledByAnotherCreditsNobody: an archon woken for the
// player and killed by someone else only tells them to mind their own
// business.
func TestQ070ArchonKilledByAnotherCreditsNobody(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 15, map[int32]int32{halishaMark: 700})
	mate, _ := sagaMate(t, w, 15, false)
	scarab := w.spawnHostile(t, "scarab", scarabID, "Monster", 40)
	killOnPlayerQueue(t, w.srv, w.player, scarab)
	archon := w.spawned(t, "archon", halishaID)
	w.srv.ReadQueued(t, w.srv.Client)

	killOnPlayerQueue(t, w.srv, mate, hostileNPC(t, w.srv, archon))
	wantLines(t, "the mate's kill", w.render(t, w.srv.ReadQueued(t, w.srv.Client)), say("archon", halishaID, 7055))
	for _, objID := range []int32{w.player, mate} {
		if vars := w.srv.QuestVars(t, objID, q070Name); vars["<cond>"] != "15" {
			t.Fatalf("player %d: vars %v, want cond 15", objID, vars)
		}
	}
	if n := w.srv.PlayerItemCount(t, w.player, halishaMark); n != 680 {
		t.Fatalf("marks %d, want 680 kept", n)
	}
}

// TestQ070ShrineCreditsARandomPartyMember: a Shrine of Loyalty death in a
// party credits one member at cond 15 of a saga, picked at random among
// them (draw 1 of 2: the mate), whose saga the core finds by the member's
// class; the mark goes to that member.
func TestQ070ShrineCreditsARandomPartyMember(t *testing.T) {
	t.Parallel()
	rs := &rolls{}
	w := bootSaga(t, 15, nil, gameservertest.WithScriptRand(rs.intn))
	mate, _ := sagaMate(t, w, 15, true)
	scarab := w.spawnHostile(t, "scarab", scarabID, "Monster", 40)

	rs.script([]int{1, 0})
	killOnPlayerQueue(t, w.srv, w.player, scarab)
	if got, want := rs.taken(), []string{"2:1", "1000000:0"}; !slices.Equal(got, want) {
		t.Fatalf("draws %v, want %v", got, want)
	}
	if n := w.srv.PlayerItemCount(t, mate, halishaMark); n != 1 {
		t.Fatalf("the mate's marks %d, want 1", n)
	}
	if n := w.srv.PlayerItemCount(t, w.player, halishaMark); n != 0 {
		t.Fatalf("the killer's marks %d, want 0", n)
	}
}

// TestQ070SixthTabletFightWon: the sixth tablet calls the defender and its
// attacker for the player; they speak as they appear, the defender cheers
// the player on, the attacker taunts the player at the first hit and
// leaves at the sixteenth. The defender then hands over the sixth amulet,
// says goodbye and leaves, freeing the tablet.
func TestQ070SixthTabletFightWon(t *testing.T) {
	t.Parallel()
	rs := &rolls{}
	w := bootSaga(t, 17, nil, gameservertest.WithScriptRand(rs.intn))
	w.spawnFolk(t, "tablet6", tablet6ID, -40, 0, 0)

	wantLines(t, "window", w.talk(t, "tablet6"), w.sagaPage(t, "tablet6", "10-01.htm"))
	wantLines(t, "10-1", w.sagaEvent(t, "10-1"), w.sagaPage(t, "tablet6", "10-02.htm"))
	defender := w.spawned(t, "defender", defenderID)
	attacker := w.spawned(t, "attacker", attackerID)
	if defender.ScriptValue() != w.player || attacker.ScriptValue() != w.player {
		t.Fatalf("script values %d, %d, want the player %d", defender.ScriptValue(), attacker.ScriptValue(), w.player)
	}
	if h := attacker.MostHated(); h == nil || h.ObjectID() != defender.ObjectID() {
		t.Fatalf("the attacker hates %v most, want the defender", h)
	}

	wantLines(t, "spawn lines", w.advance(t, 500*time.Millisecond),
		say("defender", defenderID, 7057), say("attacker", attackerID, 7064, "Seeker"))
	wantLines(t, "first cheer", w.advance(t, 1500*time.Millisecond), say("defender", defenderID, 7058, "Seeker"))
	rs.script([]int{0})
	wantLines(t, "idle cheer", w.advance(t, 10*time.Second), say("defender", defenderID, 7059, "Seeker"))

	h := hostileNPC(t, w.srv, attacker)
	hit(t, w.srv, w.player, h, 10)
	wantLines(t, "first hit", w.render(t, w.srv.ReadQueued(t, w.srv.Client)), say("attacker", attackerID, 7066, "Seeker"))
	for range 14 {
		hit(t, w.srv, w.player, h, 10)
	}
	saysNone(t, "hits 2 to 15", w.render(t, w.srv.ReadQueued(t, w.srv.Client)))
	hit(t, w.srv, w.player, h, 10)
	wantLines(t, "sixteenth hit", w.render(t, w.srv.ReadQueued(t, w.srv.Client)), say("attacker", attackerID, 7067, "Seeker"))
	if n := len(npcsOf(w.srv, attackerID)); n != 0 {
		t.Fatalf("%d attackers after the sixteenth hit, want 0", n)
	}
	if vars := w.srv.QuestVars(t, w.player, q070Name); vars["attacks"] != "16" {
		t.Fatalf("vars %v, want 16 attacks", vars)
	}
	rs.script(nil)
	saysNone(t, "no cheer once the attacker left", w.advance(t, 10*time.Second))

	wantLines(t, "the defender's first talk", w.render(t, w.interact(t, defender.ObjectID())), w.sagaPage(t, "defender", "4-06.htm"))
	wantLines(t, "4-1", w.sagaEvent(t, "4-1"), w.sagaPage(t, "defender", "4-010.htm"))
	got := w.sagaEvent(t, "4-3")
	wantLines(t, "4-3", got, "S SystemMessage id=54 3:7423", say("defender", defenderID, 7063))
	if says := keep(got, "S NpcSay "); len(says) != 1 {
		t.Fatalf("4-3: lines %q, want the goodbye alone", says)
	}
	if slices.ContainsFunc(got, func(l string) bool { return len(l) > 17 && l[:17] == "S NpcHtmlMessage " }) {
		t.Fatalf("4-3 showed a page: %q", got)
	}
	if n := len(npcsOf(w.srv, defenderID)); n != 0 {
		t.Fatalf("%d defenders after 4-3, want 0", n)
	}
	if vars := w.srv.QuestVars(t, w.player, q070Name); vars["<cond>"] != "18" || vars["attacks"] != "" {
		t.Fatalf("after 4-3: vars %v, want cond 18 and no attacks", vars)
	}
	wantLines(t, "window at cond 18", w.talk(t, "tablet6"), w.sagaPage(t, "tablet6", "10-05.htm"))
}

// TestQ070SixthTabletFightRunsOut: left alone, the attacker leaves after
// 59 s and the defender after 60 s, lost; the tablet then calls a new
// fight. The attacker says no parting line: an NPC is dead by the time its
// decayed hook runs (Npc.onDecay sets it dead first), so the line the hook
// keeps for a living NPC is never said.
func TestQ070SixthTabletFightRunsOut(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 17, nil, gameservertest.WithScriptRand(func(int) int { return 0 }))
	w.spawnFolk(t, "tablet6", tablet6ID, -40, 0, 0)
	w.talk(t, "tablet6")
	wantLines(t, "10-1", w.sagaEvent(t, "10-1"), w.sagaPage(t, "tablet6", "10-02.htm"))
	w.spawned(t, "defender", defenderID)
	w.spawned(t, "attacker", attackerID)

	w.advance(t, 58*time.Second)
	saysNone(t, "59 s", w.advance(t, time.Second))
	if n := len(npcsOf(w.srv, attackerID)); n != 0 {
		t.Fatalf("%d attackers after 59 s, want 0", n)
	}
	wantLines(t, "60 s", w.advance(t, time.Second), say("defender", defenderID, 7062))
	if n := len(npcsOf(w.srv, defenderID)) + len(npcsOf(w.srv, attackerID)); n != 0 {
		t.Fatalf("%d fight NPCs after 60 s, want 0", n)
	}

	w.talk(t, "tablet6")
	wantLines(t, "10-1 again", w.sagaEvent(t, "10-1"), w.sagaPage(t, "tablet6", "10-02.htm"))
}

// TestQ070FinishMakesAPhoenixKnight: Sedrick ends the saga of a level 76
// Paladin playing its base class: it plays a Phoenix Knight, which is its
// base class from then on.
func TestQ070FinishMakesAPhoenixKnight(t *testing.T) {
	t.Parallel()
	w := bootSaga(t, 19, nil)
	w.spawnFolk(t, "sedrick", sedrickID, -40, 0, 0)
	wantLines(t, "window", w.talk(t, "sedrick"), w.sagaPage(t, "sedrick", "0-06.htm"))
	wantLines(t, "0-2", w.sagaEvent(t, "0-2"), w.sagaPage(t, "sedrick", "0-07.htm"))
	w.srv.RunQuest(t, w.player, q070Name, func(_ *script.Quests, c *player.Character, _ *script.Script) {
		if c.ClassID() != phoenixKnight || c.BaseClassID() != phoenixKnight {
			t.Errorf("class %d base %d, want both %d", c.ClassID(), c.BaseClassID(), phoenixKnight)
		}
	})
	if st := w.srv.QuestVars(t, w.player, q070Name); st["<state>"] != "COMPLETED" {
		t.Fatalf("vars %v, want the saga completed", st)
	}
}
