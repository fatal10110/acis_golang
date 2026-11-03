package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q227"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Test of the Reformer's called NPCs (Q227_TestOfTheReformer onAdvEvent
// "30669-03.htm" and "30670-03.htm", onTalk of Katari, onAttacked, onTimer,
// onDecayed, onMyDying): Kakan calls a pilgrim and the Crimson werewolf,
// Nyakuri a pilgrim and the Krudel lizardman, Katari a pilgrim and the
// inspector, then the betrayer, each one at a time until it leaves; the
// werewolf leaves a second after a hit that is no allowed skill; the
// Nameless revenant's seventh diary calls the Aruraune. A scenario cannot
// show a script-spawned NPC appearing or leaving, nor act on one (#4020
// item 5), so these Boot tests pin them; the scenarios play the dialogs.
const (
	q227Name         = "Q227_TestOfTheReformer"
	q227Path         = "quest.Q227_TestOfTheReformer"
	q227Katari       = 30668
	q227Kakan        = 30669
	q227Nyakuri      = 30670
	q227Pilgrim      = 30732
	q227Revenant     = 27099
	q227Aruraune     = 27128
	q227Inspector    = 27129
	q227Betrayer     = 27130
	q227Werewolf     = 27131
	q227Krudel       = 27132
	q227RippedDiary  = 2831
	q227HugeNail     = 2832
	q227OlMahumMoney = 2826
)

// Where the dialogs call each NPC.
var (
	q227KakanPilgrimAt   = [3]int{-9282, -89975, -2331}
	q227WerewolfAt       = [3]int{-9382, -89852, -2333}
	q227NyakuriPilgrimAt = [3]int{125947, -180049, -1778}
	q227KrudelAt         = [3]int{126019, -179983, -1781}
	q227KatariPilgrimAt  = [3]int{-4015, 40141, -3664}
	q227InspectorAt      = [3]int{-4034, 40201, -3665}
	q227BetrayerAt       = [3]int{-4106, 40174, -3660}
)

// q227World boots a level 39 character whose Test of the Reformer is at
// cond, holding hold[id] of each item, with Katari, Kakan and Nyakuri
// standing by it.
func q227World(t *testing.T, cond string, hold map[int32]int32) *dialogWorld {
	t.Helper()
	pages := chatPages(questPages(q227Name, map[string][]string{
		"30668-01.htm": nil,
		"30668-02.htm": nil,
		"30669-01.htm": {"30669-03.htm"},
		"30670-01.htm": {"30670-03.htm"},
		"30732-01.htm": nil,
	}), q227Katari, q227Kakan, q227Nyakuri, q227Pilgrim)
	w := bootSeamQuest(t, 39, pages, func(srv *gameservertest.Server, objID int32) {
		insertJournal(t, srv.DB,
			journalRow{objID, q227Name, "<state>", val("STARTED")},
			journalRow{objID, q227Name, "<cond>", val(cond)})
		for id, count := range hold {
			srv.GiveItem(t, objID, id, count)
		}
	},
		x1Items([]int32{q227RippedDiary, q227HugeNail, q227OlMahumMoney}),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			proofMonster(q227Revenant), proofMonster(q227Aruraune), proofMonster(q227Inspector),
			proofMonster(q227Betrayer), proofMonster(q227Werewolf), proofMonster(q227Krudel),
			gameservertest.FolkTemplate("Folk", q227Katari), gameservertest.FolkTemplate("Folk", q227Kakan),
			gameservertest.FolkTemplate("Folk", q227Nyakuri), gameservertest.FolkTemplate("Folk", q227Pilgrim),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{
				q227Revenant: script.KindHostile, q227Aruraune: script.KindHostile, q227Inspector: script.KindHostile,
				q227Betrayer: script.KindHostile, q227Werewolf: script.KindHostile, q227Krudel: script.KindHostile,
				q227Katari: script.KindFolk, q227Kakan: script.KindFolk, q227Nyakuri: script.KindFolk,
				q227Pilgrim: script.KindFolk,
				// The rest of the quest's talkers and monsters, so its bindings register.
				30118: script.KindFolk, 30666: script.KindFolk, 30667: script.KindFolk,
				20022: script.KindHostile, 20100: script.KindHostile, 20102: script.KindHostile,
				20104: script.KindHostile, 20404: script.KindHostile,
			},
			[]script.Listing{{Path: q227Path}},
			script.Catalog{q227Path: q227.New},
		),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "katari", q227Katari, 40, 0, 0)
	w.spawnFolk(t, "kakan", q227Kakan, -40, 0, 0)
	w.spawnFolk(t, "nyakuri", q227Nyakuri, 0, 40, 0)
	return w
}

