package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q334"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Wishing Potion's fairy, demons and chest (Q334_TheWishingPotion
// onTimer "timer_3", onTalk, onCreated, onDecayed, onMyDying). A scenario
// cannot show a script-spawned NPC appearing, speaking from out of sight or
// leaving, nor act on one, and a placed NPC's created line is sent before
// the character stands in the world (#4020 item 5), so these Boot tests pin
// the wishes that call something out and the monsters of the wishes' dying
// hooks.
const (
	q334Name     = "Q334_TheWishingPotion"
	q334Path     = "quest.Q334_TheWishingPotion"
	q334Matild   = 30738
	q334Rupina   = 30742
	q334Chest    = 30743
	q334Grima    = 27135
	q334Succubus = 27136
	q334King     = 27138
	q334Alexandr = 27153
	q334Bonapart = 27154
	q334Ramsebal = 27155

	q334WishPotion = 3467
	q334Orb        = 3682
	q334Scroll     = 3683
	q334Necklace   = 931
	q334Heart      = 3943
	q334ScoreLove  = 4408
	q334ScoreWar   = 4409
	q334Adena      = 57
)

// q334Holds is one wish potion and Matild's orb.
var q334Holds = map[int32]int32{q334WishPotion: 1, q334Orb: 1}

// q334World boots a character at cond 5 holding the given items, with wish
// pending (0 for none), Matild standing 40 away as "matild". The script
// random answers draws.
func q334World(t *testing.T, draws *scriptDraws, wish int, holds map[int32]int32) *dialogWorld {
	t.Helper()
	pages := map[string][]string{
		"30738-12.htm": {"30738-14.htm", "30738-15a.htm"},
		"30738-13.htm": {"30738-16.htm", "30738-17.htm", "30738-18.htm", "30738-19.htm"},
		"30738-14.htm": {"30738-16.htm"},
		"30738-16.htm": nil,
		"30738-17.htm": nil,
		"30738-18.htm": {"30738-17.htm"},
		"30738-19.htm": nil,
		"30742-01.htm": nil,
		"30742-02.htm": nil,
	}
	for _, p := range []string{"30743-01.htm", "30743-02.htm", "30743-03.htm", "30743-04.htm", "30743-05.htm", "30743-06.htm"} {
		pages[p] = nil
	}
	monsters := []int{q334Grima, q334Succubus, q334King, q334Alexandr, q334Bonapart, q334Ramsebal}
	templates := []*npc.Template{
		gameservertest.FolkTemplate("Folk", q334Matild), gameservertest.FolkTemplate("Folk", q334Rupina), gameservertest.FolkTemplate("Folk", q334Chest),
	}
	kinds := map[int32]script.NPCKind{q334Matild: script.KindFolk, q334Rupina: script.KindFolk, q334Chest: script.KindFolk}
	for _, id := range monsters {
		templates = append(templates, proofMonster(id))
		kinds[int32(id)] = script.KindHostile
	}
	w := bootSeamQuest(t, 30, chatPages(questPages(q334Name, pages), q334Matild, q334Rupina, q334Chest), func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q334Name, "<state>", val("STARTED")},
			journalRow{objID, q334Name, "<cond>", val("5")},
			journalRow{objID, q334Name, "wish", val(strconv.Itoa(wish))})
		for id, count := range holds {
			srv.GiveItem(t, objID, id, count)
		}
	},
		x1Items([]int32{
			q334WishPotion, q334Orb, q334Scroll, q334Necklace, q334Heart, q334ScoreLove, q334ScoreWar,
			1979, 1980, 2952, 2953, 3075, 3941, 3090, 3469,
		}),
		gameservertest.WithNPCs(npc.NewTable(templates)),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: q334Path}}, script.Catalog{q334Path: q334.New}),
		gameservertest.WithScriptRand(draws.rand),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "matild", q334Matild, 40, 0, 0)
	return w
}

// event sends the quest event ev as a bypass.
func (w *dialogWorld) q334Event(t *testing.T, ev string) []string {
	t.Helper()
	return w.send(t, encodeBypass("Quest "+q334Name+" "+ev))
}

