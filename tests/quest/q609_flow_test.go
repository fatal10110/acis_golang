package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q609"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the Magical Power of Water, part 1 (Q609) scenarios cannot show: a
// script-spawned NPC appearing, speaking and leaving, or being killed (#4020
// item 5). The dialogs, the theft and the trade are
// q609_magical_power_of_water_part1_*.scenario. Expectations are derived
// from Q609_MagicalPowerOfWater_Part1.onSeeCreature and onMyDying, and
// Quest.addSpawn.

const (
	q609Name = "Q609_MagicalPowerOfWater_Part1"
	q609Path = "quest.Q609_MagicalPowerOfWater_Part1"

	udanEyeID      = 31684
	sayEyeCalled   = 60903
	sayEyeSawFace  = 60904
	udanEyeTimeout = 10 * time.Second
)

// q609World is a level 74 Seeker on Q609 at cond 2 with the flag spawned
// seedSpawned (when started), beside a Varka soldier 3000 away that cannot
// see it yet. eyeKind is the kind the quest binds Udan's Eye as.
func q609World(t *testing.T, started bool, seedSpawned string, eyeKind script.NPCKind) (*dialogWorld, *npc.Hostile) {
	t.Helper()
	w := bootSeamQuest(t, 74, nil, func(srv *gameservertest.Server, objID int32) {
		if started {
			insertJournal(t, srv.DB,
				journalRow{objID, q609Name, "<state>", val("STARTED")},
				journalRow{objID, q609Name, "<cond>", val("2")},
				journalRow{objID, q609Name, "spawned", val(seedSpawned)},
			)
		}
	},
		gameservertest.WithAITask(),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(varkaSoldierID), udanEyeTemplate(eyeKind)})),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{varkaSoldierID: script.KindHostile, udanEyeID: eyeKind},
			[]script.Listing{{Path: q609Path}},
			script.Catalog{q609Path: q609.New},
		),
	)
	x, y, z := w.srv.PlayerPosition(t, w.player)
	soldier := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(varkaSoldierID), location.Location{X: x + 3000, Y: y, Z: z})
	w.srv.AI.Add(soldier)
	w.srv.RunCreated(t, soldier)
	w.roles[w.player] = "player"
	w.roles[soldier.ObjectID()] = "soldier"
	return w, soldier
}

func udanEyeTemplate(kind script.NPCKind) *npc.Template {
	if kind == script.KindHostile {
		return proofMonster(udanEyeID)
	}
	return gameservertest.FolkTemplate("Folk", udanEyeID)
}

// approachSoldier puts the player 100 in front of the soldier, on its -x
// side, and runs one AI tick, whose see-creature scan notices the player.
// It returns what the player was sent.
func (w *dialogWorld) approachSoldier(t *testing.T, soldier *npc.Hostile) [][]byte {
	t.Helper()
	w.moveTo(t, soldier.Position)
	if err := w.srv.AI.Tick(); err != nil {
		t.Fatal(err)
	}
	w.srv.Settle(t)
	return w.srv.ReadQueued(t, w.srv.Client)
}

// leaveSoldier puts the player 5000 behind the soldier, out of its sight, and
// runs one AI tick, whose scan forgets the player.
func (w *dialogWorld) leaveSoldier(t *testing.T, soldier *npc.Hostile) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := soldier.Position()
	if err := w.srv.State.Move(obj, x-5000, y, z); err != nil {
		t.Fatal(err)
	}
	w.srv.ReadQueued(t, w.srv.Client)
	if err := w.srv.AI.Tick(); err != nil {
		t.Fatal(err)
	}
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
}

// moveTo puts the player 100 in front of the place at() gives, on its -x side.
func (w *dialogWorld) moveTo(t *testing.T, at func() (int, int, int)) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := at()
	if err := w.srv.State.Move(obj, x-100, y, z); err != nil {
		t.Fatal(err)
	}
	w.srv.ReadQueued(t, w.srv.Client)
}