// calledHostile returns the one hostile NPC of npc id id the dialogs called
// at the spot at, named role in the rendered packets.
func (w *dialogWorld) calledHostile(t *testing.T, role string, id int32, at [3]int) *npc.Hostile {
	t.Helper()
	called := calledAt(w, id, at)
	if len(called) != 1 {
		t.Fatalf("%d NPCs %d at %v, want 1", len(called), id, at)
	}
	for _, obj := range w.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.ObjectID() == called[0].ObjectID() {
			w.roles[h.ObjectID()], w.npcs[role] = role, h.ObjectID()
			return h
		}
	}
	t.Fatalf("NPC %d at %v is no hostile NPC", id, at)
	return nil
}

// calledFolk is calledHostile for a civilian NPC.
func (w *dialogWorld) calledFolk(t *testing.T, role string, id int32, at [3]int) *npc.Folk {
	t.Helper()
	called := calledAt(w, id, at)
	if len(called) != 1 {
		t.Fatalf("%d NPCs %d at %v, want 1", len(called), id, at)
	}
	for _, obj := range w.srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.ObjectID() == called[0].ObjectID() {
			w.roles[f.ObjectID()], w.npcs[role] = role, f.ObjectID()
			return f
		}
	}
	t.Fatalf("NPC %d at %v is no civilian NPC", id, at)
	return nil
}

// strike lands a hit of the player on h, with the skill def when it has an
// ID.
func (w *dialogWorld) strike(t *testing.T, h *npc.Hostile, def modelskill.Definition) {
	t.Helper()
	p := playerCombatant(t, w.srv, w.player)
	onPlayerQueue(t, w.srv, w.player, func() { h.ReduceHP(1, p, def) })
}

// slay lands a lethal hit of the player on h, lets the dying hooks' three
// seconds pass and renders what the player was sent.
func (w *dialogWorld) slay(t *testing.T, h *npc.Hostile) []string {
	t.Helper()
	p := playerCombatant(t, w.srv, w.player)
	onPlayerQueue(t, w.srv, w.player, func() { h.TakeDamage(h.MaxHP()+1, p) })
	return w.advance(t, 3500*time.Millisecond)
}

// stepTo puts the player 50 east of at, inside the 1500 a quest's hooks
// require of a player and the NPC it fights.
func (w *dialogWorld) stepTo(t *testing.T, at [3]int) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	if err := w.srv.State.Move(obj, at[0]+50, at[1], at[2]); err != nil {
		t.Fatal(err)
	}
	w.srv.ReadQueued(t, w.srv.Client)
}

// slayWith is slay with a lethal hit of the skill def.
func (w *dialogWorld) slayWith(t *testing.T, h *npc.Hostile, def modelskill.Definition) []string {
	t.Helper()
	p := playerCombatant(t, w.srv, w.player)
	onPlayerQueue(t, w.srv, w.player, func() { h.ReduceHP(float64(h.MaxHP()+1), p, def) })
	return w.advance(t, 3500*time.Millisecond)
}

// home puts the player back where it entered the world.
func (w *dialogWorld) home(t *testing.T) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	if err := w.srv.State.Move(obj, w.at.X, w.at.Y, w.at.Z); err != nil {
		t.Fatal(err)
	}
	w.srv.ReadQueued(t, w.srv.Client)
}

// wantQ227Cond fails unless the quest's cond is want.
func (w *dialogWorld) wantQ227Cond(t *testing.T, step, want string) {
	t.Helper()
	if rows := questRows(t, w.srv, w.player, q227Name); rows["<cond>"] != want {
		t.Fatalf("%s: rows %v, want cond %s", step, rows, want)
	}
}