// q334Wish has the character make wish 1 to 4 over Matild's pages and lets
// the three lines pass, returning what the character was sent by the last.
func q334Wish(t *testing.T, w *dialogWorld, wish int) []string {
	t.Helper()
	if got := w.talk(t, "matild"); !shows(got, "30738-12.htm") {
		t.Fatalf("Matild: got %q, want 30738-12.htm", got)
	}
	if got := w.q334Event(t, "30738-14.htm"); !shows(got, "30738-13.htm") {
		t.Fatalf("30738-14.htm: got %q, want 30738-13.htm", got)
	}
	if got := w.q334Event(t, "30738-"+strconv.Itoa(15+wish)+".htm"); !shows(got, "30738-"+strconv.Itoa(15+wish)+".htm") {
		t.Fatalf("the wish: got %q", got)
	}
	wantLines(t, "the first line", w.advance(t, 3*time.Second), say("matild", q334Matild, 33415))
	wantLines(t, "the second line", w.advance(t, 4*time.Second), say("matild", q334Matild, 33416))
	return w.advance(t, 4*time.Second)
}

// q334Say is the rendered NpcSay of the script-spawned n.
func q334Say(n *script.NPC, id int32) string {
	return say(strconv.Itoa(int(n.ObjectID())), n.NpcID(), id)
}

// q334Near fails unless n stands within 100 of the character on each axis.
func q334Near(t *testing.T, w *dialogWorld, n *script.NPC) {
	t.Helper()
	if loc := n.SpawnLocation(); loc.X < w.at.X-100 || loc.X > w.at.X+100 || loc.Y < w.at.Y-100 || loc.Y > w.at.Y+100 {
		t.Fatalf("NPC %d stands at %+v, want within 100 of %+v", n.NpcID(), loc, w.at)
	}
}

// q334Beside walks the character to where n stands, which a scattered spawn
// may leave out of talking range.
func q334Beside(t *testing.T, w *dialogWorld, n *script.NPC) {
	t.Helper()
	loc := n.SpawnLocation()
	w.walkTo(t, location.Location{X: loc.X, Y: loc.Y, Z: loc.Z})
}

// q334Kill lands the character's lethal hit on n and returns what it was
// sent meanwhile.
func q334Kill(t *testing.T, w *dialogWorld, n *script.NPC) []string {
	t.Helper()
	killOnPlayerQueue(t, w.srv, w.player, hostileNPC(t, w.srv, n))
	return w.render(t, w.srv.ReadQueued(t, w.srv.Client))
}

// q334Wishes asserts the quest variable key.
func q334Row(t *testing.T, w *dialogWorld, key, want string) {
	t.Helper()
	if got := questRows(t, w.srv, w.player, q334Name)[key]; got != want {
		t.Fatalf("quest variable %s = %q, want %q", key, got, want)
	}
}

// TestQ334FairyRupinaAnswersWishOne: wish 1 with the draw 0 of two calls
// the fairy within 100 of the character, who says her line as she appears.
// Talking to her gives the necklace of grace on a draw under 4 of 100, else
// a part of the demon set, ends the wish and sends her away; left alone she
// goes after 120 seconds.
func TestQ334FairyRupinaAnswersWishOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		draws []int // after the call's: the 100, then the demon part of 4
		page  string
		item  int32
	}{
		{"necklace", []int{3}, "30742-01.htm", q334Necklace},
		{"demon part", []int{4, 2}, "30742-02.htm", 2952},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			w := q334World(t, draws, 0, q334Holds)
			draws.queue(0)
			got := q334Wish(t, w, 1)
			rupina := w.spawned(t, "rupina", q334Rupina)
			wantLines(t, "the call", got, say("matild", q334Matild, 33417), q334Say(rupina, 33420))
			q334Near(t, w, rupina)
			q334Row(t, w, "choice", "0")
			q334Row(t, w, "wish", "1")
			draws.queue(tc.draws...)
			q334Beside(t, w, rupina)
			if got := w.talk(t, "rupina"); !shows(got, tc.page) {
				t.Fatalf("Rupina: got %q, want %s", got, tc.page)
			}
			if n := w.srv.PlayerItemCount(t, w.player, tc.item); n != 1 {
				t.Fatalf("item %d: %d, want 1", tc.item, n)
			}
			q334Row(t, w, "wish", "0")
			if n := len(npcsOf(w.srv, q334Rupina)); n != 0 {
				t.Fatalf("%d fairies after the gift, want 0", n)
			}
		})
	}
	t.Run("leaves", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q334World(t, draws, 0, q334Holds)
		q334Wish(t, w, 1)
		w.advance(t, 119900*time.Millisecond)
		if n := len(npcsOf(w.srv, q334Rupina)); n != 1 {
			t.Fatalf("%d fairies 119.9 s after the call, want 1", n)
		}
		w.advance(t, 100*time.Millisecond)
		if n := len(npcsOf(w.srv, q334Rupina)); n != 0 {
			t.Fatalf("%d fairies after 120 s, want 0", n)
		}
		q334Row(t, w, "wish", "1")
	})
}