// TestQ609EyeAppearsBesideTheThiefOnce: a Varka soldier that sees the player
// at cond 2 with the flag clear sets the flag, and calls Udan's Eye within 100
// of the player: it says its line, then the give-up sound plays, and it
// leaves after 10 s. The flag then keeps the soldier from calling a second
// one when it sees the player again.
func TestQ609EyeAppearsBesideTheThiefOnce(t *testing.T) {
	t.Parallel()
	w, soldier := q609World(t, true, "0", script.KindFolk)
	frames := w.approachSoldier(t, soldier)

	eyes := npcsOf(w.srv, udanEyeID)
	if len(eyes) != 1 {
		t.Fatalf("%d Udan's Eyes after the sighting, want 1", len(eyes))
	}
	px, py, _ := w.srv.PlayerPosition(t, w.player)
	at := eyes[0].SpawnLocation()
	if abs(at.X-px) > 100 || abs(at.Y-py) > 100 {
		t.Errorf("Udan's Eye at %d,%d, want within 100 of the player at %d,%d", at.X, at.Y, px, py)
	}
	w.roles[eyes[0].ObjectID()] = "eye"
	lines := w.render(t, frames)
	called, giveUp := slices.Index(lines, say("eye", udanEyeID, sayEyeCalled)),
		slices.Index(lines, "S PlaySound type=0 file=ItemSound.quest_giveup bind=0 obj=0 loc=0,0,0 delay=0")
	if called < 0 || giveUp < 0 || called > giveUp {
		t.Fatalf("lines %q: the eye's line at %d and the give-up sound at %d, want both, the line first", lines, called, giveUp)
	}
	if rows := questRows(t, w.srv, w.player, q609Name); rows["spawned"] != "1" || rows["<cond>"] != "2" {
		t.Fatalf("rows %v after the sighting, want spawned 1 at cond 2", rows)
	}

	w.leaveSoldier(t, soldier)
	if frames := w.approachSoldier(t, soldier); len(w.render(t, frames)) != 0 || len(npcsOf(w.srv, udanEyeID)) != 1 {
		t.Fatalf("a second sighting called another eye: %d standing", len(npcsOf(w.srv, udanEyeID)))
	}

	w.advance(t, udanEyeTimeout-time.Second)
	if n := len(npcsOf(w.srv, udanEyeID)); n != 1 {
		t.Fatalf("%d eyes 9 s on, want 1", n)
	}
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, udanEyeID)); n != 0 {
		t.Fatalf("%d eyes 11 s on, want 0", n)
	}
}

// TestQ609NoEyeUnlessTheThiefIsAtCond2WithTheFlagClear: a soldier that sees
// a player with the flag already set, or with no Q609 at all, calls no eye.
func TestQ609NoEyeUnlessTheThiefIsAtCond2WithTheFlagClear(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		started bool
		flag    string
	}{{"flag set", true, "1"}, {"no quest", false, ""}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, soldier := q609World(t, c.started, c.flag, script.KindFolk)
			frames := w.approachSoldier(t, soldier)
			if n := len(npcsOf(w.srv, udanEyeID)); n != 0 {
				t.Fatalf("%d eyes called, want none", n)
			}
			if lines := w.render(t, frames); len(lines) != 0 {
				t.Fatalf("lines %q, want none", lines)
			}
		})
	}
}

// TestQ609EyeSlainSaysItSawTheFaceForAStartedQuest: Udan's Eye, killed by a
// player on Q609 (any cond, here 2), says that Udan has seen the face; killed
// by a player without the quest it says nothing.
func TestQ609EyeSlainSaysItSawTheFaceForAStartedQuest(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		started bool
		want    bool
	}{{"started", true, true}, {"no quest", false, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, _ := q609World(t, c.started, "0", script.KindHostile)
			x, y, z := w.srv.PlayerPosition(t, w.player)
			eye := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(udanEyeID), location.Location{X: x + 40, Y: y, Z: z})
			w.roles[eye.ObjectID()] = "eye"
			killOnPlayerQueue(t, w.srv, w.player, eye)
			lines := w.render(t, w.srv.ReadQueued(t, w.srv.Client))
			if got := slices.Contains(lines, say("eye", udanEyeID, sayEyeSawFace)); got != c.want {
				t.Fatalf("lines %q: the eye's line = %v, want %v", lines, got, c.want)
			}
		})
	}
}
