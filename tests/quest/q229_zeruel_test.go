package quest

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q229"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Test of Witchcraft's called NPCs (Q229_TestOfWitchcraft onAdvEvent
// "30630-14.htm" and "30633-02.htm", onTalk of Orim and Evert, onAttacked,
// onTimer, onDecayed, onMyDying): Orim calls the Drevanul Prince Zeruel for
// 120 s and Evert another for 300 s, each one at a time until it leaves; a
// hit on Orim's moves the test on and makes it leave a second later; Evert's
// records a player who hits it with the Sword of Binding, and only that
// player's kill moves the test on. A scenario cannot show a script-spawned
// NPC appearing or leaving, nor act on one (#4020 item 5), so these Boot
// tests pin them; the scenarios play the dialogs.
const (
	q229Name       = "Q229_TestOfWitchcraft"
	q229Path       = "quest.Q229_TestOfWitchcraft"
	q229Orim       = 30630
	q229Evert      = 30633
	q229Revenant   = 27099
	q229Skeletal   = 27100
	q229Zeruel     = 27101
	q229Sword      = 3029
	q229LaraMemo   = 3314
	q229Brimstone2 = 3335
	q229Soultrap   = 3332
	q229KeyItem    = 3333
	q229Crystal    = 3334
)

// Where the dialogs call each Zeruel.
var (
	q229OrimZeruelAt  = [3]int{70381, 109638, -3726}
	q229EvertZeruelAt = [3]int{13395, 169807, -3708}
)

// q229World boots a level 39 character whose Test of Witchcraft is at cond
// with the memo gem456 gem (when not empty), holding hold[id] of each item
// and wielding the Sword of Binding when sword, with Orim and Evert standing
// by it.
func q229World(t *testing.T, cond, gem string, hold map[int32]int32, sword bool) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q229Name, map[string][]string{
		"30630-11.htm": {"30630-14.htm"},
		"30630-14.htm": nil,
		"30630-15.htm": nil,
		"30633-01.htm": {"30633-02.htm"},
		"30633-02.htm": nil,
		"30633-03.htm": nil,
	}), q229Orim, q229Evert)
	weapon := &item.Template{
		ID: q229Sword, Name: "Sword of Binding", Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1,
		Destroyable: true, Weapon: &item.WeaponDetail{Type: item.WeaponDagger},
	}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{
		3308, 3309, 3310, 3311, 3312, 3313, 3314, 3315, 3316, 3317, 3318, 3319, 3320, 3321, 3322, 3323, 3324, 3325,
		3326, 3327, 3328, 3329, 3330, 3331, 3332, 3333, 3334, 3335,
	} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	templates = append(templates, weapon)
	w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
		rows := []journalRow{
			{objID, q229Name, "<state>", val("STARTED")},
			{objID, q229Name, "<cond>", val(cond)},
		}
		if gem != "" {
			rows = append(rows, journalRow{objID, q229Name, "gem456", val(gem)})
		}
		insertJournal(t, srv.DB, rows...)
		for id, count := range hold {
			srv.GiveItem(t, objID, id, count)
		}
		if sword {
			inst := item.NewStackOrEquip(srv.NewObjectID(), weapon, 1, true)
			if !inst.Equipped() {
				t.Fatal("the Sword of Binding is not worn")
			}
			inst.OwnerID = objID
			if err := srv.Items.Create(t.Context(), objID, inst); err != nil {
				t.Fatal(err)
			}
		}
	},
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(q229Revenant), proofMonster(q229Skeletal), proofMonster(q229Zeruel),
			gameservertest.FolkTemplate("Folk", q229Orim), gameservertest.FolkTemplate("Folk", q229Evert),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{
				q229Revenant: script.KindHostile, q229Skeletal: script.KindHostile, q229Zeruel: script.KindHostile,
				q229Orim: script.KindFolk, q229Evert: script.KindFolk,
				// The rest of the quest's talkers and monsters, so its bindings register.
				30063: script.KindFolk, 30098: script.KindFolk, 30110: script.KindFolk, 30188: script.KindFolk,
				30314: script.KindFolk, 30417: script.KindFolk, 30435: script.KindFolk, 30476: script.KindFolk,
				30631: script.KindFolk, 30632: script.KindFolk,
				20557: script.KindHostile, 20565: script.KindHostile, 20577: script.KindHostile,
				20578: script.KindHostile, 20579: script.KindHostile, 20580: script.KindHostile,
				20581: script.KindHostile, 20582: script.KindHostile, 20601: script.KindHostile,
				20602: script.KindHostile,
			},
			[]script.Listing{{Path: q229Path}},
			script.Catalog{q229Path: q229.New},
		),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "orim", q229Orim, 40, 0, 0)
	w.spawnFolk(t, "evert", q229Evert, -40, 0, 0)
	return w
}