// TestQ334SuccubiAnswerWishOne: wish 1 with the draw 1 calls three
// succubi, each with its line. One killed drops the forbidden love scroll
// on a draw under 28000 of 1000000, once.
func TestQ334SuccubiAnswerWishOne(t *testing.T) {
	t.Parallel()
	draws := &scriptDraws{}
	w := q334World(t, draws, 0, q334Holds)
	draws.queue(1)
	got := q334Wish(t, w, 1)
	succubi := npcsOf(w.srv, q334Succubus)
	if len(succubi) != 3 {
		t.Fatalf("%d succubi, want 3", len(succubi))
	}
	for _, s := range succubi {
		wantLines(t, "the call", got, q334Say(s, 33423))
		q334Near(t, w, s)
	}
	q334Row(t, w, "choice", "0")

	draws.queue(28000)
	q334Kill(t, w, succubi[0])
	if n := w.srv.PlayerItemCount(t, w.player, q334Scroll); n != 0 {
		t.Fatalf("%d scrolls after a missed draw, want 0", n)
	}
	q334Row(t, w, "wish", "1")
	draws.queue(27999)
	q334Kill(t, w, succubi[1])
	if n := w.srv.PlayerItemCount(t, w.player, q334Scroll); n != 1 {
		t.Fatalf("%d scrolls after the drop, want 1", n)
	}
	// The reference's drop of a scroll without a needed count reports false, so the
	// wish is never ended by it: the scroll held is what stops further drops.
	q334Row(t, w, "wish", "1")
	q334Kill(t, w, succubi[2])
	if n := w.srv.PlayerItemCount(t, w.player, q334Scroll); n != 1 {
		t.Fatalf("%d scrolls after the wish is over, want 1", n)
	}

	w.advance(t, 200*time.Second)
	if n := len(npcsOf(w.srv, q334Succubus)); n != 0 {
		t.Fatalf("%d succubi after 200 s, want 0", n)
	}
}

// TestQ334GrimasAnswerWishTwo: wish 2 with the draw 0 calls three Grimas,
// each with its line. A Grima killed pays 900000 adena when its first draw
// of 1000 is under 33 and the second is not 0, 100000000 when it is, and
// ends the wish; a draw of 33 pays nothing.
func TestQ334GrimasAnswerWishTwo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		draws []int
		adena int
		ends  bool
	}{
		{"miss", []int{33}, 0, false},
		{"adena", []int{32, 1}, 900000, true},
		{"lucky", []int{0, 0}, 100000000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			w := q334World(t, draws, 0, q334Holds)
			draws.queue(0)
			got := q334Wish(t, w, 2)
			grimas := npcsOf(w.srv, q334Grima)
			if len(grimas) != 3 {
				t.Fatalf("%d Grimas, want 3", len(grimas))
			}
			for _, g := range grimas {
				wantLines(t, "the call", got, q334Say(g, 33422))
				q334Near(t, w, g)
			}
			draws.queue(tc.draws...)
			q334Kill(t, w, grimas[0])
			if n := w.srv.PlayerItemCount(t, w.player, q334Adena); n != tc.adena {
				t.Fatalf("%d adena, want %d", n, tc.adena)
			}
			if tc.ends {
				q334Row(t, w, "wish", "0")
			} else {
				q334Row(t, w, "wish", "2")
			}
			w.advance(t, 200*time.Second)
			if n := len(npcsOf(w.srv, q334Grima)); n != 0 {
				t.Fatalf("%d Grimas after 200 s, want 0", n)
			}
		})
	}
}

// q334Demon returns the one NPC of id in the world.
func q334Demon(t *testing.T, w *dialogWorld, id int32) *script.NPC {
	t.Helper()
	found := npcsOf(w.srv, id)
	if len(found) != 1 {
		t.Fatalf("%d NPCs %d in the world, want 1", len(found), id)
	}
	return found[0]
}