// q227Event talks to the NPC role, expecting its page, and sends the quest's
// bypass event.
func (w *dialogWorld) q227Event(t *testing.T, role, page, event string) []string {
	t.Helper()
	if got := w.talk(t, role); !shows(got, page) {
		t.Fatalf("%s: got %q, want %s", role, got, page)
	}
	return w.send(t, encodeBypass("Quest "+q227Name+" "+event))
}

// TestQ227KakanCallsThePilgrimAndTheWerewolf: Kakan's dialog sets cond 12
// and calls an Ol Mahum pilgrim and the Crimson werewolf at their spots, 300
// s, with the werewolf set on the pilgrim; while they stand the dialog calls
// no others and sets the werewolf on the pilgrim again; once both have gone
// it calls new ones.
func TestQ227KakanCallsThePilgrimAndTheWerewolf(t *testing.T) {
	t.Parallel()
	w := q227World(t, "11", nil)
	w.q227Event(t, "kakan", "30669-01.htm", "30669-03.htm")
	w.wantQ227Cond(t, "the call", "12")
	pilgrim := w.calledFolk(t, "pilgrim", q227Pilgrim, q227KakanPilgrimAt)
	werewolf := w.calledHostile(t, "werewolf", q227Werewolf, q227WerewolfAt)
	for _, n := range []*script.NPC{script.NPCOf(pilgrim), script.NPCOf(werewolf)} {
		// The call keeps the heading 0 and no scatter.
		if h := n.SpawnLocation().Heading; h != 0 {
			t.Fatalf("NPC %d has heading %d, want 0", n.NpcID(), h)
		}
	}
	if hated := script.NPCOf(werewolf).MostHated(); hated == nil || hated.ObjectID() != pilgrim.ObjectID() {
		t.Fatalf("the werewolf hates %v most, want the pilgrim", hated)
	}

	// The dialog again calls nothing and sets the werewolf on the pilgrim.
	w.q227Event(t, "kakan", "30669-01.htm", "30669-03.htm")
	if n := len(npcsOf(w.srv, q227Pilgrim)); n != 1 {
		t.Fatalf("%d pilgrims after the second call, want 1", n)
	}
	if n := len(npcsOf(w.srv, q227Werewolf)); n != 1 {
		t.Fatalf("%d werewolves after the second call, want 1", n)
	}
	if hated := script.NPCOf(werewolf).MostHated(); hated == nil || hated.ObjectID() != pilgrim.ObjectID() {
		t.Fatalf("the werewolf hates %v most after the second call, want the pilgrim", hated)
	}

	w.advance(t, 299*time.Second)
	if n := len(npcsOf(w.srv, q227Werewolf)); n != 1 {
		t.Fatalf("%d werewolves 299 s on, want 1", n)
	}
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q227Werewolf)) + len(npcsOf(w.srv, q227Pilgrim)); n != 0 {
		t.Fatalf("%d called NPCs 301 s on, want 0", n)
	}
	w.q227Event(t, "kakan", "30669-01.htm", "30669-03.htm")
	if n := len(calledAt(w, q227Werewolf, q227WerewolfAt)) + len(calledAt(w, q227Pilgrim, q227KakanPilgrimAt)); n != 2 {
		t.Fatalf("%d called NPCs after the leave, want 2", n)
	}
}

// TestQ227NyakuriCallsThePilgrimAndTheLizardman: Nyakuri's dialog sets cond
// 15 and calls an Ol Mahum pilgrim and the Krudel lizardman at their spots,
// 300 s, with the lizardman set on the pilgrim; the called lizardman's death
// is the quest's, as any other lizardman's.
func TestQ227NyakuriCallsThePilgrimAndTheLizardman(t *testing.T) {
	t.Parallel()
	w := q227World(t, "14", nil)
	w.q227Event(t, "nyakuri", "30670-01.htm", "30670-03.htm")
	w.wantQ227Cond(t, "the call", "15")
	pilgrim := w.calledFolk(t, "pilgrim", q227Pilgrim, q227NyakuriPilgrimAt)
	krudel := w.calledHostile(t, "krudel", q227Krudel, q227KrudelAt)
	if hated := script.NPCOf(krudel).MostHated(); hated == nil || hated.ObjectID() != pilgrim.ObjectID() {
		t.Fatalf("the lizardman hates %v most, want the pilgrim", hated)
	}
	w.q227Event(t, "nyakuri", "30670-01.htm", "30670-03.htm")
	if n := len(npcsOf(w.srv, q227Krudel)) + len(npcsOf(w.srv, q227Pilgrim)); n != 2 {
		t.Fatalf("%d called NPCs after the second call, want 2", n)
	}

	w.stepTo(t, q227KrudelAt)
	w.slay(t, krudel)
	w.wantQ227Cond(t, "the lizardman's death", "16")

	w.advance(t, 301*time.Second)
	if n := len(npcsOf(w.srv, q227Pilgrim)); n != 0 {
		t.Fatalf("%d pilgrims 301 s on, want 0", n)
	}
}

