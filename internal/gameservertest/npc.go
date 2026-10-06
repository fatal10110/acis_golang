package gameservertest

import (
	"fmt"
	"testing"
	"time"

	gamemanager "github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
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

// SpawnHostileNPCTemplateAt seeds a parked hostile NPC built from tmpl, for
// suites that need template data read at spawn time, such as drop categories.
func (s *Server) SpawnHostileNPCTemplateAt(t *testing.T, tmpl *npc.Template, at location.Location) *npc.Hostile {
	t.Helper()
	return s.spawnHostile(t, tmpl, at, parkedAttack{})
}

// fixturePartyRange is the shipped players.properties PartyRange, shared by
// kill rewards and party loot.
const fixturePartyRange = 1500

// killRewards is the fixture kill-reward config: the suite's level table
// and exp/sp rates, stock x1 drop rates, the shipped MultipleItemDrop, the
// stock party range and party exp rules, the suite's drop gates, and the
// server's parties.
func (s *Server) killRewards() gamemanager.KillRewardConfig {
	return gamemanager.KillRewardConfig{
		PlayerLevels:      s.levelTable,
		Rates:             item.Rates{Spoil: 1, Currency: 1, Item: 1, ItemRaid: 1, Herb: 1},
		PartyRange:        fixturePartyRange,
		RateXP:            s.rateXP,
		RateSP:            s.rateSP,
		PartyXP:           player.PartyXPRules{Cutoff: player.PartyXPCutoffLevel, CutoffLevel: 20, CutoffPercent: 3, RateXP: 1, RateSP: 1},
		Parties:           s.rewardParties,
		RaidKills:         s.raidKills,
		Channels:          s.lootChannels,
		CursedWeapons:     s.cursedLink,
		DeepBlueDropRules: s.deepBlueDrops,
		AutoLoot:          s.autoLoot,
		AutoLootRaid:      s.autoLootRaid,
		MultipleItemDrop:  true,
	}
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
	return s.spawnHostileInstance(t, inst, at, attackCtl)
}

// spawnHostileInstance is spawnHostile for an instance the caller built.
func (s *Server) spawnHostileInstance(t *testing.T, inst *npc.Instance, at location.Location, attackCtl ai.AttackController) *npc.Hostile {
	t.Helper()
	tmpl := inst.Template
	live, err := creature.NewLive(at, tmpl.RunSpeed, Geo{}, nil, effect.WithEnv(s.effectEnv))
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
	hostile.SetMaxGeoPathFailCount(s.maxGeoPathFail)
	s.installZones(hostile)
	rewards, hits := gamemanager.NewHostileRewarder(hostile, tmpl, s.State,
		s.killRewards(), s.itemTable, s.ids, s.GroundItems)
	hostile.Attach(npc.Runtime{
		World:   s.State,
		Items:   s.itemTable,
		Rewards: rewards,
		Hits:    hits,
		Sink:    network.HostileSinks(s.State, s.stance)(hostile),
		Scripts: s.quests.registry,
	})
	s.State.Spawn(hostile, at.X, at.Y, at.Z, 0)
	hostile.EnterZones()
	return hostile
}

// SpawnCastingHostileNPC seeds the parked fixture monster from tmpl with the
// production AI-cast seam installed: the cast controller runs over the
// HostileActor adapter on the monster's own queue, and the AIController the
// AI loop drives resolves skills through defs and dispatches their effects
// through the link's HostileCastEffects, as boot wires every live monster. Suites start a cast with
// AIController.Cast on the monster's queue, exactly as the AI loop does.
func (s *Server) SpawnCastingHostileNPC(t *testing.T, tmpl *npc.Template, defs actorcast.Definitions) (*npc.Hostile, *actorcast.AIController) {
	t.Helper()
	hostile := s.spawnHostile(t, tmpl, hostileNPCSpawn, parkedAttack{})
	return hostile, s.installCastSeam(hostile, defs)
}

// installCastSeam wires hostile's AI-cast seam over defs (see
// SpawnCastingHostileNPC) and returns the AIController the AI loop drives.
func (s *Server) installCastSeam(hostile *npc.Hostile, defs actorcast.Definitions) *actorcast.AIController {
	ctl := actorcast.NewController(actorcast.HostileActor{Hostile: hostile}, castCanceledBroadcast{hostile})
	ctl.SetQueue(hostile.Queue())
	aiCtl := &actorcast.AIController{Controller: ctl, Definitions: defs, Effects: s.castEffects, Caster: hostile, OnHitResult: s.castEffects.OnHitResult}
	hostile.AI().SetCastController(aiCtl)
	hostile.SetCastController(ctl)
	return aiCtl
}

// castCanceledBroadcast closes an aborted fixture AI cast with its cancel
// animation, enters the stance an offensive cast earns and hands every cast
// end to the AI, as production's hostile controller sink does.
type castCanceledBroadcast struct{ hostile *npc.Hostile }

func (c castCanceledBroadcast) Emit(ev event.Event) {
	switch e := ev.(type) {
	case event.AttackStanceRequested:
		c.hostile.EnterAttackStance()
	case event.CastAborted:
		c.hostile.BroadcastSkillCanceled(c.hostile.ObjectID())
	case event.CastFinished:
		_ = c.hostile.CastFinished(e.Interrupted)
	}
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
	srv      *Server
}

// attackFinishedSignal is the attack controller sink of an AttackingHostile:
// it signals every finished swing on its channel, and enters the stance and
// runs the chance procs of every landed hit, as production's hostile
// controller sink does.
type attackFinishedSignal struct {
	finished chan struct{}
	chance   *actorcast.ChanceProcs
	hostile  *npc.Hostile
}

func (c *attackFinishedSignal) Emit(ev event.Event) {
	switch e := ev.(type) {
	case event.AttackFinished:
		select {
		case c.finished <- struct{}{}:
		default:
		}
	case event.AttackStanceRequested:
		c.hostile.EnterAttackStance()
	case event.HitLanded:
		c.chance.AttackHit(c.hostile, e)
	}
}

// DoAttack starts one swing against target and blocks until it lands (the
// hit is already delivered by the time the animation's finish callback
// fires), so the caller can assert HP/CP state right after without a
// wall-clock sleep. It lets time pass (Server.AdvanceUntil) until the swing
// finishes.
func (h *AttackingHostile) DoAttack(t *testing.T, target attackable.Combatant) {
	t.Helper()
	h.ctl.DoAttack(target)
	h.srv.AdvanceUntil(t, "npc attack finish", func() bool {
		select {
		case <-h.finished:
			return true
		default:
			return false
		}
	})
}

// StartAttack starts one swing against target without waiting for it to
// finish, for a swing whose hit stops the attack (for example by landing a
// target-removing effect on the NPC) so that it never finishes.
func (h *AttackingHostile) StartAttack(target attackable.Combatant) {
	h.ctl.DoAttack(target)
}

// CanAttack reports whether the NPC's attack controller would start a swing
// at target now, the check its AI runs before every swing.
func (h *AttackingHostile) CanAttack(target attackable.Combatant) bool {
	return h.ctl.CanAttack(target)
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
	signal := &attackFinishedSignal{finished: make(chan struct{}, 1), chance: s.castEffects.Chance}
	attackCtl := attack.NewAttackable(actorRef, signal)
	hostile := s.spawnHostile(t, tmpl, at, attackCtl)
	actorRef.CreatureActor = hostile
	signal.hostile = hostile
	// A deterministic zero roll always lands (Missed's rate is never
	// negative) so DoAttack's swing reliably deals damage instead of
	// occasionally missing. It also always crits whenever the template's
	// CritRate is above 0 (CritSucceeds(rate, 0) is rate > 0) — fine for
	// the default template's CritRate 0, but a tuned template with a
	// non-zero CritRate will see every landed hit crit, not the
	// configured percentage.
	hostile.SetRollSource(func(int) int { return 0 })
	return &AttackingHostile{Hostile: hostile, ctl: attackCtl, finished: signal.finished, srv: s}
}

type movingHostileStatRef struct{ effect.StatOwner }

// hostileActorRef indirects a hostile NPC's attack.CreatureActor surface
// for the attack.Controller constructed before the NPC itself exists (the
// controller needs an actor at construction; the NPC needs the controller
// at construction). Shared by the moving and stationary attacking fixtures.
type hostileActorRef struct{ attack.CreatureActor }

type movingHostileLocatedRef struct{ move.Actor }

func (r *movingHostileLocatedRef) GeoPathFailCount() int {
	if h, ok := r.Actor.(*npc.Hostile); ok {
		return h.GeoPathFailCount()
	}
	return 0
}

func (r *movingHostileLocatedRef) ResetGeoPathFailCount() {
	if h, ok := r.Actor.(*npc.Hostile); ok {
		h.ResetGeoPathFailCount()
	}
}

func (r *movingHostileLocatedRef) AddGeoPathFailCount() {
	if h, ok := r.Actor.(*npc.Hostile); ok {
		h.AddGeoPathFailCount()
	}
}

func (r *movingHostileLocatedRef) TeleportTo(target location.Location) {
	if h, ok := r.Actor.(*npc.Hostile); ok {
		h.TeleportTo(target)
	}
}

func (r *movingHostileLocatedRef) OffensiveFollowLead() bool {
	h, ok := r.Actor.(*npc.Hostile)
	return ok && h.OffensiveFollowLead()
}

func (r *movingHostileLocatedRef) IntentionMovesToTarget() bool {
	h, ok := r.Actor.(*npc.Hostile)
	return !ok || h.IntentionMovesToTarget()
}

func (r *movingHostileLocatedRef) CanSee(target attackable.Combatant) bool {
	h, ok := r.Actor.(*npc.Hostile)
	return !ok || h.CanSee(target)
}

func (r *movingHostileLocatedRef) Knows(target attackable.Combatant) bool {
	h, ok := r.Actor.(*npc.Hostile)
	return !ok || h.Knows(target)
}

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
	live, err := creature.NewLive(at, tmpl.RunSpeed, geo, statRef, effect.WithEnv(s.effectEnv))
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
	hostile.SetMaxGeoPathFailCount(s.maxGeoPathFail)
	s.installZones(hostile)
	locRef.Actor = hostile
	actorRef.CreatureActor = hostile
	statRef.StatOwner = hostile
	control.hostile, control.move = hostile, moveCtl
	rewards, hits := gamemanager.NewHostileRewarder(hostile, tmpl, s.State,
		s.killRewards(), s.itemTable, s.ids, s.GroundItems)
	rt := npc.Runtime{
		World:   s.State,
		Items:   s.itemTable,
		Rewards: rewards,
		Hits:    hits,
		Sink:    network.HostileSinks(s.State, s.stance)(hostile),
		Scripts: s.quests.registry,
	}
	// Production takes line of sight from the same geodata (npcs_spawn.go).
	if los, ok := geo.(npc.LineOfSight); ok {
		rt.LOS = los
	}
	hostile.Attach(rt)
	s.State.Spawn(hostile, at.X, at.Y, at.Z, 0)
	hostile.EnterZones()
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
	switch e := ev.(type) {
	case event.Arrived:
		c.hostile.SyncPosition(c.move.Position())
		c.hostile.SettleZones()
		c.hostile.AI().Arrived()
	case event.MoveBlocked:
		c.move.BroadcastBlockedCorrection()
		c.hostile.AI().ArrivedBlocked()
	case event.AttackFinished:
		if e.BowReuse {
			c.server.think(c.hostile)
			return
		}
		if err := c.hostile.AttackFinished(); err != nil {
			c.server.log.Warn().Err(err).Msg("ai: hostile attack finished")
		}
	case event.AttackRethink:
		c.server.runAI(c.hostile)
	case event.AttackStanceRequested:
		c.hostile.EnterAttackStance()
	case event.HitLanded:
		c.server.castEffects.Chance.AttackHit(c.hostile, e)
	}
}