// TestQ334DemonsAnswerWishThree: wish 3 with the draw 1 calls Alexandro
// Sanches. Each demon killed says its line and, on an even draw, calls the
// next (Bonaparterius, Ramsebalius, the Great Demon King); on an odd one it
// ends the wish with a part of the demon set. The king pays 1406956 adena.
func TestQ334DemonsAnswerWishThree(t *testing.T) {
	t.Parallel()
	t.Run("chain", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q334World(t, draws, 0, q334Holds)
		draws.queue(1)
		got := q334Wish(t, w, 3)
		alexandro := q334Demon(t, w, q334Alexandr)
		wantLines(t, "the call", got, q334Say(alexandro, 33424))
		q334Near(t, w, alexandro)
		q334Row(t, w, "choice", "0")

		draws.queue(0)
		got = q334Kill(t, w, alexandro)
		bonaparterius := q334Demon(t, w, q334Bonapart)
		wantLines(t, "Alexandro's death", got, q334Say(alexandro, 33414), q334Say(bonaparterius, 33425))
		q334Near(t, w, bonaparterius)
		draws.queue(0)
		got = q334Kill(t, w, bonaparterius)
		ramsebalius := q334Demon(t, w, q334Ramsebal)
		wantLines(t, "Bonaparterius' death", got, q334Say(bonaparterius, 33413), q334Say(ramsebalius, 33426))
		draws.queue(0)
		got = q334Kill(t, w, ramsebalius)
		king := q334Demon(t, w, q334King)
		wantLines(t, "Ramsebalius' death", got, q334Say(ramsebalius, 33412), q334Say(king, 33418))
		q334Row(t, w, "wish", "3")
		q334Kill(t, w, king)
		if n := w.srv.PlayerItemCount(t, w.player, q334Adena); n != 1406956 {
			t.Fatalf("%d adena after the king, want 1406956", n)
		}
		q334Row(t, w, "wish", "0")
	})
	t.Run("reward", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q334World(t, draws, 0, q334Holds)
		draws.queue(1)
		q334Wish(t, w, 3)
		alexandro := q334Demon(t, w, q334Alexandr)
		draws.queue(1, 2)
		got := q334Kill(t, w, alexandro)
		wantLines(t, "Alexandro's death", got, q334Say(alexandro, 33414))
		if n := len(npcsOf(w.srv, q334Bonapart)); n != 0 {
			t.Fatalf("%d Bonaparterius after the reward, want 0", n)
		}
		if n := w.srv.PlayerItemCount(t, w.player, 2952); n != 1 {
			t.Fatalf("%d demon boots fabrics, want 1", n)
		}
		q334Row(t, w, "wish", "0")
	})
	t.Run("leave", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q334World(t, draws, 0, q334Holds)
		draws.queue(1)
		q334Wish(t, w, 3)
		alexandro := q334Demon(t, w, q334Alexandr)
		draws.queue(0)
		q334Kill(t, w, alexandro)
		bonaparterius := q334Demon(t, w, q334Bonapart)
		draws.queue(0)
		q334Kill(t, w, bonaparterius)
		draws.queue(0)
		q334Kill(t, w, q334Demon(t, w, q334Ramsebal))
		q334Demon(t, w, q334King)
		w.advance(t, 200*time.Second)
		if n := len(npcsOf(w.srv, q334King)); n != 0 {
			t.Fatalf("%d kings after 200 s, want 0", n)
		}
	})
	t.Run("leave alive", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q334World(t, draws, 0, q334Holds)
		draws.queue(1)
		q334Wish(t, w, 3)
		q334Demon(t, w, q334Alexandr)
		// The leaving words (33411, and 33409 then 33410 for the king) belong to a decayed
		// hook that must see the NPC alive: #4256.
		w.advance(t, 200*time.Second)
		if n := len(npcsOf(w.srv, q334Alexandr)); n != 0 {
			t.Fatalf("%d Alexandro after 200 s, want 0", n)
		}
	})
}

