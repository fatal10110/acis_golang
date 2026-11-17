package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The fixture monster faces east (heading 0) and the player stands at
// (10, 20): a monster to the east of the player (x=30) is struck from
// behind, one to the west (x=-10) from the front, and one directly north of
// the player (y=40) from the side.
var (
	posBehind = location.Location{X: 30, Y: playerOrigin.Y, Z: hostileZ}
	posFront  = location.Location{X: -10, Y: playerOrigin.Y, Z: hostileZ}
	posSide   = location.Location{X: playerOrigin.X, Y: 40, Z: hostileZ}
)

// positionalSwing boots a level-5 player, optionally lays def's effects on
// it, and returns the first auto-attack hit against a monster at monster.
// The hit roll lands; critRoll maps the player's unbuffed critical rate to
// the critical roll, so a swing crits exactly when the position-gated rate
// rises above (or stays above) that roll.
func positionalSwing(t *testing.T, monster location.Location, def *modelskill.Definition, critRoll func(unbuffed int) int) swingHit {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, monster)
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	pc := obj.(interface {
		effectHolder
		CriticalRate() float64
		SetRollSource(func(int) int)
	})
	onQueue(t, srv.PlayerQueue(t, objID), func() {
		unbuffed := int(pc.CriticalRate())
		if def != nil {
			effect.Apply(pc, pc, effect.SkillFromDefinition(*def), def.Effects)
		}
		calls := 0
		pc.SetRollSource(func(int) int {
			calls++
			if calls == 2 {
				return critRoll(unbuffed)
			}
			return 0
		})
	})
	drainUntilQuiet(t, c)

	c.Send(encodeAction(hostile.ObjectID(), int32(monster.X), int32(monster.Y), int32(monster.Z), false))
	c.Send(encodeAction(hostile.ObjectID(), int32(monster.X), int32(monster.Y), int32(monster.Z), false))
	hit, _ := firstHitFeedback(t, c, objID)
	if hit.Flags&attack.HitMiss != 0 {
		t.Fatalf("Attack flags = %#x, want a landed hit", hit.Flags)
	}
	return hit
}

// TestPositionalCritRateBuffsTestAttackPosition pins the shipped
// position-gated critical rate buffs against a real target: Focus Chance
// (356) lowers the rate 30% from the front and raises it 30% from the side
// and 60% from behind; Eye of Assassin (4522) doubles it from behind only. The critical roll sits just below or exactly at the unbuffed rate,
// so a swing crits exactly when the buff's term for that position raises the
// rate above the roll, and the unbuffed control crits only in the first case.
func TestPositionalCritRateBuffsTestAttackPosition(t *testing.T) {
	t.Parallel()
	skills := shippedSkills(t)
	focusChance, ok := skills.Get(356, 1)
	if !ok {
		t.Fatal("shipped skill 356-1 missing")
	}
	eyeOfAssassin, ok := skills.Get(4522, 1)
	if !ok {
		t.Fatal("shipped skill 4522-1 missing")
	}
	below := func(u int) int { return u - 1 }
	at := func(u int) int { return u }
	for _, tc := range []struct {
		name     string
		def      *modelskill.Definition
		monster  location.Location
		roll     func(int) int
		wantCrit bool
	}{
		{"control, front, roll below the rate", nil, posFront, below, true},
		{"control, side, roll at the rate", nil, posSide, at, false},
		{"control, behind, roll at the rate", nil, posBehind, at, false},
		{"356 from the front lowers the rate", focusChance, posFront, below, false},
		{"356 from the side raises the rate", focusChance, posSide, at, true},
		{"356 from behind raises the rate", focusChance, posBehind, at, true},
		{"4522 from the front leaves the rate", eyeOfAssassin, posFront, at, false},
		{"4522 from the side leaves the rate", eyeOfAssassin, posSide, at, false},
		{"4522 from behind doubles the rate", eyeOfAssassin, posBehind, func(u int) int { return 2*u - 1 }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hit := positionalSwing(t, tc.monster, tc.def, tc.roll)
			if got := hit.Flags&attack.HitCritical != 0; got != tc.wantCrit {
				t.Fatalf("critical = %v (flags %#x), want %v", got, hit.Flags, tc.wantCrit)
			}
		})
	}
}

// TestPositionalCritDamageMultiplierTestsAttackPosition pins the shipped
// Focus Death (355, cAtkPos 0.7 from the front and 1.9 from behind) and Focus
// Power (357, cAtkPos 0.7 from the front, 1.3 from the side, 1.6 from
// behind) critical damage position multipliers in the physical damage
// formula against a real target: with every roll landing a critical, the
// buffed damage is the unbuffed damage at the same position times the
// multiplier, within the integer truncation of both swings.
func TestPositionalCritDamageMultiplierTestsAttackPosition(t *testing.T) {
	t.Parallel()
	skills := shippedSkills(t)
	focusPower, ok := skills.Get(357, 1)
	if !ok {
		t.Fatal("shipped skill 357-1 missing")
	}
	focusDeath, ok := skills.Get(355, 1)
	if !ok {
		t.Fatal("shipped skill 355-1 missing")
	}
	always := func(int) int { return 0 }
	for _, tc := range []struct {
		name    string
		def     *modelskill.Definition
		monster location.Location
		mul     float64
	}{
		{"357 from the front", focusPower, posFront, 0.7},
		{"357 from the side", focusPower, posSide, 1.3},
		{"357 from behind", focusPower, posBehind, 1.6},
		{"355 from the front", focusDeath, posFront, 0.7},
		{"355 from behind", focusDeath, posBehind, 1.9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var base, buffed swingHit
			t.Run("unbuffed", func(t *testing.T) { base = positionalSwing(t, tc.monster, nil, always) })
			t.Run("buffed", func(t *testing.T) { buffed = positionalSwing(t, tc.monster, tc.def, always) })
			if base.Flags&attack.HitCritical == 0 || buffed.Flags&attack.HitCritical == 0 {
				t.Fatalf("flags base %#x buffed %#x, want two criticals", base.Flags, buffed.Flags)
			}
			// buffed = trunc(X*mul) and base = trunc(X) for the same real X.
			lo := (float64(base.Damage) * tc.mul) - 1
			hi := (float64(base.Damage)+1)*tc.mul + 1
			if float64(buffed.Damage) < lo || float64(buffed.Damage) > hi {
				t.Fatalf("buffed damage %d outside [%.1f, %.1f] for base %d x %.1f", buffed.Damage, lo, hi, base.Damage, tc.mul)
			}
			if base.Damage < 20 {
				t.Fatalf("base damage %d too small to tell x%.1f from no buff", base.Damage, tc.mul)
			}
		})
	}
}