// TestQ227KatariCallsTheInspectorAndTheBetrayer: Katari's dialog at cond 5
// takes Sla's letter, sets cond 6 and calls an Ol Mahum pilgrim and the
// inspector, 300 s, with the inspector set on the pilgrim (the dialog at
// cond 6 and 7 calls no more); the inspector's death sets cond 7, the
// pilgrim's dialog cond 8 and the Ol Mahum money; Katari at cond 8 calls the
// betrayer, in its run stance, 60 s; its death sets cond 9. (Its walk to
// (-7732, 36787, -3709) needs the geodata the Boot world has none of.)
func TestQ227KatariCallsTheInspectorAndTheBetrayer(t *testing.T) {
	t.Parallel()
	w := q227World(t, "5", nil)
	if got := w.talk(t, "katari"); !shows(got, "30668-01.htm") {
		t.Fatalf("Katari: got %q, want 30668-01.htm", got)
	}
	w.wantQ227Cond(t, "Katari's dialog", "6")
	pilgrim := w.calledFolk(t, "pilgrim", q227Pilgrim, q227KatariPilgrimAt)
	inspector := w.calledHostile(t, "inspector", q227Inspector, q227InspectorAt)
	if hated := script.NPCOf(inspector).MostHated(); hated == nil || hated.ObjectID() != pilgrim.ObjectID() {
		t.Fatalf("the inspector hates %v most, want the pilgrim", hated)
	}
	if got := w.talk(t, "katari"); !shows(got, "30668-01.htm") {
		t.Fatalf("Katari: got %q, want 30668-01.htm", got)
	}
	if n := len(npcsOf(w.srv, q227Inspector)) + len(npcsOf(w.srv, q227Pilgrim)); n != 2 {
		t.Fatalf("%d called NPCs after the second dialog, want 2", n)
	}

	w.stepTo(t, q227InspectorAt)
	w.slay(t, inspector)
	w.wantQ227Cond(t, "the inspector's death", "7")
	if got := w.talk(t, "pilgrim"); !shows(got, "30732-01.htm") {
		t.Fatalf("the pilgrim: got %q, want 30732-01.htm", got)
	}
	w.wantQ227Cond(t, "the pilgrim's dialog", "8")
	if n := w.srv.PlayerItemCount(t, w.player, q227OlMahumMoney); n != 1 {
		t.Fatalf("Ol Mahum money %d, want 1", n)
	}
	w.home(t)

	if got := w.talk(t, "katari"); !shows(got, "30668-02.htm") {
		t.Fatalf("Katari: got %q, want 30668-02.htm", got)
	}
	betrayer := w.calledHostile(t, "betrayer", q227Betrayer, q227BetrayerAt)
	if !betrayer.Running() {
		t.Fatal("the betrayer walks, want it to run")
	}
	w.home(t)
	if got := w.talk(t, "katari"); !shows(got, "30668-02.htm") {
		t.Fatalf("Katari: got %q, want 30668-02.htm", got)
	}
	if n := len(npcsOf(w.srv, q227Betrayer)); n != 1 {
		t.Fatalf("%d betrayers after the second dialog, want 1", n)
	}
	w.advance(t, 59*time.Second)
	if n := len(npcsOf(w.srv, q227Betrayer)); n != 1 {
		t.Fatalf("%d betrayers 59 s on, want 1", n)
	}
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q227Betrayer)); n != 0 {
		t.Fatalf("%d betrayers 61 s on, want 0", n)
	}

	// Gone, it is called again; its death sets cond 9.
	w.talk(t, "katari")
	again := w.calledHostile(t, "betrayer", q227Betrayer, q227BetrayerAt)
	w.stepTo(t, q227BetrayerAt)
	w.slay(t, again)
	w.wantQ227Cond(t, "the betrayer's death", "9")
}

