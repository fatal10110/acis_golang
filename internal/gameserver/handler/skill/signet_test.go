package skill

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// tickInterval is one live effect tick (Template.Time: 1 second).
const tickInterval = time.Second

// signetFakeCaster is a minimal player-shaped caster for signet handler
// tests: it can be positioned and identified, owns its own live effect
// list (for the self-targeted SIGNET_CASTTIME family), and can pay MP.
type signetFakeCaster struct {
	neutralCreature
	world.Presence
	fakeActor
	id         int32
	x, y, z    int
	gx, gy, gz int
	dead       bool
	mp         float64
	list       *effect.List
	// clock drives queue, the caster's own queue, whose clock the caster's
	// list and every effect point it spawns measure effect periods on.
	clock *sim.Inline
	queue *sim.Queue
}

func newSignetFakeCaster(id int32, x, y, z int, mp float64) *signetFakeCaster {
	c := &signetFakeCaster{id: id, x: x, y: y, z: z, mp: mp, clock: sim.NewInline(time.Unix(1000, 0))}
	c.queue = c.clock.NewQueue("caster")
	c.list = effect.NewList(noopStatOwner{}, effect.WithClock(c.queue))
	return c
}

func (c *signetFakeCaster) Queue() *sim.Queue { return c.queue }

func (c *signetFakeCaster) AlikeDead() bool           { return c.dead }
func (c *signetFakeCaster) ObjectID() int32           { return c.id }
func (c *signetFakeCaster) Position() (int, int, int) { return c.x, c.y, c.z }
func (c *signetFakeCaster) GroundTarget() (int, int, int) {
	return c.gx, c.gy, c.gz
}
func (c *signetFakeCaster) EffectList() *effect.List { return c.list }
func (c *signetFakeCaster) MPValue() float64         { return c.mp }
func (c *signetFakeCaster) ReduceMP(v float64) float64 {
	if v > c.mp {
		v = c.mp
	}
	c.mp -= v
	return v
}

type noopStatOwner struct{}

func (noopStatOwner) AddStatFuncs([]effect.Mod)          {}
func (noopStatOwner) RemoveStatsByOwner(effect.ModOwner) {}
func (noopStatOwner) MaxBuffCount() int                  { return 0 }

// signetFakeTarget is a minimal nearby-creature fixture: identifiable,
// positioned (once spawned into a world.State via its embedded Presence),
// killable, magic-damageable, and carrying its own effect list (for the
// dance-cancel and unsummon families).
type signetFakeTarget struct {
	world.Presence
	neutralCreature

	id    int32
	dead  bool
	peace bool
	hp    float64

	magicInput formulas.MagicDamageInput
	magicOK    bool

	list *effect.List

	unsummoned    bool
	selfSkillUses int
}

func newSignetFakeTarget(id int32) *signetFakeTarget {
	t := &signetFakeTarget{id: id}
	t.list = effect.NewList(noopStatOwner{})
	return t
}

func (t *signetFakeTarget) ObjectID() int32          { return t.id }
func (*signetFakeTarget) Kind() actor.Kind           { return actor.KindNPC }
func (t *signetFakeTarget) Dead() bool               { return t.dead }
func (t *signetFakeTarget) InPeaceZone() bool        { return t.peace }
func (t *signetFakeTarget) EffectList() *effect.List { return t.list }
func (t *signetFakeTarget) Unsummon()                { t.unsummoned = true }
func (t *signetFakeTarget) BroadcastSelfSkillUse(_, _ int32) {
	t.selfSkillUses++
}

func (t *signetFakeTarget) MagicDamageInput(caster creature.FormulaActor, skill modelskill.Definition, _ bool) (formulas.MagicDamageInput, bool) {
	return t.magicInput, t.magicOK
}

func (t *signetFakeTarget) ReduceHP(v float64, attacker attackable.Combatant, skill modelskill.Definition) {
	t.hp -= v
}

type fakeSignetTemplates struct {
	byID map[int]*npc.Template
}

func (f fakeSignetTemplates) Get(id int) (*npc.Template, bool) {
	tpl, ok := f.byID[id]
	return tpl, ok
}

type fakeSignetIDs struct {
	next int32
}

