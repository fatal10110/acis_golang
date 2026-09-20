package gameservertest

import (
	"fmt"
	"testing"
	"time"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
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
	return s.SpawnHostileNPCKindAt(t, "Monster", at)
}

// SpawnHostileNPCKindAt seeds a parked hostile NPC of the given instance
// kind at an explicit point. Kind must be a Hostile-supported type
// ("Monster", "Guard", "SiegeGuard", ...).
func (s *Server) SpawnHostileNPCKindAt(t *testing.T, kind string, at location.Location) *npc.Hostile {
	t.Helper()
	tmpl := &npc.Template{
		ID:              100,
		TemplateID:      100,
		Type:            kind,
		Level:           1,
		HPMax:           1000,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CollisionRadius: 8,
		CollisionHeight: 20,
	}
	return s.spawnHostile(t, tmpl, at, parkedAttack{})
}

// spawnHostile builds and world-spawns a stationary hostile NPC from tmpl at
// at, wired with attackCtl. It is the parked-stub and real-attack fixtures'
// shared construction path (SpawnHostileNPCKindAt, SpawnAttackingHostileNPCTemplate).
func (s *Server) spawnHostile(t *testing.T, tmpl *npc.Template, at location.Location, attackCtl ai.AttackController) *npc.Hostile {
	t.Helper()
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	live, err := creature.NewLive(at, tmpl.RunSpeed, Geo{}, nil, effect.WithActivityRegistry(s.Effects))
	if err != nil {
		t.Fatalf("new npc live: %v", err)
	}
	live.SetQueue(s.queues.NewQueue(fmt.Sprintf("npc-%d", inst.ObjectID)))
	if ctl, ok := attackCtl.(*attack.Controller); ok {
		ctl.SetQueue(live.Queue())
	}
	hostile, err := npc.NewHostile(inst, live, parkedMove{}, attackCtl)
	if err != nil {
		t.Fatalf("new hostile npc: %v", err)
	}
	hostile.Attach(npc.Runtime{
		World: s.State,
		Items: s.itemTable,
		Rewards: gamemanager.NewHostileRewarder(hostile, tmpl, s.State,
			gamemanager.KillRewardConfig{PlayerLevels: s.levelTable}, s.itemTable),
		Sink: network.HostileSinks(s.State)(hostile),
	})
	s.State.Spawn(hostile, at.X, at.Y, at.Z, 0)
	return hostile
}

// AttackingHostile is a stationary hostile NPC wired with a real
// attack.Controller (see attack.NewAttackable), so a suite can trigger one
// deterministic melee swing at a live target and observe the resulting
// Attack frame and damage. Unlike SpawnMovingHostileNPCAtGeo it stays
// parked: only DoAttack drives it, never the AI loop.
type AttackingHostile struct {
	*npc.Hostile
	ctl      *attack.Controller
	finished chan struct{}
}

// attackFinishedSignal is the attack controller sink of an AttackingHostile:
// it signals every finished swing on its channel.
type attackFinishedSignal chan struct{}

