package summon

import (
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type hitNight bool

func (n hitNight) IsNight() bool { return bool(n) }

func TestSummonMakeAttackHitAppliesFacingAndNight(t *testing.T) {
	place := func(t *testing.T, ax, ay int, night bool) (*Actor, *Actor) {
		t.Helper()
		target := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 1, Stats: CombatStats{DEX: 40, PDef: 50, MaxHP: 100}})
		attacker := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 1, Stats: CombatStats{DEX: 20, PDef: 50, MaxHP: 100}, Effects: effect.Env{Night: hitNight(night)}})
		state := world.New()
		state.Spawn(target, 0, 0, 0, 0)
		state.Spawn(attacker, ax, ay, 0, 0)
		return attacker, target
	}

	attacker, target := place(t, 100, 0, false)
	acc := int(attacker.Accuracy())
	eva := target.Evasion()
	frontRate := formulas.HitRate(acc, eva, 0, false, false, true)
	behindRate := formulas.HitRate(acc, eva, 0, false, true, false)
	nightRate := formulas.HitRate(acc, eva, 0, true, false, true)
	if frontRate >= behindRate {
		t.Fatalf("need positional rate gap, front=%d behind=%d", frontRate, behindRate)
	}
	if nightRate >= frontRate {
		t.Fatalf("need night rate gap, night=%d front=%d", nightRate, frontRate)
	}
	posRoll := (frontRate + behindRate) / 2
	nightRoll := (nightRate + frontRate) / 2

	tests := []struct {
		name     string
		ax, ay   int
		night    bool
		roll     int
		wantMiss bool
	}{
		{"front day misses between front and behind rates", 100, 0, false, posRoll, true},
		{"behind day hits between front and behind rates", -100, 0, false, posRoll, false},
		{"side day hits between front and behind rates", 0, 100, false, posRoll, false},
		{"front night misses between night and front rates", 100, 0, true, nightRoll, true},
		{"front day hits the night-gap roll", 100, 0, false, nightRoll, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacker, target := place(t, tt.ax, tt.ay, tt.night)
			attacker.roll = func(int) int { return tt.roll }
			hit := attacker.MakeAttackHit(target, false)
			if hit.Miss != tt.wantMiss {
				t.Fatalf("Miss = %v, want %v (roll %d acc %d eva %d)", hit.Miss, tt.wantMiss, tt.roll, acc, eva)
			}
		})
	}
}

func TestSummonMakeAttackHitUsesPosAndPvP(t *testing.T) {
	stats := CombatStats{DEX: 20, PAtk: 100, PDef: 50, MaxHP: 100}
	place := func(ax, ay int) (*Actor, *Actor) {
		t.Helper()
		target := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 1, Stats: stats})
		attacker := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 1, Stats: stats})
		state := world.New()
		state.Spawn(target, 0, 0, 0, 0)
		state.Spawn(attacker, ax, ay, 0, 0)
		n := 0
		attacker.roll = func(bound int) int {
			n++
			if n == 1 {
				return 0
			}
			if n == 2 {
				return 999
			}
			return (bound - 1) / 2
		}
		return attacker, target
	}

	frontAtk, frontTgt := place(100, 0)
	front := frontAtk.MakeAttackHit(frontTgt, false)
	if front.Miss || front.Crit {
		t.Fatalf("front miss=%v crit=%v, want hit non-crit", front.Miss, front.Crit)
	}
	wantFront := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: frontAtk.PAtk(), Defence: frontTgt.PDef(),
		PosMul: 1, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1,
	}))
	if front.Damage != wantFront {
		t.Fatalf("front damage = %d, want %d", front.Damage, wantFront)
	}

	behindAtk, behindTgt := place(-100, 0)
	behind := behindAtk.MakeAttackHit(behindTgt, false)
	wantBehind := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: behindAtk.PAtk(), Defence: behindTgt.PDef(),
		PosMul: 1.2, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1,
	}))
	if behind.Damage != wantBehind {
		t.Fatalf("behind damage = %d, want %d", behind.Damage, wantBehind)
	}

	pvpAtk, pvpTgt := place(100, 0)
	pvpAtk.AddStatFuncs([]effect.Mod{{Stat: stat.PvPPhysicalDmg, Op: effect.OpMul, Value: 2, Owner: effect.ModOwnerEffect(&effect.Effect{})}})
	pvp := pvpAtk.MakeAttackHit(pvpTgt, false)
	wantPvP := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: pvpAtk.PAtk(), Defence: pvpTgt.PDef(),
		PosMul: 1, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 2,
	}))
	if pvp.Damage != wantPvP {
		t.Fatalf("pvp damage = %d, want %d", pvp.Damage, wantPvP)
	}
}

