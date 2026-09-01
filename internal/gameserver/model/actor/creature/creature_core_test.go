package creature

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ---- from death_test.go ----
type deathTestActor struct {
	id   int32
	mu   sync.Mutex
	dead bool
}

func (a *deathTestActor) ObjectID() int32 { return a.id }

func (a *deathTestActor) MarkDead() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dead {
		return false
	}
	a.dead = true
	return true
}

type recordingRewarder struct {
	calls []DeathActor
}

func (r *recordingRewarder) CalculateRewards(killer DeathActor) {
	r.calls = append(r.calls, killer)
}

func TestDieAppliesOnce(t *testing.T) {
	actor := &deathTestActor{id: 1}
	killer := &deathTestActor{id: 2}
	rewards := &recordingRewarder{}

	if !Die(actor, killer, rewards) {
		t.Fatal("Die() = false, want true on first kill")
	}
	if len(rewards.calls) != 1 || rewards.calls[0] != killer {
		t.Fatalf("rewards.calls = %v, want one call with killer", rewards.calls)
	}

	if Die(actor, killer, rewards) {
		t.Fatal("Die() = true, want false on repeat kill")
	}
	if len(rewards.calls) != 1 {
		t.Fatalf("rewards.calls after repeat = %v, want unchanged", rewards.calls)
	}
}

func TestDieNilRewarderIsNoOp(t *testing.T) {
	actor := &deathTestActor{id: 1}
	if !Die(actor, nil, nil) {
		t.Fatal("Die() = false, want true with nil killer/rewards")
	}
}

