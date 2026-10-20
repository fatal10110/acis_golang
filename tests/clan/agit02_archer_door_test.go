package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// doomArcherID is the clan hall siege archer (Agit02DoomArcherAgit),
// spawned by Gludio's gate as one of "door_clan".
const doomArcherID = 35413

// gateArcherWorld is the siege summon gate world with the archer by the
// gate, bound to Agit02DoomArcherAgit.
func gateArcherWorld(t *testing.T) (*clanWorld, *npc.Hostile) {
	t.Helper()
	form, err := zone.NewCuboid(gateWorldAt.X-1000, gateWorldAt.X+1000, gateWorldAt.Y-1000, gateWorldAt.Y+1000,
		gateWorldAt.Z-1000, gateWorldAt.Z+1000)
	if err != nil {
		t.Fatal(err)
	}
	field := zone.NewSiege(90001, form)
	field.SetActive(true)
	zones := zone.NewIndex()
	zones.Add(field)
	const path = "script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgit"
	w := gateWorld(t,
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			siegeSummonTemplate(gateGolemNPCID, "Siege Golem"),
		})),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{doomArcherID: script.KindHostile},
			[]script.Listing{{Path: path}}, script.Catalog{path: warrior.Agit02DoomArcherAgit}),
	)
	archer := &npc.Template{
		ID: doomArcherID, TemplateID: doomArcherID, Type: "Monster", Name: "Doom Archer", Level: 40, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Clans: []string{"door_clan"}, ClanRange: 300,
	}
	h := w.srv.SpawnHostileNPCTemplateAt(t, archer, location.Location{X: gateWorldAt.X + 100, Y: gateWorldAt.Y + 200, Z: gateWorldAt.Z})
	return w, h
}

// TestAgit02DoomArcherAgitGuardsItsDoor ports Agit02DoomArcherAgit's
// static-object clan-attacked hook on the archer by Gludio's gate. The
// founder, attacking Gludio, hits the gate for 100: the archer wants to
// attack the founder by 50 for each twentieth of its maximum hit points
// the damage makes. A Siege Golem's hit on the gate makes it want to
// attack both the golem's owner, weight 5000, and the golem, weight 1000,
// with no draw.
func TestAgit02DoomArcherAgitGuardsItsDoor(t *testing.T) {
	t.Parallel()
	w, archer := gateArcherWorld(t)
	gate := gateObject(t, w, gludioGateID)
	obj, ok := w.srv.State.Object(w.leaderID)
	if !ok {
		t.Fatal("the founder is not in the world")
	}
	founder := obj.(attackable.Combatant)

	before := archer.AggroHate(founder)
	w.srv.HitDoor(t, gludioGateID, w.leaderID, 100)
	want := 100.0 / float64(archer.MaxHP()) / 0.05 * 50
	if got := archer.AggroHate(founder) - before; got != want {
		t.Fatalf("archer hate of the founder from its hit on the gate = %v, want %v", got, want)
	}

	golem := callSiegeSummon(t, w, w.leaderID, gateGolemNPCID)
	founderBefore, golemBefore := archer.AggroHate(founder), archer.AggroHate(golem)
	gate.ReduceHP(100, golem, modelskill.Definition{})
	if got := archer.AggroHate(founder) - founderBefore; got != 5000 {
		t.Fatalf("archer hate of the founder from the golem's hit = %v, want 5000", got)
	}
	if got := archer.AggroHate(golem) - golemBefore; got != 1000 {
		t.Fatalf("archer hate of the golem from its hit = %v, want 1000", got)
	}
}
