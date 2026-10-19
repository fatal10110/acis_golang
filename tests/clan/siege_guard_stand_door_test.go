package clan

import (
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
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

// gludioStandID is a standing guard of Gludio Castle (GludioStand), by
// Gludio's gate as one of "door_clan".
const gludioStandID = 35079

// gateStandWorld is the siege summon gate world with a Gludio standing
// guard by the gate, bound to GludioStand and the class it derives from,
// every script draw draw.
func gateStandWorld(t *testing.T, draw int) (*clanWorld, *npc.Hostile) {
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
		"script.ai.siegeguards.GludioStand.GludioStand",
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
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{gludioStandID: script.KindHostile}, list, script.Catalog{
			paths[0]: defaultnpc.New,
			paths[1]: siegeguard.Stand,
		}),
	)
	stand := &npc.Template{
		ID: gludioStandID, TemplateID: gludioStandID, Type: "SiegeGuard", Name: "Gludio Guard", Level: 40, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Clans: []string{"door_clan"}, ClanRange: 300,
	}
	h := w.srv.SpawnHostileNPCTemplateAt(t, stand, location.Location{X: gateWorldAt.X + 100, Y: gateWorldAt.Y + 200, Z: gateWorldAt.Z})
	return w, h
}

// TestGludioStandGuardsTheGateFromItsPost ports GludioStand's
// static-object clan-attacked hook on a Gludio standing guard by Gludio's
// gate. The founder, attacking Gludio, hits the gate for a twentieth of the
// guard's hit points: the guard wants to attack the founder by 50, from
// where it stands. A Siege Golem's hit on the gate draws for the guard:
// under 10 it wants to attack the golem's owner with weight 5000,
// otherwise the golem itself with weight 1000, from where it stands either
// way. (The golem it saw come it already attacks by 200.)
func TestGludioStandGuardsTheGateFromItsPost(t *testing.T) {
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
			w, stand := gateStandWorld(t, c.draw)
			gate := gateObject(t, w, gludioGateID)
			obj, ok := w.srv.State.Object(w.leaderID)
			if !ok {
				t.Fatal("the founder is not in the world")
			}
			founder := obj.(attackable.Combatant)
			maxHP := stand.MaxHP()
			if maxHP%20 != 0 {
				t.Fatalf("setup: standing guard max HP %d is no multiple of 20", maxHP)
			}

			w.srv.HitDoor(t, gludioGateID, w.leaderID, float64(maxHP/20))
			if got := stand.AggroHate(founder); got != 50 {
				t.Fatalf("standing guard hate of the founder after its hit on the gate = %v, want 50", got)
			}

			golem := callSiegeSummon(t, w, w.leaderID, gateGolemNPCID)
			// The guard sees the golem come: a summon of the castle's
			// attacker, which its see-creature hook attacks by 200.
			if got := stand.AggroHate(golem); got != 200 {
				t.Fatalf("standing guard hate of the golem it sees = %v, want 200", got)
			}
			gate.ReduceHP(100, golem, modelskill.Definition{})
			if got := stand.AggroHate(founder); got != 50+c.owner {
				t.Fatalf("standing guard hate of the founder after the golem's hit = %v, want %v", got, 50+c.owner)
			}
			if got := stand.AggroHate(golem); got != 200+c.siegeGolem {
				t.Fatalf("standing guard hate of the golem after its hit = %v, want %v", got, 200+c.siegeGolem)
			}
			for _, d := range stand.AI().Desires().Snapshot() {
				if d.Kind == actorai.IntentionAttack && d.MoveToTarget {
					t.Fatalf("standing guard attack desire %+v walks to its target, want it to stand", d)
				}
			}
		})
	}
}
