package quest

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q615"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// What the Magical Power of Fire, part 1 (Q615) scenarios cannot show: a
// script-spawned NPC appearing, speaking and leaving, or being killed (#4020
// item 5). The dialogs, the theft and the trade are
// q615_magical_power_of_fire_part1_*.scenario. Expectations are derived
// from Q615_MagicalPowerOfFire_Part1.onSeeCreature and onMyDying, and
// Quest.addSpawn. The soldier is a warrior of the quest's see-creature set;
// the sighting helpers are those of q609_flow_test.go.

const (
	q615Name = "Q615_MagicalPowerOfFire_Part1"
	q615Path = "quest.Q615_MagicalPowerOfFire_Part1"

	asefaEyeID      = 31685
	sayAsefaCalled  = 61503
	sayAsefaSawFace = 61504
	asefaEyeTimeout = 10 * time.Second
)

// q615World is a level 74 Seeker on Q615 at cond 2 with the flag spawned
// seedSpawned (when started), beside a soldier 3000 away that cannot see it
// yet. eyeKind is the kind the quest binds Asefa's Eye as.
func q615World(t *testing.T, started bool, seedSpawned string, eyeKind script.NPCKind) (*dialogWorld, *npc.Hostile) {
	t.Helper()
	w := bootSeamQuest(t, 74, nil, func(srv *gameservertest.Server, objID int32) {
		if started {
			insertJournal(t, srv.DB,
				journalRow{objID, q615Name, "<state>", val("STARTED")},
				journalRow{objID, q615Name, "<cond>", val("2")},
				journalRow{objID, q615Name, "spawned", val(seedSpawned)},
			)
		}
	},
		gameservertest.WithAITask(),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{proofMonster(varkaSoldierID), asefaEyeTemplate(eyeKind)})),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{varkaSoldierID: script.KindHostile, asefaEyeID: eyeKind},
			[]script.Listing{{Path: q615Path}},
			script.Catalog{q615Path: q615.New},
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

func asefaEyeTemplate(kind script.NPCKind) *npc.Template {
	if kind == script.KindHostile {
		return proofMonster(asefaEyeID)
	}
	return gameservertest.FolkTemplate("Folk", asefaEyeID)
}

// TestQ615EyeAppearsBesideTheThiefOnce: a soldier that sees the player at
// cond 2 with the flag clear sets the flag, and calls Asefa's Eye within 100
// of the player: it says its line, then the give-up sound plays, and it
// leaves after 10 s. The flag then keeps the soldier from calling a second
// one when it sees the player again.
func TestQ615EyeAppearsBesideTheThiefOnce(t *testing.T) {
	t.Parallel()
	w, soldier := q615World(t, true, "0", script.KindFolk)
	frames := w.approachSoldier(t, soldier)

	eyes := npcsOf(w.srv, asefaEyeID)
	if len(eyes) != 1 {
		t.Fatalf("%d Asefa's Eyes after the sighting, want 1", len(eyes))
	}
	px, py, _ := w.srv.PlayerPosition(t, w.player)
	at := eyes[0].SpawnLocation()
	if abs(at.X-px) > 100 || abs(at.Y-py) > 100 {
		t.Errorf("Asefa's Eye at %d,%d, want within 100 of the player at %d,%d", at.X, at.Y, px, py)
	}
	w.roles[eyes[0].ObjectID()] = "eye"
	lines := w.render(t, frames)
	called, giveUp := slices.Index(lines, say("eye", asefaEyeID, sayAsefaCalled)),
		slices.Index(lines, "S PlaySound type=0 file=ItemSound.quest_giveup bind=0 obj=0 loc=0,0,0 delay=0")
	if called < 0 || giveUp < 0 || called > giveUp {
		t.Fatalf("lines %q: the eye's line at %d and the give-up sound at %d, want both, the line first", lines, called, giveUp)
	}
	if rows := questRows(t, w.srv, w.player, q615Name); rows["spawned"] != "1" || rows["<cond>"] != "2" {
		t.Fatalf("rows %v after the sighting, want spawned 1 at cond 2", rows)
	}

	w.leaveSoldier(t, soldier)
	if frames := w.approachSoldier(t, soldier); len(w.render(t, frames)) != 0 || len(npcsOf(w.srv, asefaEyeID)) != 1 {
		t.Fatalf("a second sighting called another eye: %d standing", len(npcsOf(w.srv, asefaEyeID)))
	}

	w.advance(t, asefaEyeTimeout-time.Second)
	if n := len(npcsOf(w.srv, asefaEyeID)); n != 1 {
		t.Fatalf("%d eyes 9 s on, want 1", n)
	}
	w.advance(t, 2*time.Second)
	if n := len(npcsOf(w.srv, asefaEyeID)); n != 0 {
		t.Fatalf("%d eyes 11 s on, want 0", n)
	}
}

// TestQ615NoEyeUnlessTheThiefIsAtCond2WithTheFlagClear: a soldier that sees
// a player with the flag already set, or with no Q615 at all, calls no eye.
func TestQ615NoEyeUnlessTheThiefIsAtCond2WithTheFlagClear(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		started bool
		flag    string
	}{{"flag set", true, "1"}, {"no quest", false, ""}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, soldier := q615World(t, c.started, c.flag, script.KindFolk)
			frames := w.approachSoldier(t, soldier)
			if n := len(npcsOf(w.srv, asefaEyeID)); n != 0 {
				t.Fatalf("%d eyes called, want none", n)
			}
			if lines := w.render(t, frames); len(lines) != 0 {
				t.Fatalf("lines %q, want none", lines)
			}
		})
	}
}

// TestQ615EyeSlainSaysItSawTheFaceForAStartedQuest: Asefa's Eye, killed by a
// player on Q615 (any cond, here 2), says that Asefa has seen the face;
// killed by a player without the quest it says nothing.
func TestQ615EyeSlainSaysItSawTheFaceForAStartedQuest(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name    string
		started bool
		want    bool
	}{{"started", true, true}, {"no quest", false, false}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, _ := q615World(t, c.started, "0", script.KindHostile)
			x, y, z := w.srv.PlayerPosition(t, w.player)
			eye := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(asefaEyeID), location.Location{X: x + 40, Y: y, Z: z})
			w.roles[eye.ObjectID()] = "eye"
			killOnPlayerQueue(t, w.srv, w.player, eye)
			lines := w.render(t, w.srv.ReadQueued(t, w.srv.Client))
			if got := slices.Contains(lines, say("eye", asefaEyeID, sayAsefaSawFace)); got != c.want {
				t.Fatalf("lines %q: the eye's line = %v, want %v", lines, got, c.want)
			}
		})
	}
}