// wantQ229Cond fails unless the quest's cond is want.
func (w *dialogWorld) wantQ229Cond(t *testing.T, step, want string) {
	t.Helper()
	if rows := questRows(t, w.srv, w.player, q229Name); rows["<cond>"] != want {
		t.Fatalf("%s: rows %v, want cond %s", step, rows, want)
	}
}

// q229Event talks to the NPC role, expecting its page, and sends the quest's
// bypass event.
func (w *dialogWorld) q229Event(t *testing.T, role, page, event string) []string {
	t.Helper()
	if got := w.talk(t, role); !shows(got, page) {
		t.Fatalf("%s: got %q, want %s", role, got, page)
	}
	return w.send(t, encodeBypass("Quest "+q229Name+" "+event))
}

// TestQ229OrimCallsOneZeruel: Orim's event "30630-14.htm" sets cond 4 and
// calls the Zeruel at its spot for 120 s; while it stands the dialog at cond
// 4 calls no other; once it has gone the dialog calls a new one.
func TestQ229OrimCallsOneZeruel(t *testing.T) {
	t.Parallel()
	w := q229World(t, "3", "6", nil, false)
	w.q229Event(t, "orim", "30630-11.htm", "30630-14.htm")
	w.wantQ229Cond(t, "the call", "4")
	zeruel := w.calledHostile(t, "zeruel", q229Zeruel, q229OrimZeruelAt)
	// The call keeps the heading 0 and no scatter.
	if h := script.NPCOf(zeruel).SpawnLocation().Heading; h != 0 {
		t.Fatalf("the Zeruel has heading %d, want 0", h)
	}
	if got := w.talk(t, "orim"); !shows(got, "30630-14.htm") {
		t.Fatalf("Orim: got %q, want 30630-14.htm", got)
	}
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 1 {
		t.Fatalf("%d Zeruels after the second dialog, want 1", n)
	}

	w.advance(t, 119*time.Second)
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 1 {
		t.Fatalf("%d Zeruels 119 s on, want 1", n)
	}
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 0 {
		t.Fatalf("%d Zeruels 121 s on, want 0", n)
	}
	if got := w.talk(t, "orim"); !shows(got, "30630-14.htm") {
		t.Fatalf("Orim: got %q, want 30630-14.htm", got)
	}
	if n := len(calledAt(w, q229Zeruel, q229OrimZeruelAt)); n != 1 {
		t.Fatalf("%d Zeruels called after the leave, want 1", n)
	}
}

// TestQ229ZeruelDeathFreesTheCall: a Zeruel Evert called that dies without the
// sword hit is gone by its 300 s and lets the dialog at cond 9 call a new one; its death
// moves the test on no more.
func TestQ229ZeruelDeathFreesTheCall(t *testing.T) {
	t.Parallel()
	w := q229World(t, "9", "", map[int32]int32{q229Soultrap: 1, q229Brimstone2: 1}, false)
	w.talk(t, "evert")
	zeruel := w.calledHostile(t, "zeruel", q229Zeruel, q229EvertZeruelAt)
	w.stepTo(t, q229EvertZeruelAt)
	w.slay(t, zeruel)
	w.wantQ229Cond(t, "the death", "9")
	w.advance(t, 301*time.Second)
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 0 {
		t.Fatalf("%d Zeruels after the corpse decayed, want 0", n)
	}
	w.home(t)
	w.talk(t, "evert")
	if n := len(calledAt(w, q229Zeruel, q229EvertZeruelAt)); n != 1 {
		t.Fatalf("%d Zeruels called after the decay, want 1", n)
	}
}

