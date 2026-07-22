package npc

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameserver/world/worldtest"
)

func TestHostileUsesWorldVisibilityAndTemplateAttackRange(t *testing.T) {
	state := world.New()
	target := &hostileTarget{id: 200}
	state.Spawn(target, 100, 100, 0, 0)
	move := &hostileMove{}
	strike := &hostileAttack{canAttack: true}

	hostile := newTestHostile(t, move, strike)
	state.Spawn(hostile, 120, 100, 0, 0)

	hostile.AddDamageHate(target, 0, 100)
	hostile.Think()

	if strike.target != target {
		t.Fatalf("attack target = %v, want known target", strike.target)
	}
	if move.followTarget != target || move.followRange != 80 {
		t.Fatalf("follow = (%v, %d), want (%v, 80)", move.followTarget, move.followRange, target)
	}
}

func TestHostileAddDefaultHateUsesOpeningValue(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	target := &hostileTarget{id: 200}

	hostile.AddDefaultHate(target)

	if got := hostile.AI().Hates().Hate(target); got != 300 {
		t.Fatalf("default hate = %v, want 300", got)
	}
}

func TestHostileIgnoresUnknownTarget(t *testing.T) {
	state := world.New()
	target := &hostileTarget{id: 200}
	state.Spawn(target, world.MaxX, world.MaxY, 0, 0)
	move := &hostileMove{}
	strike := &hostileAttack{canAttack: true}

	hostile := newTestHostile(t, move, strike)
	state.Spawn(hostile, world.MinX, world.MinY, 0, 0)

	hostile.AddDamageHate(target, 0, 100)
	hostile.Think()

	if strike.target != nil {
		t.Fatalf("attack target = %v, want none for unknown target", strike.target)
	}
	if move.followTarget != nil {
		t.Fatalf("follow target = %v, want none for unknown target", move.followTarget)
	}
}

func TestHostileRunsFromAITask(t *testing.T) {
	state := world.New()
	target := &hostileTarget{id: 200}
	state.Spawn(target, 100, 100, 0, 0)
	strike := &hostileAttack{canAttack: true}
	hostile := newTestHostile(t, &hostileMove{}, strike)
	state.Spawn(hostile, 120, 100, 0, 0)
	hostile.AddDamageHate(target, 0, 100)

	brains := task.NewAI(nil)
	brains.Add(hostile)
	brains.Tick()

	if strike.target != target {
		t.Fatalf("attack target = %v, want target after AI task tick", strike.target)
	}
}

func TestHostileReturnHomeMovesTowardHomeAndClearsThreat(t *testing.T) {
	state := world.New()
	move := &hostileMove{}
	hostile := newTestHostile(t, move, &hostileAttack{})
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 100, Y: 100, Z: 0}
	state.Spawn(hostile, 500, 100, 0, 0)
	target := &hostileTarget{id: 200}
	hostile.AddDamageHate(target, 0, 100)

	hostile.AI().SetWander()
	hostile.Think()

	if got := hostile.AI().Threats().Hate(target); got != 0 {
		t.Fatalf("threat hate after return home = %v, want 0", got)
	}
	if move.home != hostile.Instance.Home {
		t.Fatalf("home move = %+v, want %+v", move.home, hostile.Instance.Home)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("current intention = %v, want wander while returning home", got)
	}
}

func TestHostileInactiveRegionSleepHonorsTemplateAndTerritory(t *testing.T) {
	state := world.New()

	sleeping := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	state.Spawn(sleeping, 0, 0, 0, 0)
	if !sleeping.SleepWhenRegionInactive() {
		t.Fatal("regular in-territory hostile sleep = false, want true")
	}

	noSleep := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	noSleep.Instance.Template.NoSleepMode = true
	state.Spawn(noSleep, 0, 0, 0, 0)
	if noSleep.SleepWhenRegionInactive() {
		t.Fatal("no-sleep hostile sleep = true, want false")
	}

	outside := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	outside.Instance.HasHome = true
	outside.Instance.Home = location.Location{X: 0, Y: 0, Z: 0}
	state.Spawn(outside, 500, 0, 0, 0)
	if outside.SleepWhenRegionInactive() {
		t.Fatal("out-of-territory hostile sleep = true, want false")
	}
}

