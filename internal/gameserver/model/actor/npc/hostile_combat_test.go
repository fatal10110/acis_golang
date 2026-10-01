package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type overhitActor struct {
	attackabletest.Combatant
	id int32
}

func (a overhitActor) ObjectID() int32 { return a.id }

type overhitSummon struct {
	attackabletest.Combatant
	id    int32
	owner attackable.Combatant
}

func (s overhitSummon) ObjectID() int32  { return s.id }
func (s overhitSummon) Kind() actor.Kind { return actor.KindSummon }
func (s overhitSummon) Owner() (attackable.Combatant, bool) {
	return s.owner, s.owner != nil
}

func TestOverhitBonusExpOracle(t *testing.T) {
	attacker := overhitActor{id: 1}
	other := overhitActor{id: 2}

	t.Run("no overhit", func(t *testing.T) {
		var s overhitState
		if s.valid(attacker) {
			t.Fatal("valid() = true with no overhit, want false")
		}
	})

	t.Run("valid overhit", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 110)
		if !s.valid(attacker) {
			t.Fatal("valid(attacker) = false, want true")
		}
		// excess 10 / maxHP 200 = 5% of 1000 = 50
		if got := s.bonusExp(1000, 200); got != 50 {
			t.Fatalf("bonusExp() = %d, want 50", got)
		}
	})

	t.Run("attacker mismatch", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 110)
		if s.valid(other) {
			t.Fatal("valid(other) = true, want false")
		}
	})

	t.Run("25 percent cap", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 200)
		// excess 100 / maxHP 200 = 50% capped at 25% of 1000 = 250
		if got := s.bonusExp(1000, 200); got != 250 {
			t.Fatalf("bonusExp() = %d, want 250", got)
		}
	})

	t.Run("half-unit rounds up", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 10, 11)
		// excess 1 / maxHP 200 = 0.5% of 100 = 0.5 → 1
		if got := s.bonusExp(100, 200); got != 1 {
			t.Fatalf("bonusExp() = %d, want 1", got)
		}
	})

	t.Run("non-lethal clears", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 40)
		if s.valid(attacker) {
			t.Fatal("valid() = true after a non-lethal hit, want false")
		}
	})

	t.Run("zero damage clears", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(attacker, 100, 0)
		if s.valid(attacker) {
			t.Fatal("valid() = true after zero damage, want false")
		}
	})

	t.Run("summon acting player", func(t *testing.T) {
		var s overhitState
		s.set(true)
		s.test(overhitSummon{id: 9, owner: attacker}, 100, 110)
		if !s.valid(attacker) {
			t.Fatal("valid(owner) = false for a summon overhit, want true")
		}
		if s.valid(other) {
			t.Fatal("valid(other) = true for a summon overhit, want false")
		}
	})
}

type hitNight bool

func (n hitNight) IsNight() bool { return bool(n) }

func TestMakeAttackHitAppliesFacingAndNight(t *testing.T) {
	tpl := &Template{ID: 1, Type: "Monster"}
	place := func(t *testing.T, ax, ay int, night bool) (*Hostile, *Hostile) {
		t.Helper()
		state := world.New()
		target := newCombatHostile(t, 1, tpl)
		live := newHostileLive(t, effect.WithEnv(effect.Env{Night: hitNight(night)}))
		attacker, err := NewHostile(&Instance{ObjectID: 2, Template: tpl, Kind: "Monster"}, live, &hostileMove{}, &hostileAttack{})
		if err != nil {
			t.Fatal(err)
		}
		state.Spawn(target, 0, 0, 0, 0)
		state.Spawn(attacker, ax, ay, 0, 0)
		return attacker, target
	}

	attacker, target := place(t, 100, 0, false)
	acc := int(attacker.calcStat(stat.AccuracyCombat, 0))
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
			attacker.SetRollSource(func(int) int { return tt.roll })
			hit := attacker.MakeAttackHit(target, false)
			if hit.Miss != tt.wantMiss {
				t.Fatalf("Miss = %v, want %v (roll %d acc %d eva %d)", hit.Miss, tt.wantMiss, tt.roll, acc, eva)
			}
		})
	}
}

func TestMakeAttackHitAppliesRacePosAndIgnoresPvP(t *testing.T) {
	tpl := &Template{ID: 1, Type: "Monster", Race: RaceBeast, PAtk: 100, PDef: 50, CritRate: 0, DEX: 30}
	state := world.New()
	target := newCombatHostile(t, 1, tpl)
	attacker := newCombatHostile(t, 2, tpl)
	state.Spawn(target, 0, 0, 0, 0)
	state.Spawn(attacker, -100, 0, 0, 0)
	attacker.SetRollSource(func(bound int) int {
		if bound == 1000 {
			return 0
		}
		return (bound - 1) / 2
	})
	attacker.AddStatFuncs([]effect.Mod{
		{Stat: stat.PAtkBeasts, Op: effect.OpSet, Value: 50, Owner: effect.ModOwnerEffect(&effect.Effect{})},
		{Stat: stat.PvPPhysicalDmg, Op: effect.OpMul, Value: 3, Owner: effect.ModOwnerEffect(&effect.Effect{})},
	})

	hit := attacker.MakeAttackHit(target, false)
	if hit.Miss || hit.Crit {
		t.Fatalf("hit miss=%v crit=%v, want connected non-crit", hit.Miss, hit.Crit)
	}
	want := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: attacker.PAtk(), Defence: target.PDef(),
		PosMul: 1.2, ElementalMul: 1, RandomMul: 1, RaceMul: target.RaceMultiplier(attacker),
		WeaponVulnMul: 1, PvPMul: 1,
	}))
	if hit.Damage != want {
		t.Fatalf("damage = %d, want %d (race %v)", hit.Damage, want, target.RaceMultiplier(attacker))
	}
	if target.RaceMultiplier(attacker) != 1.49 {
		t.Fatalf("RaceMultiplier = %v, want 1.49", target.RaceMultiplier(attacker))
	}
}

func (overhitActor) Heading() int { return 0 }

func (overhitActor) Position() (x, y, z int) { return 0, 0, 0 }

func (overhitSummon) Heading() int { return 0 }

func (overhitSummon) Position() (x, y, z int) { return 0, 0, 0 }

// A confused NPC picks its new target among neighbours within a horizontal
// point distance of 1000: a monster on a ledge far above but 900 across is a
// candidate, one 1001 across on the same floor is not, even though its body
// would reach under a collision-widened 3D check.
func TestRandomNearbyCombatantMeasuresHorizontalPointDistance(t *testing.T) {
	tpl := &Template{ID: 9001, Type: "Monster", CollisionRadius: 40}
	tests := []struct {
		name    string
		x, y, z int
		wantHit bool
	}{
		{"900 across, 700 above", 900, 0, 700, true},
		{"diagonal 1000 across, 3000 below", 600, 800, -3000, true},
		{"1001 across, same floor", 1001, 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := world.New()
			confused := newCombatHostile(t, 1, tpl)
			other := newCombatHostile(t, 2, tpl)
			w.Spawn(confused, 0, 0, 0, 0)
			w.Spawn(other, tc.x, tc.y, tc.z, 0)
			confused.Attach(Runtime{World: w})

			got, ok := confused.RandomNearbyCombatant(1000)
			if ok != tc.wantHit {
				t.Fatalf("RandomNearbyCombatant(1000) ok = %v, want %v", ok, tc.wantHit)
			}
			if ok && got.ObjectID() != other.ObjectID() {
				t.Fatalf("RandomNearbyCombatant(1000) = object %d, want %d", got.ObjectID(), other.ObjectID())
			}
		})
	}
}