func (f *fakeSignetIDs) NextID() (int32, error) {
	f.next++
	return f.next, nil
}

type fakeSignetDefinitions struct {
	byRef map[modelskill.Ref]modelskill.Definition
}

func (f fakeSignetDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	d, ok := f.byRef[ref]
	return d, ok
}

func (f fakeSignetDefinitions) MaxLevel(id modelskill.ID) int {
	max := 0
	for ref := range f.byRef {
		if ref.ID == id && ref.Level > max {
			max = ref.Level
		}
	}
	return max
}

func newTestSignetHandler(defs Definitions) (signetHandler, *world.State, *event.Recorder) {
	state := world.New()
	templates := fakeSignetTemplates{byID: map[int]*npc.Template{
		13018: {ID: 13018, Type: "EffectPoint"},
	}}
	rec := &event.Recorder{}
	h := signetHandler{defs: defs, templates: templates, ids: &fakeSignetIDs{}, world: state, newSink: func(*npc.EffectPoint) event.Sink { return rec }, effects: effect.Env{Activity: newSignetActivity()}}
	return h, state, rec
}

// signetActivity records which effect lists are registered for ticking.
type signetActivity struct {
	mu     sync.Mutex
	active map[*effect.List]bool
}

func newSignetActivity() *signetActivity {
	return &signetActivity{active: make(map[*effect.List]bool)}
}

func (a *signetActivity) SetActive(list *effect.List, active bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if active {
		a.active[list] = true
	} else {
		delete(a.active, list)
	}
}

func (a *signetActivity) isActive(list *effect.List) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.active[list]
}

// TestSignetPointListRegistersForTickingUntilDespawn pins the activity
// registry a spawned point's list reports to: without it the point's
// effects never reach the ticker, so they never expire and the point never
// despawns.
func TestSignetPointListRegistersForTickingUntilDespawn(t *testing.T) {
	h, state, _ := newTestSignetHandler(nil)
	activity := h.effects.Activity.(*signetActivity)

	h.Use(Cast{Caster: newSignetFakeCaster(1, 100, 100, 0, 100), Skill: modelskill.Definition{
		ID: 454, Level: 1, SkillType: "SIGNET", EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 2, Time: 1}},
	}})

	actors := findEffectPointObjects(state)
	if len(actors) != 1 {
		t.Fatalf("spawned actors = %d, want 1", len(actors))
	}
	list := actors[0].EffectList()
	if !activity.isActive(list) {
		t.Fatal("point's effect list not registered for ticking after its effect landed")
	}

	actors[0].Despawn()
	if activity.isActive(list) {
		t.Fatal("point's effect list still registered for ticking after Despawn")
	}
}

// TestNewDefaultRegistryWithSignetPassesActivityToSpawnedPoints covers the
// SignetDeps.Effects hop, not just the handler literal above.
func TestNewDefaultRegistryWithSignetPassesActivityToSpawnedPoints(t *testing.T) {
	state := world.New()
	activity := newSignetActivity()
	r := NewDefaultRegistryWithSignet(nil, true, SignetDeps{
		Effects:   effect.Env{Activity: activity},
		Templates: fakeSignetTemplates{byID: map[int]*npc.Template{13018: {ID: 13018, Type: "EffectPoint"}}},
		IDs:       &fakeSignetIDs{},
		World:     state,
		NewSink:   func(*npc.EffectPoint) event.Sink { return &event.Recorder{} },
		Log:       zerolog.Nop(),
	})

	if !r.Use(Cast{Caster: newSignetFakeCaster(1, 100, 100, 0, 100), Skill: modelskill.Definition{
		ID: 454, Level: 1, SkillType: "SIGNET", EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 2, Time: 1}},
	}}) {
		t.Fatal("registry has no SIGNET handler")
	}

	actors := findEffectPointObjects(state)
	if len(actors) != 1 {
		t.Fatalf("spawned actors = %d, want 1", len(actors))
	}
	if !activity.isActive(actors[0].EffectList()) {
		t.Fatal("SignetDeps.Effects did not reach the spawned point's effect list")
	}
}