// TestQ227RevenantCallsTheAruraune: a Nameless revenant hit with the skill
// 1031 by a player on the diary errand, whose death gives the seventh ripped
// diary (taken back at once, cond 2), leaves an Aruraune where it fell for
// 300 s; hit by anything else it leaves none and gives no diary. The
// Aruraune's death sets cond 3, gives the Huge Nail and makes it speak.
func TestQ227RevenantCallsTheAruraune(t *testing.T) {
	t.Parallel()
	w := q227World(t, "1", map[int32]int32{q227RippedDiary: 6})
	at := w.at
	revenant := func(dx int) *npc.Hostile {
		h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q227Revenant), location.Location{X: at.X + dx, Y: at.Y + 60, Z: at.Z})
		w.roles[h.ObjectID()] = "revenant"
		return h
	}

	// An unmarked revenant gives no diary.
	plain := revenant(0)
	w.strike(t, plain, modelskill.Definition{})
	w.slay(t, plain)
	if n := w.srv.PlayerItemCount(t, w.player, q227RippedDiary); n != 6 {
		t.Fatalf("ripped diaries %d after an unmarked revenant, want 6", n)
	}
	other := revenant(10)
	w.strike(t, other, modelskill.Definition{ID: 1040, Level: 1})
	w.slay(t, other)
	if n := w.srv.PlayerItemCount(t, w.player, q227RippedDiary); n != 6 {
		t.Fatalf("ripped diaries %d after a revenant hit by another skill, want 6", n)
	}
	if n := len(npcsOf(w.srv, q227Aruraune)); n != 0 {
		t.Fatalf("%d Aruraunes after the unmarked revenants, want 0", n)
	}

	marked := revenant(20)
	x, y, z := marked.Position()
	w.strike(t, marked, modelskill.Definition{ID: 1031, Level: 1})
	w.slay(t, marked)
	w.wantQ227Cond(t, "the marked revenant's death", "2")
	if n := w.srv.PlayerItemCount(t, w.player, q227RippedDiary); n != 0 {
		t.Fatalf("ripped diaries %d after the seventh, want 0", n)
	}
	aruraunes := npcsOf(w.srv, q227Aruraune)
	if len(aruraunes) != 1 {
		t.Fatalf("%d Aruraunes called, want 1", len(aruraunes))
	}
	if loc := aruraunes[0].SpawnLocation(); loc.X != x || loc.Y != y || abs(loc.Z-z) > 32 {
		t.Fatalf("the Aruraune stands at %+v, want the revenant's (%d, %d, %d)", loc, x, y, z)
	}

	w.advance(t, 299*time.Second)
	if n := len(npcsOf(w.srv, q227Aruraune)); n != 1 {
		t.Fatalf("%d Aruraunes 299 s on, want 1", n)
	}
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q227Aruraune)); n != 0 {
		t.Fatalf("%d Aruraunes 301 s on, want 0", n)
	}
}

// TestQ227AruraunesDeathSpeaksAndGivesTheNail: the Aruraune's death at cond
// 2 sets cond 3, gives the Huge Nail and makes it say NpcString 22719; at
// another cond it does neither.
func TestQ227AruraunesDeathSpeaksAndGivesTheNail(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, cond string
		speaks     bool
	}{
		{"cond 2", "2", true},
		{"cond 3", "3", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q227World(t, tc.cond, nil)
			h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q227Aruraune), location.Location{X: w.at.X, Y: w.at.Y + 60, Z: w.at.Z})
			w.roles[h.ObjectID()] = "aruraune"
			lines := w.slay(t, h)
			said := say("aruraune", q227Aruraune, 22719)
			if got := slices.Contains(lines, said); got != tc.speaks {
				t.Fatalf("lines %q, spoke %v, want %v", lines, got, tc.speaks)
			}
			nails := w.srv.PlayerItemCount(t, w.player, q227HugeNail)
			if tc.speaks != (nails == 1) {
				t.Fatalf("huge nails %d, speaks %v", nails, tc.speaks)
			}
		})
	}
}

