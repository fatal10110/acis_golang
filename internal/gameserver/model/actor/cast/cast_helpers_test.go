package cast

import (
	"time"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/skill/skilltest"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from abort_test.go ----
// abortActor adds the two optional owner surfaces the abort funnel consults
// to the shared cast test actor.
type abortActor struct {
	*testActor
	allDisabled bool
	enableCalls int
	signetExits int
}

func (a *abortActor) AllSkillsDisabled() bool { return a.allDisabled }

func (a *abortActor) EnableAllSkills() {
	a.enableCalls++
	a.allDisabled = false
}

func (a *abortActor) ExitSignetGround() { a.signetExits++ }

func newAbortController() (*Controller, *abortActor, *event.Recorder) {
	actor := &abortActor{testActor: scalingActor()}
	rec := &event.Recorder{}
	ctrl := NewController(actor, rec)
	return ctrl, actor, rec
}

// fakeCastCreature satisfies both attackable.Combatant (the ai package's
// desire/target surface) and skilltarget.Actor (the target-resolution
// surface ApplyEffects needs), so the same fake can stand in for an
// AIController's target on both sides of the bridge it builds.
type fakeCastCreature struct {
	world.Presence
	skilltest.Creature
	id      int32
	x, y, z int
	dead    bool
	kind    modelactor.Kind
}

func (f *fakeCastCreature) ObjectID() int32 { return f.id }

func (f *fakeCastCreature) Position() (int, int, int) { return f.x, f.y, f.z }

func (f *fakeCastCreature) Heading() int { return 0 }

func (f *fakeCastCreature) Dead() bool { return f.dead }

func (f *fakeCastCreature) Kind() modelactor.Kind { return f.kind }

func (f *fakeCastCreature) SiegeGuard() bool { return false }

func (f *fakeCastCreature) AlikeDead() bool { return f.dead }

func (f *fakeCastCreature) AttackableBy(skilltarget.Actor) bool { return true }

func (f *fakeCastCreature) AttackableWithoutForceBy(skilltarget.Actor) bool { return true }

var (
	_ attackable.Combatant = (*fakeCastCreature)(nil)
	_ skilltarget.Actor    = (*fakeCastCreature)(nil)
	_ Target               = (*fakeCastCreature)(nil)
)

// effectsKnown is a fixed roster used as the radius-scan source for
// area/aura target handlers under test.
type effectsKnown []skilltarget.Actor

func (k effectsKnown) ForEachKnownCreatureInRadius(anchor skilltarget.Actor, _ int, fn func(skilltarget.Actor)) {
	for _, c := range k {
		if c.ObjectID() == anchor.ObjectID() {
			continue
		}
		fn(c)
	}
}

// recordingSkillHandler records every Cast it receives instead of applying
// any actual skill logic, so tests can assert on exactly what ApplyEffects
// resolved and handed off.
type recordingSkillHandler struct {
	skillTypes []string
	calls      []handlerskill.Cast
	result     handlerskill.Result
}

func (h *recordingSkillHandler) Types() []string { return h.skillTypes }

func (h *recordingSkillHandler) Use(c handlerskill.Cast) { h.calls = append(h.calls, c) }

func (h *recordingSkillHandler) UseResult(c handlerskill.Cast) handlerskill.Result {
	h.Use(c)
	return h.result
}

func newEffectHandlers(known skilltarget.Known, skillType string, rec *recordingSkillHandler) EffectHandlers {
	rec.skillTypes = []string{skillType}
	return EffectHandlers{
		Targets: skilltarget.NewRegistry(known),
		Skills:  handlerskill.NewRegistry(rec),
	}
}

// ---- from player_actor_test.go ----
// permissiveGeo is a test-only move.Geo that permits every move, needed
// only because creature.NewLive requires a non-nil Geo.
type permissiveGeo struct{}

func (permissiveGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }

func (permissiveGeo) Height(x, y, z int) int16 { return int16(z) }

func (permissiveGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}

func (permissiveGeo) Walkable(int, int, int) bool { return true }

func (g permissiveGeo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g permissiveGeo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}

func (permissiveGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

// ---- from testfakes_test.go ----
type testTarget struct{}

func (testTarget) ObjectID() int32 { return 1 }

func (testTarget) Position() (x, y, z int) { return 0, 0, 0 }

type testActor struct {
	mp, hp int

	mAtkSpd, pAtkSpd                  int
	magicReuseRate, physicalReuseRate float64
	initialCost, hitCost              int
	spiritshot, blessedSpiritshot     bool
	magicMuted, physicalMuted         bool
	mastery                           bool

	items        map[int]int
	disabledKeys map[int32]bool
	disabled     []testCooldown
	reuses       []testReuse

	allDisabled bool
	held        int32
}

func (a *testActor) HeldItemTypeMask() int32 { return a.held }

func (a *testActor) AllSkillsDisabled() bool { return a.allDisabled }

func (a *testActor) EnableAllSkills() { a.allDisabled = false }

type testCooldown struct {
	key   int32
	delay time.Duration
}

type testReuse struct {
	ref   modelskill.Ref
	key   int32
	delay time.Duration
}

func (a *testActor) AttackSpeed(magic bool) int {
	if magic {
		if a.mAtkSpd == 0 {
			return 333
		}
		return a.mAtkSpd
	}
	if a.pAtkSpd == 0 {
		return 333
	}
	return a.pAtkSpd
}

func (a *testActor) ReuseRate(magic bool) float64 {
	if magic {
		if a.magicReuseRate == 0 {
			return 1
		}
		return a.magicReuseRate
	}
	if a.physicalReuseRate == 0 {
		return 1
	}
	return a.physicalReuseRate
}

func (a *testActor) MP() int { return a.mp }

func (a *testActor) HP() int { return a.hp }

func (a *testActor) MPInitialCost(def modelskill.Definition) int {
	if a.initialCost != 0 {
		return a.initialCost
	}
	return def.MPInitialConsume
}

func (a *testActor) MPCost(def modelskill.Definition) int {
	if a.hitCost != 0 {
		return a.hitCost
	}
	return def.MPConsume
}

func (a *testActor) ReduceMP(n int) { a.mp -= n }

func (a *testActor) ReduceHP(n int) { a.hp -= n }

func (a *testActor) SkillDisabled(key int32) bool {
	return a.disabledKeys[key]
}

func (a *testActor) DisableSkill(key int32, delay time.Duration) {
	a.disabled = append(a.disabled, testCooldown{key: key, delay: delay})
}

func (a *testActor) AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration) {
	a.reuses = append(a.reuses, testReuse{ref: ref, key: key, delay: delay})
}

func (a *testActor) MagicMuted() bool { return a.magicMuted }

func (a *testActor) PhysicalMuted() bool { return a.physicalMuted }

func (a *testActor) SpiritshotCharged() bool { return a.spiritshot }

func (a *testActor) BlessedSpiritshotCharged() bool { return a.blessedSpiritshot }

func (a *testActor) SkillMastery(modelskill.Definition) bool {
	return a.mastery
}

func (a *testActor) ItemCount(itemID int) int {
	if a.items == nil {
		return 0
	}
	return a.items[itemID]
}

func (a *testActor) ConsumeItem(itemID, count int) bool {
	if a.items == nil || a.items[itemID] < count {
		return false
	}
	a.items[itemID] -= count
	return true
}

func (*fakeCastCreature) CanSeeTarget(skilltarget.Actor) bool { return true }

func (abortActor) DecreaseCharges(int) bool { return false }

func (abortActor) GroundTargetUnset() bool { return false }

func (abortActor) IncreaseCharges(int, int) bool { return false }

func (testActor) DecreaseCharges(int) bool { return false }

func (testActor) ExitSignetGround() {}

func (testActor) GroundTargetUnset() bool { return false }

func (testActor) IncreaseCharges(int, int) bool { return false }

func (*fakeCastCreature) BroadcastSkillLaunched(int32, int32, []int32) {}

func (*fakeCastCreature) BroadcastSkillUse(int32, int, int, int, int32, int32, int, int) {}

var (
	_ SkillCaster  = (*fakeCastCreature)(nil)
	_ SkillCaster  = (*fakeBroadcastingCaster)(nil)
	_ SkillCaster  = (*effectsActor)(nil)
	_ SkillCaster  = (*pvpEffectsActor)(nil)
	_ SkillCaster  = (*cursePvpEffectsActor)(nil)
	_ AICaster     = (*fakeCastCreature)(nil)
	_ AICaster     = (*fakeBroadcastingCaster)(nil)
	_ effect.Actor = (*fakeCubicHealTarget)(nil)
)

func (*fakeCastCreature) NotePvPSkillTargets([]attackable.Combatant, bool, string) {}

func (*fakeCastCreature) TestCursesOnSkillSee(modelskill.Definition, []skilltarget.Actor) bool {
	return false
}

func idleQueue() *sim.Queue { return sim.NewInline(time.Unix(0, 0)).NewQueue("test") }
