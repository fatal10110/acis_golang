package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q025"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Hiding Behind the Truth's Triol's pawn and coffin (Q025_HidingBehindTheTruth
// onAdvEvent "3153x-07.htm" and "31531-02.htm", onAttacked, onTalk of the
// coffin). A scenario cannot show a script-spawned NPC appearing, speaking or
// leaving, nor act on one (#4020 item 5), so these Boot tests pin them.
const (
	q025Name      = "Q025_HidingBehindTheTruth"
	q025Path      = "quest.Q025_HidingBehindTheTruth"
	q025Shelf     = 31533
	q025Tombstone = 31531
	q025Coffin    = 31536
	q025Pawn      = 27218
	q025Dress     = 7155
	q025Doll      = 7158
)

var (
	// q025PawnAt is where the first broken bookshelf wakes Triol's pawn.
	q025PawnAt = [3]int{47142, -35941, -1623}
	// q025CoffinAt is where the tombstone raises the coffin.
	q025CoffinAt = [3]int{60104, -35820, -681}
)

// q025World boots a level 66 character whose Hiding Behind the Truth has
// the rows vars, standing at at, with the NPC role of npcID 40 east of it.
func q025World(t *testing.T, at [3]int, vars map[string]string, pages map[string][]string, role string, npcID int) *dialogWorld {
	t.Helper()
	w := bootSeamQuest(t, 66, chatPages(questPages(q025Name, pages), q025Shelf, q025Tombstone, q025Coffin), func(srv *gameservertest.Server, objID int32) {
		if _, err := srv.DB.Exec(`UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?`, at[0], at[1], at[2], objID); err != nil {
			t.Fatal(err)
		}
		rows := []journalRow{{objID, q025Name, "<state>", val("STARTED")}}
		for k, v := range vars {
			rows = append(rows, journalRow{objID, q025Name, k, val(v)})
		}
		insertJournal(t, srv.DB, rows...)
	},
		x1Items([]int32{q025Dress, q025Doll}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", q025Coffin), proofMonster(q025Pawn)})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q025Shelf: script.KindFolk, q025Tombstone: script.KindFolk, q025Coffin: script.KindFolk, q025Pawn: script.KindHostile},
			[]script.Listing{{Path: q025Path}},
			script.Catalog{q025Path: q025.New},
		),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, role, npcID, 40, 0, 0)
	return w
}

// pawnWorld boots a character that found the gem box in the first broken
// bookshelf (state 8, cond 6) and wakes Triol's pawn: it stands at its
// spot as the bookshelf's private, calls the player out by name and is
// bound to the player (cond 7).
func pawnWorld(t *testing.T) (*dialogWorld, *script.NPC) {
	t.Helper()
	w := q025World(t, q025PawnAt, map[string]string{"<cond>": "6", "state": "8", "bookshelf": "31533"}, map[string][]string{
		"3153x-05.htm": {"3153x-07.htm"},
		"3153x-07.htm": nil,
		"3153x-08.htm": nil,
		"3153x-09.htm": nil,
		"3153x-10.htm": {"3153x-11.htm"},
	}, "shelf", q025Shelf)
	if got := w.talk(t, "shelf"); !shows(got, "3153x-05.htm") {
		t.Fatalf("bookshelf: got %q, want 3153x-05.htm", got)
	}
	got := w.send(t, encodeBypass("Quest "+q025Name+" 3153x-07.htm"))
	pawn := w.spawned(t, "pawn", q025Pawn)
	wantLines(t, "the wake", got, say(strconv.Itoa(int(pawn.ObjectID())), q025Pawn, 2550, "Seeker"))
	if !shows(got, "3153x-07.htm") {
		t.Fatalf("the wake: got %q, want 3153x-07.htm", got)
	}
	if loc := pawn.SpawnLocation(); loc.X != q025PawnAt[0] || loc.Y != q025PawnAt[1] || abs(loc.Z-q025PawnAt[2]) > 32 {
		t.Fatalf("the pawn stands at %+v, want %v", loc, q025PawnAt)
	}
	if m := pawn.Master(); m == nil || m.ObjectID() != w.npcs["shelf"] {
		t.Fatalf("the pawn's master is %v, want the bookshelf", m)
	}
	if rows := questRows(t, w.srv, w.player, q025Name); rows["<cond>"] != "7" {
		t.Fatalf("rows %v after the wake, want cond 7", rows)
	}
	return w, pawn
}