func (c attackFinishedSignal) Emit(ev event.Event) {
	if _, ok := ev.(event.AttackFinished); ok {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

// DoAttack starts one swing against target and blocks until it lands (the
// hit is already delivered by the time the animation's finish callback
// fires), so the caller can assert HP/CP state right after without a
// wall-clock sleep. It fails the test if the swing does not finish within
// timeout.
func (h *AttackingHostile) DoAttack(t *testing.T, target attackable.Combatant, timeout time.Duration) {
	t.Helper()
	if err := h.ctl.DoAttack(target); err != nil {
		t.Fatalf("npc attack: %v", err)
	}
	select {
	case <-h.finished:
	case <-time.After(timeout):
		t.Fatal("npc attack did not finish within timeout")
	}
}

// AttackingHostileTemplate returns a fresh copy of the template
// SpawnAttackingHostileNPCAt spawns, for a suite that tunes a field (for
// example a heavier or lighter PAtk) and spawns it with
// SpawnAttackingHostileNPCTemplate.
func AttackingHostileTemplate() *npc.Template {
	return &npc.Template{
		ID:         100,
		TemplateID: 100,
		Type:       "Monster",
		Level:      1,
		HPMax:      1000,
		// PAtk deals roughly 20 damage against the
		// WithCharacter("Newbie", 5, 0) fixture player (35 max HP) under
		// the deterministic always-hit roll SpawnAttackingHostileNPCTemplate
		// installs: enough for a suite to tell a real, non-lethal landed
		// hit apart from a miss, a zero-damage roll, or a one-shot kill
		// that leaves no HP delta to assert. This template's own CritRate
		// is 0, so that roll never crits here — but a suite that tunes
		// CritRate above 0 should expect every landed hit to crit; see
		// SpawnAttackingHostileNPCTemplate's roll-source comment.
		PAtk:            1,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CollisionRadius: 8,
		CollisionHeight: 20,
	}
}

// SpawnAttackingHostileNPCAt is SpawnAttackingHostileNPCTemplate for
// AttackingHostileTemplate's default template.
func (s *Server) SpawnAttackingHostileNPCAt(t *testing.T, at location.Location) *AttackingHostile {
	t.Helper()
	return s.SpawnAttackingHostileNPCTemplate(t, AttackingHostileTemplate(), at)
}

// SpawnAttackingHostileNPCTemplate seeds a caller-tuned hostile NPC (see
// AttackingHostileTemplate) at an explicit point with a real attack
// controller instead of SpawnHostileNPCAt's parked stub, so a suite can
// drive AttackingHostile.DoAttack against a live target.
func (s *Server) SpawnAttackingHostileNPCTemplate(t *testing.T, tmpl *npc.Template, at location.Location) *AttackingHostile {
	t.Helper()
	actorRef := &hostileActorRef{}
	finished := make(attackFinishedSignal, 1)
	attackCtl := attack.NewAttackable(actorRef, finished)
	hostile := s.spawnHostile(t, tmpl, at, attackCtl)
	actorRef.CreatureActor = hostile
	// A deterministic zero roll always lands (Missed's rate is never
	// negative) so DoAttack's swing reliably deals damage instead of
	// occasionally missing. It also always crits whenever the template's
	// CritRate is above 0 (CritSucceeds(rate, 0) is rate > 0) — fine for
	// the default template's CritRate 0, but a tuned template with a
	// non-zero CritRate will see every landed hit crit, not the
	// configured percentage.
	hostile.SetRollSource(func(int) int { return 0 })
	return &AttackingHostile{Hostile: hostile, ctl: attackCtl, finished: finished}
}

type movingHostileStatRef struct{ effect.StatOwner }

// hostileActorRef indirects a hostile NPC's attack.CreatureActor surface
// for the attack.Controller constructed before the NPC itself exists (the
// controller needs an actor at construction; the NPC needs the controller
// at construction). Shared by the moving and stationary attacking fixtures.
type hostileActorRef struct{ attack.CreatureActor }

type movingHostileLocatedRef struct{ move.Actor }

// SpawnMovingHostileNPCAt seeds a hostile monster with the production move
// controller wired through BroadcastMove, so leash-return and other
// server-initiated moves emit real observer packets.
func (s *Server) SpawnMovingHostileNPCAt(t *testing.T, kind string, home, at location.Location) *npc.Hostile {
	t.Helper()
	return s.SpawnMovingHostileNPCAtGeo(t, kind, home, at, Geo{})
}

// SpawnMovingHostileNPCAtGeo is SpawnMovingHostileNPCAt with an explicit
// movement geo, so a suite can close the path after the walk starts.
func (s *Server) SpawnMovingHostileNPCAtGeo(t *testing.T, kind string, home, at location.Location, geo move.Geo) *npc.Hostile {
	t.Helper()
	return s.spawnMovingHostile(t, MovingHostileTemplate(kind), home, at, geo)
}

// MovingHostileTemplate returns a fresh copy of the template
// SpawnMovingHostileNPCAt spawns for kind, for a suite that tunes a field
// and spawns it with SpawnMovingHostileNPCTemplate.
func MovingHostileTemplate(kind string) *npc.Template {
	return &npc.Template{
		ID:              100,
		TemplateID:      100,
		Type:            kind,
		Level:           1,
		HPMax:           1000,
		AtkSpd:          300,
		RunSpeed:        120,
		WalkSpeed:       60,
		CanMove:         true,
		CollisionRadius: 8,
		CollisionHeight: 20,
	}
}

// SpawnMovingHostileNPCTemplate is SpawnMovingHostileNPCAt for a
// caller-tuned template (see MovingHostileTemplate).
func (s *Server) SpawnMovingHostileNPCTemplate(t *testing.T, tmpl *npc.Template, home, at location.Location) *npc.Hostile {
	t.Helper()
	return s.spawnMovingHostile(t, tmpl, home, at, Geo{})
}

func (s *Server) spawnMovingHostile(t *testing.T, tmpl *npc.Template, home, at location.Location, geo move.Geo) *npc.Hostile {
	t.Helper()
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	inst.Kind = npc.InstanceKind(tmpl.Type)
	inst.HasHome = true
	inst.Home = home
	statRef := &movingHostileStatRef{}
	live, err := creature.NewLive(at, tmpl.RunSpeed, geo, statRef, effect.WithActivityRegistry(s.Effects))
	if err != nil {
		t.Fatalf("new npc live: %v", err)
	}
	live.SetQueue(s.queues.NewQueue(fmt.Sprintf("npc-%d", inst.ObjectID)))
	locRef := &movingHostileLocatedRef{}
	control := &movingHostileControl{server: s}
	moveCtl, err := move.NewController(live.Move(), locRef, control)
	if err != nil {
		t.Fatalf("new move controller: %v", err)
	}
	moveCtl.SetPositionUpdates(s.positions)
	actorRef := &hostileActorRef{}
	attackCtl := attack.NewAttackable(actorRef, control)
	attackCtl.SetQueue(live.Queue())
	hostile, err := npc.NewHostile(inst, live, moveCtl, attackCtl)
	if err != nil {
		t.Fatalf("new hostile npc: %v", err)
	}
	locRef.Actor = hostile
	actorRef.CreatureActor = hostile
	statRef.StatOwner = hostile
	control.hostile, control.move = hostile, moveCtl
	hostile.Attach(npc.Runtime{
		World: s.State,
		Rewards: gamemanager.NewHostileRewarder(hostile, tmpl, s.State,
			gamemanager.KillRewardConfig{PlayerLevels: s.levelTable}, s.itemTable),
		Sink: network.HostileSinks(s.State)(hostile),
	})
	s.State.Spawn(hostile, at.X, at.Y, at.Z, 0)
	return hostile
}

// movingHostileControl reacts to a moving fixture NPC's controller events
// the way production does (data/manager newLiveHostile): without it a
// hostile re-thinks only once per AI tick, and its final arrival position
// never reaches world presence.
type movingHostileControl struct {
	server  *Server
	hostile *npc.Hostile
	move    *move.Controller
}

func (c *movingHostileControl) Emit(ev event.Event) {
	switch ev.(type) {
	case event.Arrived:
		c.hostile.SyncPosition(c.move.Position())
		c.hostile.AI().Arrived()
		c.server.think(c.hostile)
	case event.MoveBlocked:
		c.move.BroadcastBlockedCorrection()
		c.hostile.AI().ArrivedBlocked()
		c.server.think(c.hostile)
	case event.AttackFinished:
		c.server.think(c.hostile)
	}
}

func (s *Server) think(hostile *npc.Hostile) {
	if err := hostile.Think(); err != nil {
		s.log.Warn().Err(err).Msg("ai: hostile think")
	}
}

// TickEffects advances every spawned actor's live effect list once — the
// production one-second effect sweep — and waits for the posted ticks to
// run, so buff expiry and damage-over-time ticks are deterministic instead of
// wall-clock driven.
func (s *Server) TickEffects() {
	s.Effects.Tick()
	if err := s.queues.settle(); err != nil {
		panic(err)
	}
}

// parkedMove is a MoveController that never moves.
type parkedMove struct{}

func (parkedMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (parkedMove) MoveToLocation(location.Location) (bool, error) { return false, nil }
func (parkedMove) CanMoveTo(location.Location) bool               { return true }
func (parkedMove) MoveHome(location.Location) error               { return nil }
func (parkedMove) Stop() error                                    { return nil }

// parkedAttack is an AttackController that never attacks.
type parkedAttack struct{}

func (parkedAttack) BowCoolingDown() bool                { return false }
func (parkedAttack) AttackingNow() bool                  { return false }
func (parkedAttack) CanAttack(attackable.Combatant) bool { return false }
func (parkedAttack) DoAttack(attackable.Combatant) error { return nil }
func (parkedAttack) Stop()                               {}
