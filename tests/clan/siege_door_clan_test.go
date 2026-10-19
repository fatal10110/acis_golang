package clan

import (
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Door.reduceCurrentHp returns at once on a Swoop Cannon's hit
// during its castle's siege in progress: no HP is lost and no NPC is
// called. Any other hit, harmful or not, then calls the living NPCs of
// "door_clan" within 600 of the door, whose scripts' static-object
// clan-attacked hooks see the attacker, a siege summon or not.

// gateGuardID is the door clan guard standing by Gludio's gate.
const gateGuardID = 20911

// gateClanCall is one static-object clan-attacked hook run on the guard:
// the attacker's name, the damage and whether the attacker was a siege
// summon.
type gateClanCall struct {
	attacker    string
	damage      int32
	siegeSummon bool
}

type gateClanLog struct {
	mu    sync.Mutex
	calls []gateClanCall
}

func (l *gateClanLog) behavior() script.Script {
	return script.Script{Behavior: true, NPCs: []int32{gateGuardID}, Hooks: script.Hooks{
		OnStaticObjectClanAttacked: func(_ *script.Script, e script.StaticObjectClanAttacked) {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.calls = append(l.calls, gateClanCall{attacker: e.Attacker.Name(), damage: e.Damage, siegeSummon: e.Attacker.SiegeSummon()})
		},
	}}
}

func (l *gateClanLog) take() []gateClanCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.calls
	l.calls = nil
	return out
}

// gateClanWorld is the siege summon gate world with a door clan guard by
// the gate, its hook recorded in log.
func gateClanWorld(t *testing.T, log *gateClanLog) *clanWorld {
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
	w := gateWorld(t,
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			siegeSummonTemplate(gateGolemNPCID, "Siege Golem"),
			siegeSummonTemplate(gateCannonNPCID, "Swoop Cannon"),
		})),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{gateGuardID: script.KindHostile},
			[]script.Listing{{Path: "ai.GateGuardRecorder"}}, script.Catalog{"ai.GateGuardRecorder": log.behavior}),
	)
	guard := &npc.Template{
		ID: gateGuardID, TemplateID: gateGuardID, Type: "SiegeGuard", Name: "Gate Guard", Level: 40, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
		Clans: []string{"door_clan"}, ClanRange: 300,
	}
	w.srv.SpawnHostileNPCTemplateAt(t, guard, location.Location{X: gateWorldAt.X + 100, Y: gateWorldAt.Y + 200, Z: gateWorldAt.Z})
	return w
}

// TestSiegeGateDoorClanCall drives Gludio's real gate siege: before the
// siege a player's hit leaves the gate whole and still calls the guard;
// during it the Swoop Cannon's hit is dropped, calling no one, while the
// Siege Golem's lowers the gate and calls the guard with a siege summon as
// the attacker.
func TestSiegeGateDoorClanCall(t *testing.T) {
	t.Parallel()
	log := &gateClanLog{}
	w := gateClanWorld(t, log)
	s := gludioSiegeOf(t, w)
	gate := gateObject(t, w, gludioGateID)

	w.srv.HitDoor(t, gludioGateID, w.leaderID, 40)
	if got := gate.HP(); got != gateDoorHP {
		t.Fatalf("gate HP after a hit before the siege = %d, want %d", got, gateDoorHP)
	}
	if got, want := log.take(), []gateClanCall{{attacker: "Founder", damage: 40}}; !slices.Equal(got, want) {
		t.Fatalf("guard calls of a hit before the siege = %+v, want %+v", got, want)
	}

	s.Start()
	if !s.InProgress() {
		t.Fatal("Gludio's siege did not start")
	}
	cannon := callSiegeSummon(t, w, w.leaderID, gateCannonNPCID)
	gate.ReduceHP(100, cannon, modelskill.Definition{})
	if got := gate.HP(); got != gateDoorHP {
		t.Fatalf("gate HP after the Swoop Cannon's hit = %d, want %d", got, gateDoorHP)
	}
	if got := log.take(); len(got) != 0 {
		t.Fatalf("the Swoop Cannon's hit called the guard: %+v", got)
	}

	dismissSiegeSummon(t, w, w.leaderID, cannon)
	golem := callSiegeSummon(t, w, w.leaderID, gateGolemNPCID)
	gate.ReduceHP(100, golem, modelskill.Definition{})
	if got := gate.HP(); got != gateDoorHP-100 {
		t.Fatalf("gate HP after the golem's hit = %d, want %d", got, gateDoorHP-100)
	}
	if got, want := log.take(), []gateClanCall{{attacker: "Siege Golem", damage: 100, siegeSummon: true}}; !slices.Equal(got, want) {
		t.Fatalf("guard calls of the golem's hit = %+v, want %+v", got, want)
	}
}