// TestQ025TriolsPawnGuardsTheGemBox: while Triol's pawn stands, the gem box
// tells the player the pawn is its own. A hit on the pawn below 30% of its
// HP makes it leave the third totem doll (cond 8) with its last word and go,
// and the box then asks for the key.
func TestQ025TriolsPawnGuardsTheGemBox(t *testing.T) {
	t.Parallel()
	w, pawn := pawnWorld(t)
	w.talk(t, "shelf")
	if got := w.send(t, encodeBypass("Quest "+q025Name+" 3153x-07.htm")); !shows(got, "3153x-08.htm") {
		t.Fatalf("the box while the pawn stands: got %q, want 3153x-08.htm", got)
	}

	// The hook reads the HP the hit found: down to 30% and the next hit there
	// leave it be; the hit after that finds it below.
	h := hostileNPC(t, w.srv, pawn)
	hit(t, w.srv, w.player, h, h.MaxHP()*7/10)
	hit(t, w.srv, w.player, h, 1)
	saysNone(t, "the pawn at 30%", w.advance(t, 500*time.Millisecond))
	if n := w.srv.PlayerItemCount(t, w.player, q025Doll); n != 0 {
		t.Fatalf("dolls %d with the pawn at 30%%, want 0", n)
	}

	hit(t, w.srv, w.player, h, 1)
	wantLines(t, "the flight", w.advance(t, 500*time.Millisecond), say("pawn", q025Pawn, 2551))
	if n := w.srv.PlayerItemCount(t, w.player, q025Doll); n != 1 {
		t.Fatalf("dolls %d after the flight, want 1", n)
	}
	if rows := questRows(t, w.srv, w.player, q025Name); rows["<cond>"] != "8" {
		t.Fatalf("rows %v after the flight, want cond 8", rows)
	}
	if n := len(npcsOf(w.srv, q025Pawn)); n != 0 {
		t.Fatalf("%d pawns after the flight, want 0", n)
	}
	w.talk(t, "shelf")
	if got := w.send(t, encodeBypass("Quest "+q025Name+" 3153x-07.htm")); !shows(got, "3153x-10.htm") {
		t.Fatalf("the box after the flight: got %q, want 3153x-10.htm", got)
	}
}

// TestQ025TriolsPawnLeavesAfterTwoMinutes: Triol's pawn leaves two minutes
// after it woke, without a word, and the box then wakes another.
func TestQ025TriolsPawnLeavesAfterTwoMinutes(t *testing.T) {
	t.Parallel()
	w, _ := pawnWorld(t)
	w.advance(t, 119900*time.Millisecond)
	if n := len(npcsOf(w.srv, q025Pawn)); n != 1 {
		t.Fatalf("%d pawns 119.9 s after the wake, want 1", n)
	}
	saysNone(t, "two minutes", w.advance(t, 100*time.Millisecond))
	if n := len(npcsOf(w.srv, q025Pawn)); n != 0 {
		t.Fatalf("%d pawns two minutes after the wake, want 0", n)
	}
	w.talk(t, "shelf")
	got := w.send(t, encodeBypass("Quest "+q025Name+" 3153x-07.htm"))
	pawn := w.spawned(t, "pawn2", q025Pawn)
	wantLines(t, "the second wake", got, say(strconv.Itoa(int(pawn.ObjectID())), q025Pawn, 2550, "Seeker"))
	if pawn.ScriptValue() != w.player {
		t.Fatalf("the second pawn is bound to %d, want the player", pawn.ScriptValue())
	}
}