func TestSignetBuffAppliesSubSkillToNearbyTargetsAndDespawns(t *testing.T) {
	defs := fakeSignetDefinitions{byRef: map[modelskill.Ref]modelskill.Definition{
		{ID: 5123, Level: 1}: {
			ID: 5123, Level: 1, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Count: 1}},
		},
	}}
	h, state, _ := newTestSignetHandler(defs)

	caster := newSignetFakeCaster(1, 100, 100, 0, 100)
	target := newSignetFakeTarget(2)
	state.Spawn(target, 120, 100, 0, 0)

	def := modelskill.Definition{
		ID: 454, Level: 1, SkillType: "SIGNET", EffectID: 5123, EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 2, Time: 1}},
	}

	h.Use(Cast{Caster: caster, Skill: def})

	all := findEffectPointObjects(state)
	if len(all) != 1 {
		t.Fatalf("spawned actors = %d, want 1", len(all))
	}
	actor := all[0]

	// First tick: the sub-skill's buff lands on the nearby target.
	caster.clock.Advance(tickInterval)
	actor.EffectList().Tick()

	if len(target.list.All()) != 1 {
		t.Fatalf("target effects after tick = %d, want 1 (sub-skill applied)", len(target.list.All()))
	}

	// Second (final, Count: 2) tick exhausts the driving effect, which must
	// despawn the actor on exit.
	caster.clock.Advance(tickInterval)
	actor.EffectList().Tick()

	if _, ok := state.Object(actor.ObjectID()); ok {
		t.Fatal("actor still tracked in world after its driving effect exited")
	}
}

func TestSignetSpawnsAtGroundTarget(t *testing.T) {
	h, state, _ := newTestSignetHandler(nil)
	caster := newSignetFakeCaster(1, 100, 100, 0, 100)
	caster.gx, caster.gy, caster.gz = 300, 400, 50

	h.Use(Cast{Caster: caster, Skill: modelskill.Definition{
		ID: 454, Level: 1, SkillType: "SIGNET", Target: modelskill.TargetGround,
		EffectNpcID: 13018, Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 1}},
	}})

	actors := findEffectPointObjects(state)
	if len(actors) != 1 {
		t.Fatalf("spawned actors = %d, want 1", len(actors))
	}
	if x, y, z := actors[0].Position(); x != 300 || y != 400 || z != 50 {
		t.Fatalf("signet position = (%d,%d,%d), want ground target (300,400,50)", x, y, z)
	}
}

func TestSignetCasttimeMDamSpawnsPaysMPAndDamagesOnItsLiveTick(t *testing.T) {
	h, state, _ := newTestSignetHandler(nil)

	caster := newSignetFakeCaster(1, 100, 100, 0, 100)
	target := newSignetFakeTarget(2)
	target.magicOK = true
	target.magicInput = formulas.MagicDamageInput{MAtk: 100, MDef: 1, SkillPower: 1, PvPMul: 1, ElementalMul: 1}
	state.Spawn(target, 120, 100, 0, 0)

	def := modelskill.Definition{
		ID: 1419, Level: 1, SkillType: "SIGNET_CASTTIME", EffectNpcID: 13018, Radius: 180, MPConsume: 10,
		// Count: 3 exercises "skip the first two ticks" with the minimum
		// tick count that still reaches a third, live tick.
		SelfEffects: []modelskill.EffectTemplate{{Name: "SignetMDam", Self: true, Count: 3, Time: 1}},
	}

	h.Use(Cast{Caster: caster, Skill: def})

	actors := findEffectPointObjects(state)
	if len(actors) != 1 {
		t.Fatalf("spawned actors = %d, want 1", len(actors))
	}
	actor := actors[0]

	// Ticks 1 and 2 are the effect's documented warmup: no MP paid, no
	// damage dealt.
	caster.clock.Advance(tickInterval)
	caster.EffectList().Tick()
	caster.clock.Advance(tickInterval)
	caster.EffectList().Tick()
	if caster.mp != 100 {
		t.Fatalf("caster mp after warmup ticks = %v, want 100 (unpaid)", caster.mp)
	}
	if target.hp != 0 {
		t.Fatalf("target hp after warmup ticks = %v, want 0 (undamaged)", target.hp)
	}

	// Tick 3 is live (and, at Count: 3, also the effect's last): it pays
	// MP, deals damage, and despawns the actor on exit.
	caster.clock.Advance(tickInterval)
	caster.EffectList().Tick()

	if caster.mp != 90 {
		t.Fatalf("caster mp after live tick = %v, want 90", caster.mp)
	}
	if target.hp >= 0 {
		t.Fatalf("target hp after live tick = %v, want negative (damaged)", target.hp)
	}
	if _, ok := state.Object(actor.ObjectID()); ok {
		t.Fatal("actor still tracked in world after its driving effect exited")
	}
}