// TestQ229HitOnOrimsZeruel: the first hit on the Zeruel Orim called, at cond
// 4, sets cond 5, makes it say NpcString 22934 and, a second later, stop its
// attack and leave; the hit of a player at another cond does nothing.
func TestQ229HitOnOrimsZeruel(t *testing.T) {
	t.Parallel()
	w := q229World(t, "4", "", nil, false)
	w.talk(t, "orim")
	zeruel := w.calledHostile(t, "zeruel", q229Zeruel, q229OrimZeruelAt)
	w.stepTo(t, q229OrimZeruelAt)
	w.strike(t, zeruel, modelskill.Definition{})
	lines := w.advance(t, 999*time.Millisecond)
	if !slices.Contains(lines, say("zeruel", q229Zeruel, 22934)) {
		t.Fatalf("lines %q, want the Zeruel's 22934", lines)
	}
	w.wantQ229Cond(t, "the hit", "5")
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 1 {
		t.Fatalf("%d Zeruels 999 ms after the hit, want 1", n)
	}
	w.advance(t, 2*time.Millisecond)
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 0 {
		t.Fatalf("%d Zeruels after the leave, want 0", n)
	}
}

// TestQ229OnlyOrimsCalledZeruelLeaves: a Zeruel that is not the one Orim
// called is neither marked nor made to leave by a hit.
func TestQ229OnlyOrimsCalledZeruelLeaves(t *testing.T) {
	t.Parallel()
	w := q229World(t, "4", "", nil, false)
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q229Zeruel), location.Location{X: w.at.X, Y: w.at.Y + 60, Z: w.at.Z})
	w.roles[h.ObjectID()] = "zeruel"
	w.strike(t, h, modelskill.Definition{})
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 1 {
		t.Fatalf("%d Zeruels after the hit, want 1", n)
	}
	w.wantQ229Cond(t, "the hit", "4")
}

// TestQ229EvertCallsOneZeruel: Evert's event "30633-02.htm" sets cond 9,
// gives the second brimstone and calls the Zeruel at its spot for 300 s;
// while it stands the dialog at cond 9 calls no other; once it has gone the
// dialog calls a new one, which no timer removes after a hit.
func TestQ229EvertCallsOneZeruel(t *testing.T) {
	t.Parallel()
	w := q229World(t, "8", "", map[int32]int32{q229Soultrap: 1}, false)
	w.q229Event(t, "evert", "30633-01.htm", "30633-02.htm")
	w.wantQ229Cond(t, "the call", "9")
	if n := w.srv.PlayerItemCount(t, w.player, q229Brimstone2); n != 1 {
		t.Fatalf("second brimstones %d, want 1", n)
	}
	zeruel := w.calledHostile(t, "zeruel", q229Zeruel, q229EvertZeruelAt)
	if h := script.NPCOf(zeruel).SpawnLocation().Heading; h != 0 {
		t.Fatalf("the Zeruel has heading %d, want 0", h)
	}
	if got := w.talk(t, "evert"); !shows(got, "30633-02.htm") {
		t.Fatalf("Evert: got %q, want 30633-02.htm", got)
	}
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 1 {
		t.Fatalf("%d Zeruels after the second dialog, want 1", n)
	}
	w.advance(t, 299*time.Second)
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 1 {
		t.Fatalf("%d Zeruels 299 s on, want 1", n)
	}
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q229Zeruel)); n != 0 {
		t.Fatalf("%d Zeruels 301 s on, want 0", n)
	}
	w.talk(t, "evert")
	if n := len(calledAt(w, q229Zeruel, q229EvertZeruelAt)); n != 1 {
		t.Fatalf("%d Zeruels called after the leave, want 1", n)
	}
}