// TestQ334ChestOfWisdomAnswersWishFour: wish 4 with the draw 1 or 2 calls
// the chest. Talking to it shows the page its draw of 10 picks, gives one
// item of each of the three lists, the musical scores and, on a draw of 0
// of 3, the heart of Paagrio, ends the wish and takes the chest away; left
// alone it goes after 120 seconds.
func TestQ334ChestOfWisdomAnswersWishFour(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		choice int
		rand   int
		page   string
		heart  int
	}{
		{"choice 1, draw 1", 1, 1, "30743-01.htm", 1},
		{"choice 2, draw 3", 2, 3, "30743-02.htm", 0},
		{"draw 5", 1, 5, "30743-03.htm", 1},
		{"draw 6", 1, 6, "30743-04.htm", 0},
		{"draw 8", 2, 8, "30743-05.htm", 1},
		{"draw 9", 2, 9, "30743-06.htm", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draws := &scriptDraws{}
			w := q334World(t, draws, 0, q334Holds)
			draws.queue(tc.choice)
			got := q334Wish(t, w, 4)
			chest := w.spawned(t, "chest", q334Chest)
			wantLines(t, "the call", got, q334Say(chest, 33421))
			q334Near(t, w, chest)
			q334Row(t, w, "choice", strconv.Itoa(tc.choice))
			heartDraw := 1
			if tc.heart == 1 {
				heartDraw = 0
			}
			draws.queue(tc.rand, 2, 3, 4, heartDraw)
			q334Beside(t, w, chest)
			if got := w.talk(t, "chest"); !shows(got, tc.page) {
				t.Fatalf("chest: got %q, want %s", got, tc.page)
			}
			for _, id := range []int32{3075, 3941, 3090, q334ScoreLove, q334ScoreWar} {
				if n := w.srv.PlayerItemCount(t, w.player, id); n != 1 {
					t.Fatalf("item %d: %d, want 1", id, n)
				}
			}
			if n := w.srv.PlayerItemCount(t, w.player, q334Heart); n != tc.heart {
				t.Fatalf("hearts of Paagrio: %d, want %d", n, tc.heart)
			}
			q334Row(t, w, "wish", "0")
			if n := len(npcsOf(w.srv, q334Chest)); n != 0 {
				t.Fatalf("%d chests after the gift, want 0", n)
			}
		})
	}
	t.Run("leaves", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q334World(t, draws, 0, q334Holds)
		draws.queue(1)
		q334Wish(t, w, 4)
		w.advance(t, 119900*time.Millisecond)
		if n := len(npcsOf(w.srv, q334Chest)); n != 1 {
			t.Fatalf("%d chests 119.9 s after the call, want 1", n)
		}
		w.advance(t, 100*time.Millisecond)
		if n := len(npcsOf(w.srv, q334Chest)); n != 0 {
			t.Fatalf("%d chests after 120 s, want 0", n)
		}
	})
}

// TestQ334WishRefusals: an event of a wish takes the potion only when the
// character holds one and no wish is under way; it shows Matild's page of the
// event all the same.
func TestQ334WishRefusals(t *testing.T) {
	t.Parallel()
	t.Run("no potion", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q334World(t, draws, 0, map[int32]int32{q334Orb: 1})
		w.talk(t, "matild")
		if got := w.q334Event(t, "30738-14.htm"); !shows(got, "30738-14.htm") {
			t.Fatalf("30738-14.htm: got %q", got)
		}
		if got := w.q334Event(t, "30738-16.htm"); !shows(got, "30738-16.htm") {
			t.Fatalf("30738-16.htm: got %q", got)
		}
		saysNone(t, "twelve seconds", w.advance(t, 12*time.Second))
		q334Row(t, w, "wish", "0")
		if _, ok := questRows(t, w.srv, w.player, q334Name)["exchange"]; ok {
			t.Fatal("an exchange is under way with no potion")
		}
	})
	t.Run("wish under way", func(t *testing.T) {
		t.Parallel()
		draws := &scriptDraws{}
		w := q334World(t, draws, 0, map[int32]int32{q334WishPotion: 2, q334Orb: 1})
		w.talk(t, "matild")
		w.q334Event(t, "30738-14.htm")
		w.q334Event(t, "30738-18.htm")
		if got := w.q334Event(t, "30738-17.htm"); !shows(got, "30738-17.htm") {
			t.Fatalf("30738-17.htm: got %q", got)
		}
		q334Row(t, w, "wish", "3")
		q334Row(t, w, "exchange", "3")
		if n := w.srv.PlayerItemCount(t, w.player, q334WishPotion); n != 1 {
			t.Fatalf("%d potions after the second wish, want 1", n)
		}
		draws.queue(0)
		wantLines(t, "one first line", w.advance(t, 3*time.Second), say("matild", q334Matild, 33415))
		w.advance(t, 8*time.Second)
		q334Row(t, w, "exchange", "0")
		if n := w.srv.PlayerItemCount(t, w.player, 3469); n != 1 {
			t.Fatalf("%d certificates, want 1 from the one wish", n)
		}
	})
}