func TestSummonMakeAttackHitUsesTemplateCritRate(t *testing.T) {
	// Auto-attack crit uses CreatureStatus.getCriticalHit's template base
	// (CreatureStatus.java:551-553), not the NPC XML default of 4.
	// Cursed Man servitor npc 14074 has <set name="crit" val="8.0"/>.
	// Summon AtkCritical is value*10 with no DEX mul, so rates 40 vs 80.
	const betweenDefaultAndCursedMan = 50
	place := func(critRate float64) (*Actor, *Actor) {
		t.Helper()
		target := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 1, Stats: CombatStats{DEX: 20, PDef: 50, MaxHP: 100}})
		attacker := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 1, Stats: CombatStats{DEX: 20, PDef: 50, MaxHP: 100, CritRate: critRate}})
		state := world.New()
		state.Spawn(target, 0, 0, 0, 0)
		state.Spawn(attacker, 100, 0, 0, 0)
		n := 0
		attacker.roll = func(int) int {
			n++
			if n == 1 {
				return 0
			}
			return betweenDefaultAndCursedMan
		}
		return attacker, target
	}

	defaultAtk, defaultTgt := place(4)
	if hit := defaultAtk.MakeAttackHit(defaultTgt, false); hit.Miss || hit.Crit {
		t.Fatalf("npc default crit=4: miss=%v crit=%v, want hit non-crit (roll %d)", hit.Miss, hit.Crit, betweenDefaultAndCursedMan)
	}

	cursedAtk, cursedTgt := place(8)
	if hit := cursedAtk.MakeAttackHit(cursedTgt, false); hit.Miss || !hit.Crit {
		t.Fatalf("Cursed Man crit=8: miss=%v crit=%v, want hit crit (roll %d)", hit.Miss, hit.Crit, betweenDefaultAndCursedMan)
	}
}

// ---- avoid-attack refusals ----
// avoidOwner is an owner whose position and attack stance a test sets.
type avoidOwner struct {
	fakeSummonOwner
	x, y, z  int
	inCombat bool
}

func (o *avoidOwner) Position() (int, int, int) { return o.x, o.y, o.z }
func (o *avoidOwner) InCombat() bool            { return o.inCombat }

// stepAsideAI records every StepAside destination it is asked to walk to.
type stepAsideAI struct {
	thinkCountingAI
	steps []location.Location
}

func (a *stepAsideAI) StepAside(dest location.Location) bool {
	a.steps = append(a.steps, dest)
	return true
}

const avoidRoll = 3

// newAvoidFixture places a servitor at summonAt beside an in-combat owner
// standing at (1000, 1000, 0), with a recording brain and sink and a fixed
// step-aside roll.
func newAvoidFixture(t *testing.T, summonAt location.Location) (*Actor, *avoidOwner, *stepAsideAI, *event.Recorder) {
	t.Helper()
	owner := &avoidOwner{fakeSummonOwner: fakeSummonOwner{id: 42}, x: 1000, y: 1000, z: 0, inCombat: true}
	a := mustServitor(t, ServitorConfig{ObjectID: 7, Owner: owner})
	if err := a.InitMovement(summonAt, 100, openGeo{}); err != nil {
		t.Fatalf("InitMovement: %v", err)
	}
	SpawnBesideOwner(world.New(), a, owner, location.Location{X: summonAt.X - owner.x, Y: summonAt.Y - owner.y, Z: summonAt.Z - owner.z})
	if x, y, z := a.Position(); (location.Location{X: x, Y: y, Z: z}) != summonAt {
		t.Fatalf("summon at (%d, %d, %d), want %v", x, y, z, summonAt)
	}
	brain := &stepAsideAI{}
	rec := &event.Recorder{}
	a.Attach(Runtime{AI: brain, Sink: rec})
	a.SetRollSource(func(int) int { return avoidRoll })
	return a, owner, brain, rec
}