func TestHostileOnInactiveRegionResetsCombatAndReturnsHome(t *testing.T) {
	state := world.New()
	move := &hostileMove{}
	strike := &hostileAttack{canAttack: true}
	hostile := newTestHostile(t, move, strike)
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 0, Y: 0, Z: 0}
	state.Spawn(hostile, 500, 0, 0, 0)
	target := &hostileTarget{id: 200}
	state.Spawn(target, 520, 0, 0, 0)

	hostile.AddDamageHate(target, 5, 20)
	hostile.AddHate(target, 30)
	hostile.Think()

	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("current intention before reset = %v, want %v", got, ai.IntentionAttack)
	}

	hostile.OnInactiveRegion()

	if !hostile.AI().Threats().IsEmpty() {
		t.Fatal("threat table not cleared")
	}
	if !hostile.AI().Hates().IsEmpty() {
		t.Fatal("hate table not cleared")
	}
	if got := hostile.AI().Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0", got)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
		t.Fatalf("current intention after reset = %v, want %v", got, ai.IntentionWander)
	}
	if move.stopCount == 0 {
		t.Fatal("movement was not stopped on inactive reset")
	}

	hostile.Think()

	if move.home != hostile.Instance.Home {
		t.Fatalf("home move = %+v, want %+v", move.home, hostile.Instance.Home)
	}
}

func TestHostileThinkSleepsInInactiveRegion(t *testing.T) {
	state := world.New()
	move := &hostileMove{}
	strike := &hostileAttack{canAttack: true}
	hostile := newTestHostile(t, move, strike)
	hostile.SetWorld(state)
	state.Spawn(hostile, 0, 0, 0, 0)
	target := &hostileTarget{id: 200}
	state.Spawn(target, 10, 0, 0, 0)

	hostile.AddDamageHate(target, 5, 20)

	hostile.Think()
	hostile.Think()

	if strike.target != nil {
		t.Fatalf("attack target = %v, want none while region inactive", strike.target)
	}
	if !hostile.AI().Threats().IsEmpty() {
		t.Fatal("threat table not cleared by inactive Think")
	}
	if got := hostile.AI().Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0", got)
	}
	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want one inactive reset", move.stopCount)
	}
}

func TestHostileRegionDeactivationResetsCombatImmediately(t *testing.T) {
	state := world.New()
	move := &hostileMove{}
	hostile := newTestHostile(t, move, &hostileAttack{})
	hostile.SetWorld(state)
	player := worldtest.SpawnPlayer(state, 1, 0, 0, 0)
	state.Spawn(hostile, 10, 0, 0, 0)
	target := &hostileTarget{id: 200}

	hostile.AddDamageHate(target, 5, 20)
	hostile.AddHate(target, 30)

	state.Despawn(player)

	if !hostile.AI().Threats().IsEmpty() {
		t.Fatal("threat table not cleared on region deactivation")
	}
	if !hostile.AI().Hates().IsEmpty() {
		t.Fatal("hate table not cleared on region deactivation")
	}
	if got := hostile.AI().Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0", got)
	}
	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want one deactivation reset", move.stopCount)
	}
}

func TestHostileOutOfTerritoryReturnsHomeAfterRegionDeactivation(t *testing.T) {
	state := world.New()
	move := &hostileMove{}
	hostile := newTestHostile(t, move, &hostileAttack{})
	hostile.SetWorld(state)
	hostile.Instance.HasHome = true
	hostile.Instance.Home = location.Location{X: 0, Y: 0, Z: 0}
	player := worldtest.SpawnPlayer(state, 1, 500, 0, 0)
	state.Spawn(hostile, 500, 0, 0, 0)
	target := &hostileTarget{id: 200}

	hostile.AddHate(target, 30)

	state.Despawn(player)
	hostile.Think()

	if move.home != hostile.Instance.Home {
		t.Fatalf("home move = %+v, want %+v", move.home, hostile.Instance.Home)
	}
}