func TestDieConcurrentOnlyOneWinner(t *testing.T) {
	actor := &deathTestActor{id: 1}
	rewards := &recordingRewarder{}

	const attempts = 50
	results := make(chan bool, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- Die(actor, nil, rewards)
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

// ---- from formula_random_damage_test.go ----
// randomDamageTestActor is a minimal FormulaActor stub for exercising
// RandomDamageMultiplier in isolation.
type randomDamageTestActor struct {
	level         int
	spread        int
	roll          int
	attackType    item.WeaponType
	pSkillEvasion float64
}

func (a *randomDamageTestActor) Position() (int, int, int)      { return 0, 0, 0 }
func (a *randomDamageTestActor) ObjectID() int32                { return 1 }
func (a *randomDamageTestActor) Heading() int                   { return 0 }
func (a *randomDamageTestActor) Level() int                     { return a.level }
func (a *randomDamageTestActor) STR() int                       { return 0 }
func (a *randomDamageTestActor) CON() int                       { return 0 }
func (a *randomDamageTestActor) DEX() int                       { return 0 }
func (a *randomDamageTestActor) INT() int                       { return 0 }
func (a *randomDamageTestActor) WIT() int                       { return 0 }
func (a *randomDamageTestActor) MEN() int                       { return 0 }
func (a *randomDamageTestActor) PAtk() float64                  { return 0 }
func (a *randomDamageTestActor) PDef() float64                  { return 0 }
func (a *randomDamageTestActor) MAtk() float64                  { return 0 }
func (a *randomDamageTestActor) MDef() float64                  { return 0 }
func (a *randomDamageTestActor) MagicCriticalRate() float64     { return 0 }
func (a *randomDamageTestActor) AttackType() item.WeaponType    { return a.attackType }
func (a *randomDamageTestActor) SoulshotCharged() bool          { return false }
func (a *randomDamageTestActor) SpiritshotCharged() bool        { return false }
func (a *randomDamageTestActor) BlessedSpiritshotCharged() bool { return false }
func (a *randomDamageTestActor) CalcStat(kind stat.Stat, base float64) float64 {
	if kind == stat.PSkillEvasion {
		return a.pSkillEvasion
	}
	return base
}
func (a *randomDamageTestActor) RandomDamageSpread() int { return a.spread }
func (a *randomDamageTestActor) Roll(int) int            { return a.roll }

var _ FormulaActor = (*randomDamageTestActor)(nil)

func TestResolvePhysicalSkillInputSkipsEvasionForUnarmedAndBow(t *testing.T) {
	target := &randomDamageTestActor{pSkillEvasion: 100}
	for _, attackType := range []item.WeaponType{item.WeaponFist, item.WeaponBow} {
		in, ok := ResolvePhysicalSkillInput(&randomDamageTestActor{attackType: attackType}, target, modelskill.Definition{}, false, 1)
		if !ok || in.Evaded {
			t.Fatalf("AttackType %v evaded = %v, ok = %v; want false, true", attackType, in.Evaded, ok)
		}
	}

	in, ok := ResolvePhysicalSkillInput(&randomDamageTestActor{attackType: item.WeaponSword}, target, modelskill.Definition{}, false, 1)
	if !ok || !in.Evaded {
		t.Fatalf("sword evaded = %v, ok = %v; want true, true", in.Evaded, ok)
	}
}

// Mirrors Creature.getRandomDamageMultiplier (Creature.java:1699-1710):
// weaponless attackers (spread == -1, the sentinel) roll `5 + sqrt(level)`
// spread, not a fixed 1x.
func TestRandomDamageMultiplierWeaponlessUsesLevelSpread(t *testing.T) {
	// level 50 -> spread = 5 + int(sqrt(50)) = 5 + 7 = 12
	attacker := &randomDamageTestActor{level: 50, spread: -1, roll: 2*12 + 1 - 1} // max roll
	got := RandomDamageMultiplier(attacker, modelskill.Definition{})
	want := 1 + float64(12)/100
	if got != want {
		t.Fatalf("RandomDamageMultiplier() = %v, want %v (max-roll bound for level 50 weaponless)", got, want)
	}

	attacker = &randomDamageTestActor{level: 50, spread: -1, roll: 0} // min roll
	got = RandomDamageMultiplier(attacker, modelskill.Definition{})
	want = 1 - float64(12)/100
	if got != want {
		t.Fatalf("RandomDamageMultiplier() = %v, want %v (min-roll bound for level 50 weaponless)", got, want)
	}
}

// A weapon-supplied spread still takes priority over the level fallback.
func TestRandomDamageMultiplierWeaponSpreadTakesPriority(t *testing.T) {
	attacker := &randomDamageTestActor{level: 50, spread: 20, roll: 0}
	got := RandomDamageMultiplier(attacker, modelskill.Definition{})
	want := 1 - float64(20)/100
	if got != want {
		t.Fatalf("RandomDamageMultiplier() = %v, want %v (weapon spread should win over level fallback)", got, want)
	}
}

// A weapon with an explicit or defaulted 0 random-damage spread (e.g. item
// 8763 "Elrokian Trap", which has no random_damage attribute) must resolve
// to a neutral 1x, NOT the level fallback — Java's gate is
// `activeWeapon != null`, not "spread > 0" (Creature.java:1699-1709).
func TestRandomDamageMultiplierZeroSpreadWeaponStaysNeutral(t *testing.T) {
	attacker := &randomDamageTestActor{level: 50, spread: 0, roll: 0}
	got := RandomDamageMultiplier(attacker, modelskill.Definition{})
	if got != 1 {
		t.Fatalf("RandomDamageMultiplier() = %v, want 1 (0-spread weapon must not trigger the level fallback)", got)
	}
}

// ---- from health_test.go ----
func TestHealthDamageClampsAndReportsFirstDeath(t *testing.T) {
	current := 10.0
	h := NewHealth(&current)

	if h.Damage(-5) {
		t.Fatal("Damage(-5) = true, want false")
	}
	if current != 10 {
		t.Fatalf("current after negative damage = %v, want 10", current)
	}

	if h.Damage(4) {
		t.Fatal("Damage(4) = true, want false")
	}
	if current != 6 {
		t.Fatalf("current after partial damage = %v, want 6", current)
	}

	if !h.Damage(99) {
		t.Fatal("Damage(99) = false, want first death")
	}
	if current != 0 {
		t.Fatalf("current after lethal damage = %v, want 0", current)
	}

	if h.Damage(1) {
		t.Fatal("Damage after death = true, want false")
	}
}

func TestHealthSetCurrentLeavesDeadHealthAlone(t *testing.T) {
	current := 0.0
	h := NewHealth(&current)

	h.SetCurrent(5)

	if current != 0 {
		t.Fatalf("current after SetCurrent on dead health = %v, want 0", current)
	}
}

// ---- from live_test.go ----
type liveGeo struct {
	canMove bool
	height  int16
}

func (g liveGeo) CanMove(_, _, _, _, _, _ int) bool { return g.canMove }
func (g liveGeo) Height(_, _, _ int) int16          { return g.height }

// liveGeo does not exercise pathfinding or fall-back resolution: tests that
// build it either walk a clear line or simulate a fully blocked one.
func (g liveGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (g liveGeo) Walkable(int, int, int) bool                                 { return true }
func (g liveGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

func TestLiveOwnsOneMovementState(t *testing.T) {
	live, err := NewLive(location.Location{X: 10, Y: 20, Z: 30}, 50, liveGeo{canMove: true, height: 30}, nil)
	if err != nil {
		t.Fatal(err)
	}

	first := live.Move()
	if first != &live.movement {
		t.Fatal("Move() does not return the embedded movement state")
	}

	if _, err := first.MoveToLocation(location.Location{X: 60, Y: 20, Z: 999}); err != nil {
		t.Fatal(err)
	}
	second := live.Move()
	if second != first {
		t.Fatal("Move() returned a different movement state")
	}
	if got := second.Destination(); got != (location.Location{X: 60, Y: 20, Z: 30}) {
		t.Fatalf("Destination() = %+v, want the accepted target", got)
	}

	if _, err := second.MoveToLocation(location.Location{X: 70, Y: 20, Z: 999}); err != nil {
		t.Fatal(err)
	}
	if live.Move() != first {
		t.Fatal("repeated movement replaced the embedded movement state")
	}
	if got := first.Destination(); got != (location.Location{X: 70, Y: 20, Z: 30}) {
		t.Fatalf("Destination() = %+v, want the second accepted target", got)
	}
}

func TestLiveMovementStateIsPerCreature(t *testing.T) {
	geo := liveGeo{canMove: true, height: 30}
	first, err := NewLive(location.Location{X: 0, Y: 0, Z: 30}, 100, geo, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLive(location.Location{X: 100, Y: 0, Z: 30}, 100, geo, nil)
	if err != nil {
		t.Fatal(err)
	}

	if first.Move() == second.Move() {
		t.Fatal("two live creatures share movement state")
	}
	if _, err := first.Move().MoveToLocation(location.Location{X: 50, Y: 0, Z: 999}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Move().MoveToLocation(location.Location{X: 150, Y: 0, Z: 999}); err != nil {
		t.Fatal(err)
	}

	if got := first.Move().Destination(); got != (location.Location{X: 50, Y: 0, Z: 30}) {
		t.Fatalf("first Destination() = %+v, want its own target", got)
	}
	if got := second.Move().Destination(); got != (location.Location{X: 150, Y: 0, Z: 30}) {
		t.Fatalf("second Destination() = %+v, want its own target", got)
	}
}

func newTestLive(t *testing.T) *Live {
	t.Helper()
	live, err := NewLive(location.Location{}, 0, liveGeo{canMove: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return live
}

// ccTestTarget satisfies every optional target interface a core effect's
// hooks type-assert against, so the effect always activates regardless of
// which one is under test.
type ccTestTarget struct{}

func (ccTestTarget) ObjectID() int32                                    { return 0 }
func (ccTestTarget) Dead() bool                                         { return false }
func (ccTestTarget) FleeFrom(effector effect.Participant, distance int) {}

func addTestEffect(t *testing.T, live *Live, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = ccTestTarget{}
	live.EffectList().Add(e)
	return e
}

func TestLiveCrowdControlGettersTrackActiveEffectsAndClearOnRemoval(t *testing.T) {
	tests := []struct {
		name       string
		effectName string
		get        func(*Live) bool
	}{
		{"Stunned", "Stun", (*Live).Stunned},
		{"Rooted", "Root", (*Live).Rooted},
		{"Sleeping", "Sleep", (*Live).Sleeping},
		{"Afraid", "Fear", (*Live).Afraid},
		{"ImmobileUntilAttacked", "ImmobileUntilAttacked", (*Live).ImmobileUntilAttacked},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			live := newTestLive(t)
			if tt.get(live) {
				t.Fatalf("%s() = true before any effect is active", tt.name)
			}

			e := addTestEffect(t, live, tt.effectName)
			if !tt.get(live) {
				t.Fatalf("%s() = false with the effect active", tt.name)
			}

			live.EffectList().Remove(e)
			if tt.get(live) {
				t.Fatalf("%s() = true after the effect was removed", tt.name)
			}
		})
	}
}

func TestLiveParalyzedUnionsManualLockAndActiveEffect(t *testing.T) {
	live := newTestLive(t)
	if live.Paralyzed() {
		t.Fatal("Paralyzed() = true on a fresh creature")
	}

	if !live.SetParalyzed(true) {
		t.Fatal("SetParalyzed(true) reported no change on first call")
	}
	if !live.Paralyzed() {
		t.Fatal("Paralyzed() = false with only the manual lock set, want true (OR-union)")
	}
	if live.SetParalyzed(true) {
		t.Fatal("SetParalyzed(true) reported a change on a no-op call")
	}

	if !live.SetParalyzed(false) {
		t.Fatal("SetParalyzed(false) reported no change")
	}
	if live.Paralyzed() {
		t.Fatal("Paralyzed() = true after the manual lock was cleared and no effect is active")
	}

	e := addTestEffect(t, live, "Paralyze")
	if !live.Paralyzed() {
		t.Fatal("Paralyzed() = false with an active paralyze effect and no manual lock")
	}

	live.EffectList().Remove(e)
	if live.Paralyzed() {
		t.Fatal("Paralyzed() = true after the paralyze effect was removed")
	}
}

func TestLiveImmobilizedReportsChange(t *testing.T) {
	live := newTestLive(t)
	if live.Immobilized() {
		t.Fatal("Immobilized() = true on a fresh creature")
	}

	if !live.SetImmobilized(true) {
		t.Fatal("SetImmobilized(true) reported no change on first call")
	}
	if !live.Immobilized() {
		t.Fatal("Immobilized() = false after SetImmobilized(true)")
	}
	if live.SetImmobilized(true) {
		t.Fatal("SetImmobilized(true) reported a change on a no-op call")
	}

	if !live.SetImmobilized(false) {
		t.Fatal("SetImmobilized(false) reported no change")
	}
	if live.Immobilized() {
		t.Fatal("Immobilized() = true after SetImmobilized(false)")
	}
}

func TestLiveTeleportingReportsChange(t *testing.T) {
	live := newTestLive(t)
	if live.Teleporting() {
		t.Fatal("Teleporting() = true on a fresh creature")
	}

	if !live.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change on first call")
	}
	if !live.Teleporting() {
		t.Fatal("Teleporting() = false after SetTeleporting(true)")
	}
	if live.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported a change on a no-op call")
	}

	if !live.SetTeleporting(false) {
		t.Fatal("SetTeleporting(false) reported no change")
	}
	if live.Teleporting() {
		t.Fatal("Teleporting() = true after SetTeleporting(false)")
	}
}

func TestLiveInvulReportsChange(t *testing.T) {
	live := newTestLive(t)
	if live.Invul() {
		t.Fatal("Invul() = true on a fresh creature")
	}
	if !live.SetInvul(true) || !live.Invul() {
		t.Fatal("SetInvul(true) did not enable invulnerability")
	}
	if live.SetInvul(true) {
		t.Fatal("repeating SetInvul(true) reported a change")
	}
	if !live.SetInvul(false) || live.Invul() {
		t.Fatal("SetInvul(false) did not disable invulnerability")
	}
	if !live.SetTeleporting(true) || !live.Invul() {
		t.Fatal("teleporting creature was not invulnerable")
	}
	if !live.SetTeleporting(false) || live.Invul() {
		t.Fatal("cleared teleport protection left creature invulnerable")
	}
}

type facingStub struct {
	x, y, z, heading int
}

func (s facingStub) Position() (int, int, int) { return s.x, s.y, s.z }
func (s facingStub) Heading() int              { return s.heading }

type currentHeadingStub struct {
	facingStub
	current int
}

func (s currentHeadingStub) CurrentHeading() int { return s.current }

type nightStub bool

func (n nightStub) IsNight() bool { return bool(n) }

func TestAttackFacingBehindFrontAndSide(t *testing.T) {
	target := facingStub{heading: 0}

	behind, inFront := AttackFacing(target, facingStub{x: -100})
	if !behind || inFront {
		t.Fatalf("behind attacker: behind=%v inFront=%v, want true, false", behind, inFront)
	}

	behind, inFront = AttackFacing(target, facingStub{x: 100})
	if behind || !inFront {
		t.Fatalf("front attacker: behind=%v inFront=%v, want false, true", behind, inFront)
	}

	behind, inFront = AttackFacing(target, facingStub{y: 100})
	if behind || inFront {
		t.Fatalf("side attacker: behind=%v inFront=%v, want false, false", behind, inFront)
	}
}

func TestAttackFacingPrefersCurrentHeading(t *testing.T) {
	// Heading() faces north (16384); CurrentHeading faces east (0).
	target := currentHeadingStub{facingStub: facingStub{heading: 16384}, current: 0}

	behind, inFront := AttackFacing(target, facingStub{x: -100})
	if !behind || inFront {
		t.Fatalf("CurrentHeading 0, attacker x=-100: behind=%v inFront=%v, want true, false", behind, inFront)
	}
}

func TestNightReadsInstalledSource(t *testing.T) {
	prev := nightSource
	t.Cleanup(func() { SetNightSource(prev) })

	SetNightSource(nil)
	if Night() {
		t.Fatal("Night() = true with no source, want day")
	}

	SetNightSource(nightStub(true))
	if !Night() {
		t.Fatal("Night() = false after installing night source")
	}

	SetNightSource(nightStub(false))
	if Night() {
		t.Fatal("Night() = true after installing day source")
	}
}

func TestLiveNilReceiverGettersDoNotPanic(t *testing.T) {
	var live *Live

	if live.EffectList() != nil {
		t.Fatal("EffectList() on a nil receiver = non-nil")
	}
	if live.Stunned() || live.Rooted() || live.Sleeping() || live.Afraid() || live.ImmobileUntilAttacked() || live.Paralyzed() || live.Immobilized() || live.Teleporting() {
		t.Fatal("a crowd-control getter on a nil receiver reported true")
	}
	if live.SetParalyzed(true) || live.SetImmobilized(true) || live.SetTeleporting(true) {
		t.Fatal("a crowd-control setter on a nil receiver reported a change")
	}
}