func (s *Server) think(hostile *npc.Hostile) {
	if err := hostile.Think(); err != nil {
		s.log.Warn().Err(err).Msg("ai: hostile think")
	}
}

func (s *Server) runAI(hostile *npc.Hostile) {
	if err := hostile.RunAI(); err != nil {
		s.log.Warn().Err(err).Msg("ai: hostile run")
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

// TickPositions lets one position-update interval pass for the actor queues
// and runs one production movement-correction tick for every actor with
// movement in flight at its end — interpolation, arrival and the offensive
// follow re-check — then waits for the posted ticks to run. A player's
// update walks the queue-clock time since its last one, so the interval is
// what a tick stands for.
//
// On the driven clock the clock moves by exactly the interval, as Advance
// does, and the tick runs ahead of the timers due at the same instant, as
// the production ticker posts its ticks just ahead of a move's own timing:
// a player's update walks exactly the interval (less what a retarget since
// the last tick already walked), and a move started right before a run of
// ticks ends on the tick that reaches its end, not on its arrival timer.
//
// On the real pool a run of ticks keeps a fixed rate, as the production
// ticker does: each is posted one interval after the one before, or one
// interval after the call when the one before is longer ago than that (or
// there is none). A player's update then walks about the interval on the
// wall clock, and the updates since a move started at least as many
// intervals.
func (s *Server) TickPositions() {
	if s.queues.advanceThen != nil {
		if err := s.tickPositionsDriven(move.PositionUpdateInterval); err != nil {
			panic(err)
		}
		return
	}
	s.positionTicks.Lock()
	next := s.positionTicks.last.Add(move.PositionUpdateInterval)
	if now := time.Now(); next.Before(now) {
		next = now.Add(move.PositionUpdateInterval)
	}
	time.Sleep(time.Until(next))
	if err := s.awaitHandled(); err != nil {
		panic(err)
	}
	s.positionTicks.last = time.Now()
	s.positions.Tick()
	s.positionTicks.Unlock()
	if err := s.queues.settle(); err != nil {
		panic(err)
	}
}

// TickPositionsAfter is TickPositions on the driven clock with d, rather
// than one interval, passing before the tick, so a test can pin how a
// player's update walks an uneven gap such as the real pool's scheduling
// jitter. It fails the test on the real pool, whose ticks keep the
// production ticker's fixed rate.
func (s *Server) TickPositionsAfter(tb testing.TB, d time.Duration) {
	tb.Helper()
	if s.queues.advanceThen == nil {
		tb.Fatal("TickPositionsAfter needs the driven clock")
	}
	if err := s.tickPositionsDriven(d); err != nil {
		tb.Fatal(err)
	}
}

// tickPositionsDriven moves the driven clock by d, runs one production
// movement-correction tick ahead of the timers due at that instant, and
// waits for the posted ticks to run.
func (s *Server) tickPositionsDriven(d time.Duration) error {
	if err := s.catchUp(); err != nil {
		return err
	}
	s.queues.advanceThen(d, s.positions.Tick)
	return s.queues.settle()
}

// parkedMove is a MoveController that never moves.
type parkedMove struct{}

func (parkedMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (parkedMove) MoveToLocation(location.Location) (bool, error) { return false, nil }
func (parkedMove) CanMoveTo(location.Location) bool               { return true }
func (parkedMove) MoveHome(location.Location) error               { return nil }
func (parkedMove) Stop()                                          {}
func (parkedMove) CancelFollow()                                  {}

// parkedAttack is an AttackController that never attacks.
type parkedAttack struct{}

func (parkedAttack) BowCoolingDown() bool                { return false }
func (parkedAttack) AttackingNow() bool                  { return false }
func (parkedAttack) CanAttack(attackable.Combatant) bool { return false }
func (parkedAttack) DoAttack(attackable.Combatant)       {}
func (parkedAttack) Stop()                               {}

// installZones gives a fixture NPC the zone membership boot gives every
// live NPC, over the zones the suite supplied through WithZones.
func (s *Server) installZones(hostile *npc.Hostile) {
	hostile.SetZones(s.zones)
	if s.zones != nil && hostile.Live != nil {
		hostile.Move().UseCreatureZoneSwim()
	}
}

// FolkTemplate is a fixture civilian NPC template of the given instance
// kind and template id. Like the shipped service NPCs it is undying, so no
// damage takes it below 1 HP.
func FolkTemplate(kind string, npcID int) *npc.Template {
	return &npc.Template{
		ID:              npcID,
		TemplateID:      npcID,
		Type:            kind,
		Name:            "Folk",
		Level:           70,
		HPMax:           2444,
		AtkSpd:          253,
		RunSpeed:        120,
		WalkSpeed:       50,
		CON:             43,
		CollisionRadius: 8,
		CollisionHeight: 24,
		CorpseTime:      7,
		Undying:         true,
	}
}

// SpawnFolkNPCAt places a civilian NPC built from tmpl at at through the
// production civilian spawner, standing (no route walker).
func (s *Server) SpawnFolkNPCAt(t *testing.T, tmpl *npc.Template, at location.Location) *npc.Folk {
	t.Helper()
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	inst.Home, inst.HasHome = at, true
	f, err := s.folkSpawner(nil, Geo{}).Spawn(inst, at, 0, nil)
	if err != nil {
		t.Fatalf("spawn folk npc: %v", err)
	}
	return f
}

// SpawnCastingFolkNPCAt is SpawnFolkNPCAt with the production cast runtime
// installed over defs: the NPC casts what a dialog command or script asks
// of it on the AI task's next tick (WithAITask, Server.AI.Tick), its
// effects dispatched through the link's NPC cast seam.
func (s *Server) SpawnCastingFolkNPCAt(t *testing.T, tmpl *npc.Template, at location.Location, defs actorcast.Definitions) *npc.Folk {
	t.Helper()
	inst, err := npc.NewInstance(s.NewObjectID(), tmpl)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	inst.Home, inst.HasHome = at, true
	spawner := s.folkSpawner(nil, Geo{})
	spawner.Skills = defs
	f, err := spawner.Spawn(inst, at, 0, nil)
	if err != nil {
		t.Fatalf("spawn folk npc: %v", err)
	}
	return f
}

// folkSpawner is the production civilian spawner over this server's world,
// with walker as its route walker task.
func (s *Server) folkSpawner(walker *task.Walker, geo move.Geo) gamemanager.FolkSpawner {
	return gamemanager.FolkSpawner{
		State:               s.State,
		Walker:              walker,
		Geo:                 geo,
		Positions:           s.positions,
		Queues:              s.queues,
		NewSink:             s.folkSinks,
		Zones:               s.zones,
		CastEffects:         s.castEffects,
		AI:                  s.AI,
		Items:               s.itemTable,
		Decay:               s.decay,
		Effects:             s.effectEnv,
		MaxGeoPathFailCount: s.maxGeoPathFail,
		Log:                 s.log,
	}
}

// SpawnRouteFolkNPCAt places a civilian NPC built from tmpl at at through
// the production civilian spawner, with routes as the walker route data, and
// returns it with the route walker task it walks under. A template whose
// alias names a route in routes walks it from the moment it spawns,
// shown to its observers. walkMode puts it in walk stance, as the spawner
// does for the reference's walking ids.
func (s *Server) SpawnRouteFolkNPCAt(t *testing.T, tmpl *npc.Template, at location.Location, routes route.WalkerRoutes, walkMode bool) (*npc.Folk, *task.Walker) {
	t.Helper()
	return s.SpawnRouteFolkNPC(t, RouteFolkSpawn{Template: tmpl, At: at, Routes: routes, WalkMode: walkMode})
}

// RouteFolkSpawn describes a civilian NPC SpawnRouteFolkNPC places.
type RouteFolkSpawn struct {
	Template *npc.Template
	At       location.Location
	// Heading is the spawn heading, faced again on arriving back at At.
	Heading int
	// Routes is the walker route data.
	Routes route.WalkerRoutes
	// WalkMode puts the NPC in walk stance.
	WalkMode bool
	// Geo is the geodata the NPC walks and teleports on; nil for the
	// always-passable Geo.
	Geo move.Geo
	// Path answers the route walker's reachability checks; nil for every
	// node reachable.
	Path task.WalkerPath
	// Skills gives the NPC the production cast runtime over these
	// definitions; nil leaves it without one.
	Skills actorcast.Definitions
}

// SpawnRouteFolkNPC is SpawnRouteFolkNPCAt with the spawn heading, geodata
// and route reachability spec gives.
func (s *Server) SpawnRouteFolkNPC(t *testing.T, spec RouteFolkSpawn) (*npc.Folk, *task.Walker) {
	t.Helper()
	now := time.Now
	if s.queues.inline != nil {
		now = s.queues.inline.Now
	}
	geo := spec.Geo
	if geo == nil {
		geo = Geo{}
	}
	path := spec.Path
	if path == nil {
		path = task.GeoPath{Geo: Geo{}}
	}
	walker, err := task.NewWalker(spec.Routes, path, now, s.State)
	if err != nil {
		t.Fatalf("new walker: %v", err)
	}
	inst, err := npc.NewInstance(s.NewObjectID(), spec.Template)
	if err != nil {
		t.Fatalf("new npc instance: %v", err)
	}
	inst.Home, inst.HasHome, inst.WalkMode = spec.At, true, spec.WalkMode
	inst.SpawnHeading = spec.Heading
	spawner := s.folkSpawner(walker, geo)
	spawner.Skills = spec.Skills
	f, err := spawner.Spawn(inst, spec.At, spec.Heading, nil)
	if err != nil {
		t.Fatalf("spawn folk npc: %v", err)
	}
	return f, walker
}