// TestQ229EvertsZeruelKnowsItsSwordBearer: a hit on the Zeruel Evert called
// with the Sword of Binding in the right hand records the player and makes
// it say NpcString 22935, and only that player's kill sets cond 10, gives the
// Purgatory key and the Zeruel bind crystal, takes the second brimstone and
// the soultrap crystal and makes it say 22936; without the sword the hit
// records nothing, and the Zeruel, never leaving after a hit, is a plain kill.
func TestQ229EvertsZeruelKnowsItsSwordBearer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		sword bool
	}{
		{"with the sword", true},
		{"without the sword", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q229World(t, "9", "", map[int32]int32{q229Soultrap: 1, q229Brimstone2: 1}, tc.sword)
			w.talk(t, "evert")
			zeruel := w.calledHostile(t, "zeruel", q229Zeruel, q229EvertZeruelAt)
			w.stepTo(t, q229EvertZeruelAt)
			w.strike(t, zeruel, modelskill.Definition{})
			lines := w.advance(t, 2*time.Second)
			if got := slices.Contains(lines, say("zeruel", q229Zeruel, 22935)); got != tc.sword {
				t.Fatalf("lines %q, spoke %v, want %v", lines, got, tc.sword)
			}
			w.wantQ229Cond(t, "the hit", "9")
			if n := len(npcsOf(w.srv, q229Zeruel)); n != 1 {
				t.Fatalf("%d Zeruels after the hit, want 1", n)
			}
			lines = w.slay(t, zeruel)
			killed := slices.Contains(lines, say("zeruel", q229Zeruel, 22936))
			if killed != tc.sword {
				t.Fatalf("lines %q, spoke %v, want %v", lines, killed, tc.sword)
			}
			want, held := "9", 0
			if tc.sword {
				want, held = "10", 1
			}
			w.wantQ229Cond(t, "the kill", want)
			for _, id := range []int32{q229KeyItem, q229Crystal} {
				if n := w.srv.PlayerItemCount(t, w.player, id); n != held {
					t.Fatalf("item %d x%d after the kill, want %d", id, n, held)
				}
			}
			for _, id := range []int32{q229Soultrap, q229Brimstone2} {
				if n := w.srv.PlayerItemCount(t, w.player, id); n != 1-held {
					t.Fatalf("item %d x%d after the kill, want %d", id, n, 1-held)
				}
			}
		})
	}
}

// TestQ229AnotherZeruelMovesNothing: a Zeruel Evert did not call, hit with the
// sword and killed by the player, records nothing and gives nothing.
func TestQ229AnotherZeruelMovesNothing(t *testing.T) {
	t.Parallel()
	w := q229World(t, "9", "", map[int32]int32{q229Soultrap: 1, q229Brimstone2: 1}, true)
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q229Zeruel), location.Location{X: w.at.X, Y: w.at.Y + 60, Z: w.at.Z})
	w.roles[h.ObjectID()] = "zeruel"
	w.strike(t, h, modelskill.Definition{})
	w.slay(t, h)
	w.wantQ229Cond(t, "the kill of another Zeruel", "9")
}

// TestQ229RevenantAndMercenarySpeakOnce: the Nameless revenant hit by a
// player holding Lara's memo, and the Skeletal mercenary hit by a player
// whose gem456 memo is 3 to 5, say NpcString 22933 once, whatever the hits
// after; with the memo or the item missing they stay silent.
func TestQ229RevenantAndMercenarySpeakOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, gem string
		hold      map[int32]int32
		monster   int
		speaks    bool
	}{
		{"revenant with the memo", "2", map[int32]int32{q229LaraMemo: 1}, q229Revenant, true},
		{"revenant without the memo", "2", nil, q229Revenant, false},
		{"mercenary at gem 3", "3", nil, q229Skeletal, true},
		{"mercenary at gem 5", "5", nil, q229Skeletal, true},
		{"mercenary at gem 2", "2", nil, q229Skeletal, false},
		{"mercenary at gem 6", "6", nil, q229Skeletal, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q229World(t, "2", tc.gem, tc.hold, false)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(tc.monster), location.Location{X: w.at.X, Y: w.at.Y + 60, Z: w.at.Z})
			w.roles[h.ObjectID()] = "monster"
			said := say("monster", int32(tc.monster), 22933)
			w.strike(t, h, modelskill.Definition{})
			first := w.advance(t, 100*time.Millisecond)
			w.strike(t, h, modelskill.Definition{})
			second := w.advance(t, 100*time.Millisecond)
			if got := slices.Contains(first, said); got != tc.speaks {
				t.Fatalf("first hit: lines %q, spoke %v, want %v", first, got, tc.speaks)
			}
			if slices.Contains(second, said) {
				t.Fatalf("second hit: lines %q, want no second line", second)
			}
		})
	}
}
