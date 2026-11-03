package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// vampireArcherID is the clan hall siege vampire (Agit02VampireArcherAgit),
// placed by Gludio's gate as one of "door_clan".
const vampireArcherID = 35634

// TestAgit02VampireArcherAgitGuardsItsDoor runs Agit02VampireArcherAgit's
// static-object clan-attacked hook on a vampire by Gludio's gate. The
// founder, attacking Gludio, hits the gate for 100: the vampire wants to
// attack the founder by 50 for each twentieth of its maximum hit points
// the damage makes. A Siege Golem's hit on the gate makes it want to
// attack both the golem's owner, weight 5000, and the golem, weight 1000.
func TestAgit02VampireArcherAgitGuardsItsDoor(t *testing.T) {
	t.Parallel()
	form, err := zone.NewCuboid(gateWorldAt.X-1000, gateWorldAt.X+1000, gateWorldAt.Y-1000, gateWorldAt.Y+1000,
		gateWorldAt.Z-1000, gateWorldAt.Z+1000)
	if err != nil {
		t.Fatal(err)
	}
	field := zone.NewSiege(90001, form)
	field.SetActive(true)
	zones := zone.NewIndex()
	zones.Add(field)
	const path = "script.ai.individual.Monster.WizardBase.Wizard.Agit02VampireArcherAgit"
	w := gateWorld(t,
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			siegeSummonTemplate(gateGolemNPCID, "Siege Golem"),
		})),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{vampireArcherID: script.KindHostile},
			[]script.Listing{{Path: path}}, script.Catalog{path: wizard.Agit02VampireArcherAgit}),
	)
	vampire := w.srv.SpawnHostileNPCTemplateAt(t, &npc.Template{
		ID: vampireArcherID, TemplateID: vampireArcherID, Type: "Monster", Name: "Vampire Caster", Level: 40, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Clans: []string{"door_clan"}, ClanRange: 300,
	}, location.Location{X: gateWorldAt.X + 100, Y: gateWorldAt.Y + 200, Z: gateWorldAt.Z})
	gate := gateObject(t, w, gludioGateID)
	obj, ok := w.srv.State.Object(w.leaderID)
	if !ok {
		t.Fatal("the founder is not in the world")
	}
	founder := obj.(attackable.Combatant)

	before := vampire.AggroHate(founder)
	w.srv.HitDoor(t, gludioGateID, w.leaderID, 100)
	want := 100.0 / float64(vampire.MaxHP()) / 0.05 * 50
	if got := vampire.AggroHate(founder) - before; got != want {
		t.Fatalf("vampire hate of the founder from its hit on the gate = %v, want %v", got, want)
	}

	golem := callSiegeSummon(t, w, w.leaderID, gateGolemNPCID)
	founderBefore, golemBefore := vampire.AggroHate(founder), vampire.AggroHate(golem)
	gate.ReduceHP(100, golem, modelskill.Definition{})
	if got := vampire.AggroHate(founder) - founderBefore; got != 5000 {
		t.Fatalf("vampire hate of the founder from the golem's hit = %v, want 5000", got)
	}
	if got := vampire.AggroHate(golem) - golemBefore; got != 1000 {
		t.Fatalf("vampire hate of the golem from its hit = %v, want 1000", got)
	}
}