func TestSignetCasttimeAppliesEveryRecognizedSelfEffectTemplate(t *testing.T) {
	h, state, _ := newTestSignetHandler(nil)
	caster := newSignetFakeCaster(1, 100, 100, 0, 100)

	def := modelskill.Definition{
		ID: 1419, Level: 1, SkillType: "SIGNET_CASTTIME", EffectNpcID: 13018, Radius: 180, MPConsume: 10,
		SelfEffects: []modelskill.EffectTemplate{
			// An unrecognized template ahead of SignetMDam must not stop
			// dispatch from reaching it: useCasttime walks every self
			// effect template rather than filtering the slice down to one
			// known name before iterating.
			{Name: "SomeUnimplementedSelfEffect", Self: true, Count: 3, Time: 1},
			{Name: "SignetMDam", Self: true, Count: 3, Time: 1},
		},
	}

	h.Use(Cast{Caster: caster, Skill: def})

	if got := len(caster.EffectList().All()); got != 1 {
		t.Fatalf("caster active effects = %d, want 1 (only the recognized SignetMDam template)", got)
	}
	if actors := findEffectPointObjects(state); len(actors) != 1 {
		t.Fatalf("spawned actors = %d, want 1 (SignetMDam's own onStart spawn)", len(actors))
	}
}

func TestSignetCasttimeMDamDropsOnLackOfMP(t *testing.T) {
	h, _, _ := newTestSignetHandler(nil)

	caster := newSignetFakeCaster(1, 100, 100, 0, 5) // below the skill's mpConsume
	def := modelskill.Definition{
		ID: 1419, Level: 1, SkillType: "SIGNET_CASTTIME", EffectNpcID: 13018, Radius: 180, MPConsume: 10,
		SelfEffects: []modelskill.EffectTemplate{{Name: "SignetMDam", Self: true, Count: 3, Time: 1}},
	}
	h.Use(Cast{Caster: caster, Skill: def})

	caster.clock.Advance(tickInterval)
	caster.EffectList().Tick()
	caster.clock.Advance(tickInterval)
	caster.EffectList().Tick()
	caster.clock.Advance(tickInterval)
	caster.EffectList().Tick() // the live tick: insufficient MP

	if caster.mp != 5 {
		t.Fatalf("caster mp after failed tick = %v, want unchanged 5", caster.mp)
	}
	if len(caster.EffectList().All()) != 0 {
		t.Fatal("effect still active after a lack-of-MP tick, want removed")
	}
}

func TestSignetNoiseCancelsDanceEffectsAfterFirstTick(t *testing.T) {
	defs := fakeSignetDefinitions{byRef: map[modelskill.Ref]modelskill.Definition{
		{ID: 5124, Level: 1}: {ID: 5124, Level: 1, SkillType: "DEBUFF"},
	}}
	h, state, _ := newTestSignetHandler(defs)

	caster := newSignetFakeCaster(1, 100, 100, 0, 100)
	target := newSignetFakeTarget(2)
	state.Spawn(target, 120, 100, 0, 0)
	target.list.Add(&effect.Effect{Skill: effect.Skill{ID: 999, Dance: true}, Template: modelskill.EffectTemplate{Time: 60, Count: 1}, OnStart: func(*effect.Effect) bool { return true }})

	def := modelskill.Definition{
		ID: 455, Level: 1, SkillType: "SIGNET", EffectID: 5124, EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "SignetNoise", Count: 2, Time: 1}},
	}
	h.Use(Cast{Caster: caster, Skill: def})

	actor := findEffectPointObjects(state)[0]

	// First tick is a documented skip: the dance effect must survive it.
	caster.clock.Advance(tickInterval)
	actor.EffectList().Tick()
	if len(target.list.All()) != 1 {
		t.Fatalf("target effects after skipped first tick = %d, want 1", len(target.list.All()))
	}

	// Second (final) tick strips it.
	caster.clock.Advance(tickInterval)
	actor.EffectList().Tick()
	if len(target.list.All()) != 0 {
		t.Fatalf("target effects after second tick = %d, want 0 (dance stripped)", len(target.list.All()))
	}
}

