package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Door.isAttackableBy lets a castle wall be attacked during
// its siege only by a SiegeSummon of the attacker side other than the
// Swoop Cannon (14839), the side read from the summon's acting player;
// Door.reduceCurrentHp drops a Swoop Cannon's hit on any castle door.

// The siege summons: the Siege Golem breaches walls, the anti-infantry
// Swoop Cannon does not.
const (
	gateGolemNPCID  = 14737
	gateCannonNPCID = 14839
)

func siegeSummonTemplate(id int, name string) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "SiegeSummon", Name: name, Level: 60,
		HPMax: 5000, MPMax: 100, AtkSpd: 300, RunSpeed: 60, WalkSpeed: 30,
		CollisionRadius: 60, CollisionHeight: 60,
	}
}

// gateSummonWorld is the gate world with the siege summons known and a
// running siege battlefield under the players, so a siege summon called
// there stays. The battlefield belongs to no castle: Gludio's siege start
// throws no one off it.
func gateSummonWorld(t *testing.T) *clanWorld {
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
	return gateWorld(t,
		gameservertest.WithZones(zones),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			siegeSummonTemplate(gateGolemNPCID, "Siege Golem"),
			siegeSummonTemplate(gateCannonNPCID, "Swoop Cannon"),
		})),
	)
}

// callSiegeSummon has the player ownerID call the siege summon npcID
// through the servitor summon path and returns it once it is in the world.
func callSiegeSummon(t *testing.T, w *clanWorld, ownerID int32, npcID int) *summon.Actor {
	t.Helper()
	obj, ok := w.srv.State.Player(ownerID)
	if !ok {
		t.Fatalf("player %d is not in the world", ownerID)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %d = %T, want an online character", ownerID, obj)
	}
	w.srv.PlayerQueue(t, ownerID).Post(func() {
		c.SummonServitor(modelskill.Definition{ID: 13, Level: 1, SkillType: "SUMMON", NpcID: npcID, SummonTotalLifeTime: 1_200_000})
	})
	var s *summon.Actor
	w.srv.AdvanceUntil(t, "the siege summon is in the world", func() bool {
		obj, ok := w.srv.State.Summon(ownerID)
		if ok {
			s, ok = obj.(*summon.Actor)
		}
		return ok && s.NPCID() == npcID && !s.OffBattlefield()
	})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	return s
}

// dismissSiegeSummon unsummons ownerID's summon s and waits for its slot
// to empty.
func dismissSiegeSummon(t *testing.T, w *clanWorld, ownerID int32, s *summon.Actor) {
	t.Helper()
	s.Queue().Post(s.Unsummon)
	w.srv.AdvanceUntil(t, "the siege summon is gone", func() bool {
		_, ok := w.srv.State.Summon(ownerID)
		return !ok
	})
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
}

// TestSiegeGatesSiegeSummons drives Gludio's real gate siege with siege
// summons: the attacking founder's Siege Golem may attack the gate and the
// wall, its hit lowering the wall; the defending rival's golem may attack
// neither; the founder's Swoop Cannon may attack the gate but not the
// wall, and its hit leaves the gate whole, which the founder's own hit
// then lowers.
func TestSiegeGatesSiegeSummons(t *testing.T) {
	t.Parallel()
	w := gateSummonWorld(t)
	s := gludioSiegeOf(t, w)
	gate, wall := gateObject(t, w, gludioGateID), gateObject(t, w, gludioWallID)
	s.Start()
	if !s.InProgress() {
		t.Fatal("Gludio's siege did not start")
	}

	golem := callSiegeSummon(t, w, w.leaderID, gateGolemNPCID)
	if !golem.SiegeSummon() {
		t.Fatal("the Siege Golem is not a siege summon")
	}
	if !gate.AttackableBy(golem) || !wall.AttackableBy(golem) {
		t.Fatalf("attacker's golem may attack gate %t, wall %t; want both", gate.AttackableBy(golem), wall.AttackableBy(golem))
	}
	wall.ReduceHP(100, golem, modelskill.Definition{})
	if got := wall.HP(); got != gateDoorHP-100 {
		t.Fatalf("wall HP after the golem's hit = %d, want %d", got, gateDoorHP-100)
	}

	rivalGolem := callSiegeSummon(t, w, w.memberID, gateGolemNPCID)
	if gate.AttackableBy(rivalGolem) || wall.AttackableBy(rivalGolem) {
		t.Fatalf("defender's golem may attack gate %t, wall %t; want neither",
			gate.AttackableBy(rivalGolem), wall.AttackableBy(rivalGolem))
	}

	dismissSiegeSummon(t, w, w.leaderID, golem)
	cannon := callSiegeSummon(t, w, w.leaderID, gateCannonNPCID)
	if !gate.AttackableBy(cannon) {
		t.Fatal("attacker's Swoop Cannon may not attack the gate")
	}
	if wall.AttackableBy(cannon) {
		t.Fatal("attacker's Swoop Cannon may attack the wall")
	}
	gate.ReduceHP(100, cannon, modelskill.Definition{})
	if got := gate.HP(); got != gateDoorHP {
		t.Fatalf("gate HP after the Swoop Cannon's hit = %d, want %d", got, gateDoorHP)
	}
	w.srv.HitDoor(t, gludioGateID, w.leaderID, 100)
	if got := gate.HP(); got != gateDoorHP-100 {
		t.Fatalf("gate HP after the founder's own hit = %d, want %d", got, gateDoorHP-100)
	}
}