// TestSummonAvoidAttackRefusals pins each gate of SummonMove.avoidAttack
// that keeps the summon where it is: the owner itself attacking, the owner
// at 140 or more in 3D, the owner out of stance, and a summon that is
// already moving or cannot move.
func TestSummonAvoidAttackRefusals(t *testing.T) {
	stranger := &fakeSummonOwner{id: 99}
	wantStep := location.EquidistantPoint(1000, 1000, 0, avoidRadius, avoidPoints, avoidRoll)

	tests := []struct {
		name     string
		summonAt location.Location
		setup    func(t *testing.T, a *Actor, owner *avoidOwner)
		wantStep bool
	}{
		{name: "stranger hit beside a fighting owner steps aside", summonAt: location.Location{X: 1100, Y: 1000}, wantStep: true},
		{name: "just inside 140 in 3D steps aside", summonAt: location.Location{X: 1139, Y: 1000}, wantStep: true},
		{name: "owner at exactly 140 stays", summonAt: location.Location{X: 1140, Y: 1000}},
		{name: "owner within 140 in 2D but not in 3D stays", summonAt: location.Location{X: 1100, Y: 1000, Z: 100}},
		{
			name:     "owner out of attack stance stays",
			summonAt: location.Location{X: 1100, Y: 1000},
			setup:    func(_ *testing.T, _ *Actor, owner *avoidOwner) { owner.inCombat = false },
		},
		{
			name:     "moving summon stays",
			summonAt: location.Location{X: 1100, Y: 1000},
			setup: func(t *testing.T, a *Actor, _ *avoidOwner) {
				if _, err := a.Move().MoveToLocation(location.Location{X: 1500, Y: 1000}); err != nil {
					t.Fatalf("MoveToLocation: %v", err)
				}
				if !a.IsMoving() {
					t.Fatal("IsMoving() = false after an accepted walk, want true")
				}
			},
		},
		{
			name:     "dead summon stays",
			summonAt: location.Location{X: 1100, Y: 1000},
			setup: func(_ *testing.T, a *Actor, _ *avoidOwner) {
				a.vitals.mu.Lock()
				a.dead = true
				a.vitals.mu.Unlock()
			},
		},
		{
			name:     "immobilized summon stays",
			summonAt: location.Location{X: 1100, Y: 1000},
			setup: func(t *testing.T, a *Actor, _ *avoidOwner) {
				a.SetImmobilized(true)
				if !a.MovementDisabled() {
					t.Fatal("MovementDisabled() = false while immobilized, want true")
				}
			},
		},
	}

	for _, tc := range tests {
		for _, notify := range []string{"attacked", "evaded"} {
			t.Run(tc.name+"/"+notify, func(t *testing.T) {
				a, owner, brain, _ := newAvoidFixture(t, tc.summonAt)
				if tc.setup != nil {
					tc.setup(t, a, owner)
				}
				if notify == "attacked" {
					a.NotifyAttacked(stranger)
				} else {
					a.NotifyEvaded(stranger)
				}
				if !tc.wantStep {
					if len(brain.steps) != 0 {
						t.Fatalf("StepAside calls = %v, want none", brain.steps)
					}
					return
				}
				if want := []location.Location{wantStep}; !reflect.DeepEqual(brain.steps, want) {
					t.Fatalf("StepAside calls = %v, want %v", brain.steps, want)
				}
			})
		}
	}
}

// TestSummonAvoidAttackIgnoresOwnHitButOwnerStillEntersStance covers an
// owner that ctrl-attacks its own summon: SummonMove.avoidAttack refuses to
// step aside, while ATTACKED still puts the owner in attack stance.
func TestSummonAvoidAttackIgnoresOwnHitButOwnerStillEntersStance(t *testing.T) {
	a, owner, brain, rec := newAvoidFixture(t, location.Location{X: 1100, Y: 1000})

	a.NotifyAttacked(owner)
	if len(brain.steps) != 0 {
		t.Fatalf("StepAside calls after the owner's hit = %v, want none", brain.steps)
	}
	if got := event.Of[event.Attacked](rec); len(got) != 1 || got[0].Attacker != owner {
		t.Fatalf("Attacked events = %+v, want one naming the owner", got)
	}

	a.NotifyEvaded(owner)
	if len(brain.steps) != 0 {
		t.Fatalf("StepAside calls after the owner's miss = %v, want none", brain.steps)
	}
	if got := event.Count[event.Attacked](rec); got != 1 {
		t.Fatalf("Attacked events after the owner's miss = %d, want still 1", got)
	}
}
