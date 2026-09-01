package gameservertest

import (
	"testing"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// hostileNPCSpawn is the fixed spawn point every fixture NPC uses: inside
// the class template's spawn neighborhood, so a just-entered player sees it
// without moving.
var hostileNPCSpawn = location.Location{X: 60, Y: 20, Z: 30}

// SpawnHostileNPC seeds a hostile monster at the fixture spawn point through
// the real NPC model and world state, exactly the way the spawner task does,
// and returns it so suites can assert its HP. The monster's AI controllers
// are parked stubs: behavior suites drive the monster from the client side
// (targeting, skill damage) and never need it to act on its own.
func (s *Server) SpawnHostileNPC(t *testing.T) *npc.Hostile {
	t.Helper()
	return s.SpawnHostileNPCAt(t, hostileNPCSpawn)
}

// SpawnHostileNPCAt seeds the fixture monster at an explicit point, so
// combat scenarios control attack range. It shares SpawnHostileNPC's
// construction path.
func (s *Server) SpawnHostileNPCAt(t *testing.T, at location.Location) *npc.Hostile {
	t.Helper()
	tmpl := &npc.Template{
		ID:              100,
		TemplateID:      100,
		Type:            "Monster",
		Level:           1,
		HPMax:           1000,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CollisionRadius: 8,
		CollisionHeight: 20,
	}
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	live, err := creature.NewLive(at, tmpl.RunSpeed, Geo{}, nil)
	if err != nil {
		t.Fatalf("new npc live: %v", err)
	}
	hostile, err := npc.NewHostile(inst, live, parkedMove{}, parkedAttack{})
	if err != nil {
		t.Fatalf("new hostile npc: %v", err)
	}
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	hostile.SetWorld(s.State)
	hostile.SetRewarder(gamemanager.NewHostileRewarder(hostile, tmpl, s.State,
		gamemanager.KillRewardConfig{PlayerLevels: s.levelTable}, s.itemTable))
	s.State.Spawn(hostile, at.X, at.Y, at.Z, 0)
	return hostile
}

type movingHostileStatRef struct{ effect.StatOwner }

type movingHostileActorRef struct{ attack.CreatureActor }

type movingHostileLocatedRef struct{ move.Actor }

// SpawnMovingHostileNPCAt seeds a hostile NPC of the given instance kind
// and template id with the production move controller wired through
// BroadcastMove, so leash-return and other server-initiated moves emit
// real observer packets.
func (s *Server) SpawnMovingHostileNPCAt(t *testing.T, kind string, npcID int, home, at location.Location) *npc.Hostile {
	t.Helper()
	tmpl := &npc.Template{
		ID:              npcID,
		TemplateID:      npcID,
		Type:            kind,
		Level:           1,
		HPMax:           1000,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CollisionRadius: 8,
		CollisionHeight: 20,
	}
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	inst.Kind = npc.InstanceKind(kind)
	inst.HasHome = true
	inst.Home = home
	statRef := &movingHostileStatRef{}
	live, err := creature.NewLive(at, tmpl.RunSpeed, Geo{}, statRef)
	if err != nil {
		t.Fatalf("new npc live: %v", err)
	}
	locRef := &movingHostileLocatedRef{}
	moveCtl, err := move.NewController(live.Move(), locRef)
	if err != nil {
		t.Fatalf("new move controller: %v", err)
	}
	moveCtl.SetPositionUpdates(task.NewPositionUpdates(s.State))
	actorRef := &movingHostileActorRef{}
	attackCtl := attack.NewAttackable(actorRef)
	hostile, err := npc.NewHostile(inst, live, moveCtl, attackCtl)
	if err != nil {
		t.Fatalf("new hostile npc: %v", err)
	}
	locRef.Actor = hostile
	actorRef.CreatureActor = hostile
	statRef.StatOwner = hostile
	hostile.SetFrameBuilder(serverpackets.NpcFrameBuilder{})
	hostile.SetWorld(s.State)
	hostile.SetRewarder(gamemanager.NewHostileRewarder(hostile, tmpl, s.State,
		gamemanager.KillRewardConfig{PlayerLevels: s.levelTable}, s.itemTable))
	s.State.Spawn(hostile, at.X, at.Y, at.Z, 0)
	return hostile
}

// TickEffects advances every spawned actor's live effect list once — the
// production one-second effect sweep — so buff expiry and damage-over-time
// ticks are deterministic instead of wall-clock driven.
func (s *Server) TickEffects() {
	task.NewEffects(s.State).Tick()
}

// parkedMove is a MoveController that never moves.
type parkedMove struct{}

func (parkedMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (parkedMove) MoveHome(location.Location) error { return nil }
func (parkedMove) Stop() error                      { return nil }

// parkedAttack is an AttackController that never attacks.
type parkedAttack struct{}

func (parkedAttack) BowCoolingDown() bool                { return false }
func (parkedAttack) AttackingNow() bool                  { return false }
func (parkedAttack) CanAttack(attackable.Combatant) bool { return false }
func (parkedAttack) DoAttack(attackable.Combatant) error { return nil }