// TestHostileMovingIntoAlreadyInactiveRegionResetsWithoutThink covers issue
// #816: a hostile's own region never toggles when it (a non-player) is the
// one crossing into a neighbor that was already inactive, so nothing would
// reset it without Region.Add's current-state notify — and unlike the
// notify-on-toggle path, this doesn't depend on Think ever running again.
func TestHostileMovingIntoAlreadyInactiveRegionResetsWithoutThink(t *testing.T) {
	state := world.New()
	move := &hostileMove{}
	hostile := newTestHostile(t, move, &hostileAttack{})
	hostile.SetWorld(state)
	worldtest.SpawnPlayer(state, 1, 0, 0, 0)
	state.Spawn(hostile, 0, 0, 0, 0)
	target := &hostileTarget{id: 200}

	hostile.AddHate(target, 30)

	const far = 8192 // outside the player's 3x3 neighborhood; destination stays inactive
	if err := state.Move(hostile, far, 0, 0); err != nil {
		t.Fatalf("Move: %v", err)
	}

	if !hostile.AI().Hates().IsEmpty() {
		t.Fatal("hate table not cleared after moving into an already-inactive region")
	}
	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want one reset from the move-in notify", move.stopCount)
	}
}

type hostileRewarder struct {
	calls []creature.DeathActor
}

func (r *hostileRewarder) CalculateRewards(killer creature.DeathActor) {
	r.calls = append(r.calls, killer)
}

func TestHostileDieAppliesOnceAndRunsRewardHook(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	killer := &hostileTarget{id: 200}
	rewards := &hostileRewarder{}

	if hostile.AlikeDead() {
		t.Fatal("AlikeDead() = true before death, want false")
	}

	if !hostile.Die(killer, rewards) {
		t.Fatal("Die() = false, want true on first kill")
	}
	if !hostile.AlikeDead() {
		t.Fatal("AlikeDead() = false after death, want true")
	}
	if len(rewards.calls) != 1 || rewards.calls[0] != killer {
		t.Fatalf("rewards.calls = %v, want one call with killer", rewards.calls)
	}

	if hostile.Die(killer, rewards) {
		t.Fatal("Die() = true on repeat kill, want false")
	}
	if len(rewards.calls) != 1 {
		t.Fatalf("rewards.calls after repeat kill = %v, want unchanged", rewards.calls)
	}
}

func TestHostileDieConcurrentOnlyOneWinner(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})

	const attempts = 50
	results := make(chan bool, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- hostile.Die(nil, nil)
		}()
	}
	wg.Wait()
	close(results)

	wins := 0
	for r := range results {
		if r {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("wins = %d, want exactly 1", wins)
	}
}

func TestHostileDecayRemovesFromWorldAndRunsRespawnHook(t *testing.T) {
	state := world.New()
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	state.Spawn(hostile, 100, 100, 0, 0)
	hostile.Die(nil, nil)

	respawned := false
	if !hostile.Decay(state, func() { respawned = true }) {
		t.Fatal("Decay() = false, want true on first decay")
	}
	if !hostile.Decayed() {
		t.Fatal("Decayed() = false after Decay, want true")
	}
	if !respawned {
		t.Fatal("respawn hook was not called")
	}
	if _, ok := state.Object(hostile.ObjectID()); ok {
		t.Fatal("hostile is still tracked in the world after Decay")
	}

	respawned = false
	if hostile.Decay(state, func() { respawned = true }) {
		t.Fatal("Decay() = true on repeat call, want false")
	}
	if respawned {
		t.Fatal("respawn hook ran again on repeat Decay call")
	}
}

func TestHostileDecayToleratesNilWorldAndRespawnHook(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	hostile.Die(nil, nil)

	if !hostile.Decay(nil, nil) {
		t.Fatal("Decay() = false with nil world/respawn, want true")
	}
}