// coffinWorld boots a character the maid frightened (state 11, cond 11),
// standing where the tombstone raises the coffin, and raises it: the
// tombstone's private, at its spot (cond 12).
func coffinWorld(t *testing.T) (*dialogWorld, *script.NPC) {
	t.Helper()
	w := q025World(t, q025CoffinAt, map[string]string{"<cond>": "11", "state": "11"}, map[string][]string{
		"31531-01.htm": {"31531-02.htm"},
		"31531-02.htm": nil,
		"31531-03.htm": nil,
		"31536-01.htm": nil,
	}, "tombstone", q025Tombstone)
	if got := w.talk(t, "tombstone"); !shows(got, "31531-01.htm") {
		t.Fatalf("tombstone: got %q, want 31531-01.htm", got)
	}
	if got := w.send(t, encodeBypass("Quest "+q025Name+" 31531-02.htm")); !shows(got, "31531-02.htm") {
		t.Fatalf("the raise: got %q, want 31531-02.htm", got)
	}
	coffin := w.spawned(t, "coffin", q025Coffin)
	if loc := coffin.SpawnLocation(); loc.X != q025CoffinAt[0] || loc.Y != q025CoffinAt[1] || abs(loc.Z-q025CoffinAt[2]) > 32 {
		t.Fatalf("the coffin stands at %+v, want %v", loc, q025CoffinAt)
	}
	if m := coffin.Master(); m == nil || m.ObjectID() != w.npcs["tombstone"] {
		t.Fatalf("the coffin's master is %v, want the tombstone", m)
	}
	if rows := questRows(t, w.srv, w.player, q025Name); rows["<cond>"] != "12" {
		t.Fatalf("rows %v after the raise, want cond 12", rows)
	}
	return w, coffin
}

// TestQ025CoffinHoldsTheDress: while the coffin stands the tombstone says
// so; the coffin gives the dress (cond 13) and is gone, and the tombstone
// has nothing more to say. A coffin left alone sinks after twenty seconds,
// and the tombstone then raises another.
func TestQ025CoffinHoldsTheDress(t *testing.T) {
	t.Parallel()
	t.Run("opened", func(t *testing.T) {
		t.Parallel()
		w, _ := coffinWorld(t)
		if got := w.talk(t, "tombstone"); !shows(got, "31531-02.htm") {
			t.Fatalf("tombstone while the coffin stands: got %q, want 31531-02.htm", got)
		}
		if got := w.talk(t, "coffin"); !shows(got, "31536-01.htm") {
			t.Fatalf("coffin: got %q, want 31536-01.htm", got)
		}
		if n := w.srv.PlayerItemCount(t, w.player, q025Dress); n != 1 {
			t.Fatalf("dresses %d after the coffin, want 1", n)
		}
		if rows := questRows(t, w.srv, w.player, q025Name); rows["<cond>"] != "13" || rows["state"] != "12" {
			t.Fatalf("rows %v after the coffin, want cond 13 and state 12", rows)
		}
		if n := len(npcsOf(w.srv, q025Coffin)); n != 0 {
			t.Fatalf("%d coffins after the dress, want 0", n)
		}
		if got := w.talk(t, "tombstone"); !shows(got, "31531-03.htm") {
			t.Fatalf("tombstone after the dress: got %q, want 31531-03.htm", got)
		}
	})
	t.Run("sinks", func(t *testing.T) {
		t.Parallel()
		w, _ := coffinWorld(t)
		w.advance(t, 19900*time.Millisecond)
		if n := len(npcsOf(w.srv, q025Coffin)); n != 1 {
			t.Fatalf("%d coffins 19.9 s after the raise, want 1", n)
		}
		w.advance(t, 100*time.Millisecond)
		if n := len(npcsOf(w.srv, q025Coffin)); n != 0 {
			t.Fatalf("%d coffins twenty seconds after the raise, want 0", n)
		}
		if got := w.talk(t, "tombstone"); !shows(got, "31531-01.htm") {
			t.Fatalf("tombstone after the coffin sank: got %q, want 31531-01.htm", got)
		}
		w.send(t, encodeBypass("Quest "+q025Name+" 31531-02.htm"))
		if n := len(npcsOf(w.srv, q025Coffin)); n != 1 {
			t.Fatalf("%d coffins after the second raise, want 1", n)
		}
	})
}

