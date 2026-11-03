package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/siegeguard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// gludioKnightID is a knight of Gludio Castle (GludioKnight, a GludioHold
// guard), standing by Gludio's gate as one of "door_clan".
const gludioKnightID = 35066

// gateKnightWorld is the siege summon gate world with a Gludio knight by
// the gate, bound to GludioKnight and the classes it derives from, every
// script draw draw.
func gateKnightWorld(t *testing.T, draw int) (*clanWorld, *npc.Hostile) {
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
	paths := []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.siegeguards.GludioHold.GludioHold",
		"script.ai.siegeguards.GludioHold.GludioKnight",
	}
	list := make([]script.Listing, len(paths))
	for i, p := range paths {
		list[i] = script.Listing{Path: p}
	}
	w := gateWorld(t,
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			siegeSummonTemplate(gateGolemNPCID, "Siege Golem"),
		})),
		gameservertest.WithScriptRand(func(int) int { return draw }),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{gludioKnightID: script.KindHostile}, list, script.Catalog{
			paths[0]: defaultnpc.New,
			paths[1]: siegeguard.Hold,
			paths[2]: siegeguard.Knight,
		}),
	)
	knight := &npc.Template{
		ID: gludioKnightID, TemplateID: gludioKnightID, Type: "SiegeGuard", Name: "Gludio Knight", Level: 40, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Clans: []string{"door_clan"}, ClanRange: 300,
	}
	h := w.srv.SpawnHostileNPCTemplateAt(t, knight, location.Location{X: gateWorldAt.X + 100, Y: gateWorldAt.Y + 200, Z: gateWorldAt.Z})
	return w, h
}

// TestGludioHoldGuardsTheGate ports GludioHold's static-object
// clan-attacked hook on a Gludio knight by Gludio's gate. The founder,
// attacking Gludio, hits the gate for a twentieth of the knight's hit
// points: the knight wants to attack the founder by 50. A Siege Golem's
// hit on the gate draws for the knight: under 10 it wants to attack the
// golem's owner with weight 5000, otherwise the golem itself with weight
// 1000.
func TestGludioHoldGuardsTheGate(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name       string
		draw       int
		owner      float64
		siegeGolem float64
	}{
		{"draw 9", 9, 5000, 0},
		{"draw 10", 10, 0, 1000},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			w, knight := gateKnightWorld(t, c.draw)
			gate := gateObject(t, w, gludioGateID)
			obj, ok := w.srv.State.Object(w.leaderID)
			if !ok {
				t.Fatal("the founder is not in the world")
			}
			founder := obj.(attackable.Combatant)
			maxHP := knight.MaxHP()
			if maxHP%20 != 0 {
				t.Fatalf("setup: knight max HP %d is no multiple of 20", maxHP)
			}

			w.srv.HitDoor(t, gludioGateID, w.leaderID, float64(maxHP/20))
			if got := knight.AggroHate(founder); got != 50 {
				t.Fatalf("knight hate of the founder after its hit on the gate = %v, want 50", got)
			}

			golem := callSiegeSummon(t, w, w.leaderID, gateGolemNPCID)
			gate.ReduceHP(100, golem, modelskill.Definition{})
			if got := knight.AggroHate(founder); got != 50+c.owner {
				t.Fatalf("knight hate of the founder after the golem's hit = %v, want %v", got, 50+c.owner)
			}
			if got := knight.AggroHate(golem); got != c.siegeGolem {
				t.Fatalf("knight hate of the golem after its hit = %v, want %v", got, c.siegeGolem)
			}
		})
	}
}