func TestNewHostileRejectsInvalidDependencies(t *testing.T) {
	inst := &Instance{ObjectID: 101, Template: &Template{ID: 9001, Type: "Monster"}}
	move := &hostileMove{}
	strike := &hostileAttack{}

	tests := []struct {
		name   string
		inst   *Instance
		live   *creature.Live
		move   ai.MoveController
		strike ai.AttackController
	}{
		{name: "nil instance", live: newHostileLive(t), move: move, strike: strike},
		{name: "nil template", inst: &Instance{ObjectID: 101}, live: newHostileLive(t), move: move, strike: strike},
		{name: "nil live creature", inst: inst, move: move, strike: strike},
		{name: "nil move", inst: inst, live: newHostileLive(t), strike: strike},
		{name: "nil attack", inst: inst, live: newHostileLive(t), move: move},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewHostile(tc.inst, tc.live, tc.move, tc.strike); err == nil {
				t.Fatal("NewHostile() error = nil")
			}
		})
	}
}

func TestNewHostileRejectsNonAttackableKind(t *testing.T) {
	inst := &Instance{
		ObjectID: 101,
		Template: &Template{ID: 9001, Type: "Folk"},
		Kind:     "Folk",
	}

	if _, err := NewHostile(inst, newHostileLive(t), &hostileMove{}, &hostileAttack{}); err == nil {
		t.Fatal("NewHostile() error = nil")
	}
}

func newTestHostile(t *testing.T, move ai.MoveController, strike ai.AttackController) *Hostile {
	t.Helper()
	hostile, err := NewHostile(&Instance{
		ObjectID: 101,
		Template: &Template{
			ID:              9001,
			Type:            "Monster",
			BaseAttackRange: 80,
		},
		Kind: "Monster",
	}, newHostileLive(t), move, strike)
	if err != nil {
		t.Fatal(err)
	}
	return hostile
}

func newHostileLive(t testing.TB) *creature.Live {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, hostileGeo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return live
}

type hostileGeo struct{}