// TestQ025TriolsPawnAnswersOnlyItsPlayer: another player at the same point
// of the quest finds the gem box guarded by a pawn that is not its own, and
// its hits on that pawn below 30% of its HP give it nothing.
func TestQ025TriolsPawnAnswersOnlyItsPlayer(t *testing.T) {
	t.Parallel()
	w, pawn := pawnWorld(t)
	srv := w.srv
	mate := srv.SeedCharacterFor(t, "mate", "Mate", 66, 0).ID
	if _, err := srv.DB.Exec(`UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?`, q025PawnAt[0], q025PawnAt[1], q025PawnAt[2], mate); err != nil {
		t.Fatal(err)
	}
	insertJournal(t, srv.DB,
		journalRow{mate, q025Name, "<state>", val("STARTED")},
		journalRow{mate, q025Name, "<cond>", val("7")},
		journalRow{mate, q025Name, "state", val("8")},
		journalRow{mate, q025Name, "bookshelf", val("31533")})
	c := srv.DialClient(t, "mate", 1)
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, c)
	srv.ReadQueued(t, srv.Client)
	w.roles[mate] = "mate"

	if got := w.talkAs(t, c, "shelf"); !shows(got, "3153x-05.htm") {
		t.Fatalf("bookshelf to the mate: got %q, want 3153x-05.htm", got)
	}
	c.Send(encodeBypass("Quest " + q025Name + " 3153x-07.htm"))
	if got := w.render(t, srv.ReadQueued(t, c)); !shows(got, "3153x-09.htm") {
		t.Fatalf("the box to the mate: got %q, want 3153x-09.htm", got)
	}

	h := hostileNPC(t, srv, pawn)
	hit(t, srv, mate, h, h.MaxHP()*8/10)
	hit(t, srv, mate, h, 1)
	srv.Advance(t, 500*time.Millisecond)
	if n := srv.PlayerItemCount(t, mate, q025Doll); n != 0 {
		t.Fatalf("the mate's dolls %d after its hits below 30%%, want 0", n)
	}
	if rows := questRows(t, srv, mate, q025Name); rows["<cond>"] != "7" {
		t.Fatalf("the mate's rows %v after its hits, want cond 7", rows)
	}
	if n := len(npcsOf(srv, q025Pawn)); n != 1 {
		t.Fatalf("%d pawns after the mate's hits, want 1", n)
	}
}

// TestQ025OnlyABookshelfWakesThePawn: the gem box's event sent through the
// tombstone wakes no pawn and shows nothing, and leaves the tombstone free
// to raise the coffin.
func TestQ025OnlyABookshelfWakesThePawn(t *testing.T) {
	t.Parallel()
	w := q025World(t, q025CoffinAt, map[string]string{"<cond>": "11", "state": "11"}, map[string][]string{
		"31531-01.htm": {"3153x-07.htm", "31531-02.htm"},
		"31531-02.htm": nil,
		"3153x-07.htm": nil,
	}, "tombstone", q025Tombstone)
	if got := w.talk(t, "tombstone"); !shows(got, "31531-01.htm") {
		t.Fatalf("tombstone: got %q, want 31531-01.htm", got)
	}
	if got := w.send(t, encodeBypass("Quest "+q025Name+" 3153x-07.htm")); len(got) != 0 {
		t.Fatalf("the box's event through the tombstone: got %q, want nothing", got)
	}
	if n := len(npcsOf(w.srv, q025Pawn)); n != 0 {
		t.Fatalf("%d pawns after the event through the tombstone, want 0", n)
	}
	if rows := questRows(t, w.srv, w.player, q025Name); rows["<cond>"] != "11" {
		t.Fatalf("rows %v after the event through the tombstone, want cond 11", rows)
	}
	w.talk(t, "tombstone")
	w.send(t, encodeBypass("Quest "+q025Name+" 31531-02.htm"))
	if n := len(npcsOf(w.srv, q025Coffin)); n != 1 {
		t.Fatalf("%d coffins after the raise, want 1", n)
	}
}