func TestSignetAntiSummonUnsummonsAfterFirstTick(t *testing.T) {
	h, state, _ := newTestSignetHandler(nil)

	caster := newSignetFakeCaster(1, 100, 100, 0, 100)
	target := newSignetFakeTarget(2)
	state.Spawn(target, 120, 100, 0, 0)

	def := modelskill.Definition{
		ID: 1422, Level: 1, SkillType: "SIGNET", EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "SignetAntiSummon", Count: 2, Time: 1}},
	}
	h.Use(Cast{Caster: caster, Skill: def})

	actor := findEffectPointObjects(state)[0]

	caster.clock.Advance(tickInterval)
	actor.EffectList().Tick()
	if target.unsummoned {
		t.Fatal("target unsummoned on the skipped first tick")
	}
	if target.selfSkillUses != 0 {
		t.Fatalf("target self skill-use broadcasts on the skipped first tick = %d, want 0", target.selfSkillUses)
	}

	caster.clock.Advance(tickInterval)
	actor.EffectList().Tick()
	if !target.unsummoned {
		t.Fatal("target not unsummoned after the second tick")
	}
	if target.selfSkillUses != 1 {
		t.Fatalf("target self skill-use broadcasts after the second tick = %d, want 1 (MagicSkillUse self-cast)", target.selfSkillUses)
	}
}

// findEffectPointObjects scans state for every tracked *npc.EffectPoint.
func findEffectPointObjects(state *world.State) []*npc.EffectPoint {
	var out []*npc.EffectPoint
	for _, obj := range state.Objects() {
		if ep, ok := obj.(*npc.EffectPoint); ok {
			out = append(out, ep)
		}
	}
	return out
}

func (*signetFakeCaster) Kind() actor.Kind { return actor.KindNPC }

func (noopStatOwner) NotifyEffectAborted(modelskill.ID, int) {}

func (noopStatOwner) NotifyEffectDisappeared(modelskill.ID, int) {}

func (noopStatOwner) NotifyEffectWornOff(modelskill.ID, int) {}

func (noopStatOwner) UpdateEffectIcons() {}

// TestSignetOutlivesItsCastersQueue keeps an effect point's driving effect
// off the caster's queue. The point's OnExit is what despawns it, and it
// outlives the caster's session: bound to the caster's queue, a logout
// (detachLivePlayer closes that queue) would strand the point in world with
// its effect list registered forever.
func TestSignetOutlivesItsCastersQueue(t *testing.T) {
	defs := fakeSignetDefinitions{byRef: map[modelskill.Ref]modelskill.Definition{
		{ID: 5123, Level: 1}: {
			ID: 5123, Level: 1, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Count: 1}},
		},
	}}
	h, state, _ := newTestSignetHandler(defs)

	caster := newSignetFakeCaster(1, 100, 100, 0, 100)
	def := modelskill.Definition{
		ID: 454, Level: 1, SkillType: "SIGNET", EffectID: 5123, EffectNpcID: 13018, Radius: 180,
		Effects: []modelskill.EffectTemplate{{Name: "Signet", Count: 1, Time: 1}},
	}

	h.Use(Cast{Caster: caster, Skill: def})

	all := findEffectPointObjects(state)
	if len(all) != 1 {
		t.Fatalf("spawned actors = %d, want 1", len(all))
	}
	actor := all[0]
	if q := actor.EffectList().Queue(); q != nil {
		t.Fatal("effect point's list is bound to a queue; its expiry must not depend on one")
	}

	// The caster logs out: its queue is closed and drops every later task.
	caster.queue.Close()

	caster.clock.Advance(tickInterval)
	actor.EffectList().Tick()
	if _, ok := state.Object(actor.ObjectID()); ok {
		t.Fatal("effect point still in world after its driving effect exited")
	}
}