func (hostileGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (hostileGeo) Height(_, _, _ int) int16          { return 0 }

// hostileGeo never blocks in these tests, so pathfinding and fall-back
// queries never need a useful answer: return no path and reflect the origin.
func (hostileGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (hostileGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

type hostileTarget struct {
	world.Presence
	id int32
}

func (t *hostileTarget) ObjectID() int32  { return t.id }
func (t *hostileTarget) SiegeGuard() bool { return false }
func (t *hostileTarget) AlikeDead() bool  { return false }

type hostileMove struct {
	followTarget attackable.Combatant
	followRange  int
	home         location.Location
	stopCount    int
}

func (m *hostileMove) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) bool {
	m.followTarget = target
	m.followRange = attackRange
	return false
}

func (m *hostileMove) MoveHome(home location.Location) {
	m.home = home
}

func (m *hostileMove) Stop() { m.stopCount++ }

type hostileAttack struct {
	canAttack bool
	target    attackable.Combatant
}

func (a *hostileAttack) BowCoolingDown() bool { return false }
func (a *hostileAttack) AttackingNow() bool   { return false }
func (a *hostileAttack) CanAttack(attackable.Combatant) bool {
	return a.canAttack
}
func (a *hostileAttack) DoAttack(target attackable.Combatant) {
	a.target = target
}

// hostileEffectTarget satisfies the flee hook a Fear effect's runtime needs,
// so it activates regardless of what its actual effected actor is.
type hostileEffectTarget struct{}

func (hostileEffectTarget) FleeFrom(effector any, distance int) {}

func addHostileEffect(t *testing.T, hostile *Hostile, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = hostileEffectTarget{}
	hostile.EffectList().Add(e)
	return e
}

func TestHostileDenyAIActionReflectsCrowdControlAndDeath(t *testing.T) {
	tests := []struct {
		name       string
		effectName string
	}{
		{"stunned", "Stun"},
		{"sleeping", "Sleep"},
		{"paralyzed", "Paralyze"},
		{"afraid", "Fear"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
			if hostile.DenyAIAction() {
				t.Fatal("DenyAIAction() = true before any effect is active")
			}

			e := addHostileEffect(t, hostile, tt.effectName)
			if !hostile.DenyAIAction() {
				t.Fatalf("DenyAIAction() = false while %s, want true", tt.name)
			}

			hostile.EffectList().Remove(e)
			if hostile.DenyAIAction() {
				t.Fatalf("DenyAIAction() = true after the %s effect was removed", tt.name)
			}
		})
	}

	t.Run("dead", func(t *testing.T) {
		hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
		if !hostile.MarkDead() {
			t.Fatal("MarkDead() reported no change on a live NPC")
		}
		if !hostile.DenyAIAction() {
			t.Fatal("DenyAIAction() = false for a dead NPC")
		}
	})
}

func newTestHostileOfKind(t *testing.T, state *world.State, kind InstanceKind, objectID int32, x, y int) *Hostile {
	t.Helper()
	hostile, err := NewHostile(&Instance{
		ObjectID: objectID,
		Template: &Template{
			ID:              9000 + int(objectID),
			Type:            string(kind),
			BaseAttackRange: 80,
		},
		Kind: kind,
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	hostile.SetWorld(state)
	state.Spawn(hostile, x, y, 0, 0)
	return hostile
}

func TestHostileMonsterKind(t *testing.T) {
	tests := []struct {
		kind InstanceKind
		want bool
	}{
		{"Monster", true},
		{"RaidBoss", true},
		{"GrandBoss", true},
		{"FeedableBeast", true},
		{"FestivalMonster", true},
		{"Chest", true},
		{"HalishaChest", true},
		{"Guard", false},
		{"SiegeGuard", false},
		{"FriendlyMonster", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			state := world.New()
			hostile := newTestHostileOfKind(t, state, tt.kind, 1, 100, 100)
			if got := hostile.MonsterKind(); got != tt.want {
				t.Fatalf("MonsterKind() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostileRandomNearbyMonsterExcludesChestAndNonMonsterKinds(t *testing.T) {
	state := world.New()
	self := newTestHostileOfKind(t, state, "Monster", 1, 100, 100)
	newTestHostileOfKind(t, state, "Chest", 2, 110, 100)
	newTestHostileOfKind(t, state, "Guard", 3, 110, 105)

	if _, ok := self.RandomNearbyMonster(600); ok {
		t.Fatal("RandomNearbyMonster() found a candidate with only a chest and a guard nearby, want none")
	}

	halisha := newTestHostileOfKind(t, state, "HalishaChest", 4, 110, 110)

	got, ok := self.RandomNearbyMonster(600)
	if !ok {
		t.Fatal("RandomNearbyMonster() found no candidate with a HalishaChest nearby, want one")
	}
	if got.ObjectID() != halisha.ObjectID() {
		t.Fatalf("RandomNearbyMonster() candidate = %d, want the HalishaChest (%d)", got.ObjectID(), halisha.ObjectID())
	}
}

func TestHostileRandomNearbyCombatantExcludesOnlyChest(t *testing.T) {
	state := world.New()
	self := newTestHostileOfKind(t, state, "Monster", 1, 100, 100)
	newTestHostileOfKind(t, state, "Chest", 2, 110, 100)

	if _, ok := self.RandomNearbyCombatant(1000); ok {
		t.Fatal("RandomNearbyCombatant() found a candidate with only a chest nearby, want none")
	}

	guard := newTestHostileOfKind(t, state, "Guard", 3, 110, 105)

	got, ok := self.RandomNearbyCombatant(1000)
	if !ok {
		t.Fatal("RandomNearbyCombatant() found no candidate with a guard nearby, want one")
	}
	if got.ObjectID() != guard.ObjectID() {
		t.Fatalf("RandomNearbyCombatant() candidate = %d, want the guard (%d)", got.ObjectID(), guard.ObjectID())
	}
}

func TestHostileStopMostHatedTargetClearsOnlyTheTopThreatEntry(t *testing.T) {
	hostile := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	top := &hostileTarget{id: 200}
	other := &hostileTarget{id: 201}
	hostile.AddDamageHate(top, 0, 100)
	hostile.AddDamageHate(other, 0, 10)

	hostile.StopMostHatedTarget()

	if got := hostile.AI().Threats().Hate(top); got != 0 {
		t.Fatalf("top threat hate after StopMostHatedTarget = %v, want 0", got)
	}
	if got := hostile.AI().Threats().Hate(other); got != 10 {
		t.Fatalf("other threat hate after StopMostHatedTarget = %v, want unchanged 10", got)
	}
}