// TestQ227WerewolfLeavesAfterAHit: at cond 12 the first hit on the called
// Crimson werewolf that is no allowed skill (none, or any but 1031, 1069,
// 1164, 1168, 1147, 1177, 1184, 1201 and 1206) marks it and, a second
// later, makes it say NpcString 22720, stop its attack and leave; a marked
// werewolf's death moves the quest on no more. An allowed skill leaves it
// to its death, which sets cond 13.
func TestQ227WerewolfLeavesAfterAHit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		skill modelskill.Definition
		stays bool
	}{
		{"plain hit", modelskill.Definition{}, false},
		{"other skill", modelskill.Definition{ID: 1040, Level: 1}, false},
		{"allowed skill", modelskill.Definition{ID: 1031, Level: 1}, true},
		{"last allowed skill", modelskill.Definition{ID: 1206, Level: 1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := q227World(t, "11", nil)
			w.q227Event(t, "kakan", "30669-01.htm", "30669-03.htm")
			werewolf := w.calledHostile(t, "werewolf", q227Werewolf, q227WerewolfAt)
			w.stepTo(t, q227WerewolfAt)
			w.strike(t, werewolf, tc.skill)
			w.advance(t, 999*time.Millisecond)
			if n := len(npcsOf(w.srv, q227Werewolf)); n != 1 {
				t.Fatalf("%d werewolves 999 ms after the hit, want 1", n)
			}
			lines := w.advance(t, 2*time.Millisecond)
			said := say("werewolf", q227Werewolf, 22720)
			if tc.stays {
				if slices.Contains(lines, said) || len(npcsOf(w.srv, q227Werewolf)) != 1 {
					t.Fatalf("the werewolf left (lines %q), want it to stay", lines)
				}
				w.slayWith(t, werewolf, tc.skill)
				w.wantQ227Cond(t, "the werewolf's death", "13")
				return
			}
			if !slices.Contains(lines, said) {
				t.Fatalf("lines %q, want %q", lines, said)
			}
			if n := len(npcsOf(w.srv, q227Werewolf)); n != 0 {
				t.Fatalf("%d werewolves after the leave, want 0", n)
			}
			w.wantQ227Cond(t, "the leave", "12")
			// It is free to be called again.
			w.home(t)
			w.q227Event(t, "kakan", "30669-01.htm", "30669-03.htm")
			if n := len(calledAt(w, q227Werewolf, q227WerewolfAt)); n != 1 {
				t.Fatalf("%d werewolves called after the leave, want 1", n)
			}
		})
	}
}

// TestQ227OnlyTheCalledWerewolfLeaves: a werewolf that is not the called one
// is marked by a hit, yet stays: the timer leaves only the called one.
func TestQ227OnlyTheCalledWerewolfLeaves(t *testing.T) {
	t.Parallel()
	w := q227World(t, "12", nil)
	h := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(q227Werewolf), location.Location{X: w.at.X, Y: w.at.Y + 60, Z: w.at.Z})
	w.roles[h.ObjectID()] = "werewolf"
	w.strike(t, h, modelskill.Definition{})
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, q227Werewolf)); n != 1 {
		t.Fatalf("%d werewolves after the hit, want 1", n)
	}
	// Marked, its death moves the quest on no more.
	w.slay(t, h)
	w.wantQ227Cond(t, "the marked werewolf's death", "12")
}

// TestQ227HitPilgrimTurnsOnItsAttacker: a called pilgrim hit by the player
// turns on it, whatever the player's quest step.
func TestQ227HitPilgrimTurnsOnItsAttacker(t *testing.T) {
	t.Parallel()
	w := q227World(t, "11", nil)
	w.q227Event(t, "kakan", "30669-01.htm", "30669-03.htm")
	pilgrim := w.calledFolk(t, "pilgrim", q227Pilgrim, q227KakanPilgrimAt)
	w.stepTo(t, q227KakanPilgrimAt)
	p := playerCombatant(t, w.srv, w.player)
	onPlayerQueue(t, w.srv, w.player, func() { pilgrim.TakeDamage(1, p) })
	if got := script.NPCOf(pilgrim).Intention(); got != script.IntentionAttack {
		t.Fatalf("the pilgrim's intention is %v after the hit, want attacking", got)
	}
}
