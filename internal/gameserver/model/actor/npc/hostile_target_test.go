package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// gateTarget is a minimal attackable.Combatant double whose gate-relevant
// state (dead, silently moving, standing in a peace zone) is set directly
// by each test case, rather than derived from a live effect list or zone.
type gateTarget struct {
	world.Presence
	id     int32
	dead   bool
	silent bool
	peace  bool
}

func (t *gateTarget) ObjectID() int32    { return t.id }
func (t *gateTarget) SiegeGuard() bool   { return false }
func (t *gateTarget) AlikeDead() bool    { return t.dead }
func (t *gateTarget) SilentMoving() bool { return t.silent }
func (t *gateTarget) InPeaceZone() bool  { return t.peace }

func TestHostileAutoAttackTargetValid(t *testing.T) {
	const rangeVal = 500

	tests := []struct {
		name          string
		aggroRange    int
		canSeeThrough bool
		allowPeaceful bool
		target        func() *gateTarget
		targetPos     [3]int
		want          bool
	}{
		{
			name:       "in-range aggressive npc attacks a plain target",
			aggroRange: 10,
			target:     func() *gateTarget { return &gateTarget{id: 2} },
			targetPos:  [3]int{100, 100, 0},
			want:       true,
		},
		{
			name:       "out-of-range target is excluded",
			aggroRange: 10,
			target:     func() *gateTarget { return &gateTarget{id: 2} },
			targetPos:  [3]int{100 + rangeVal + 1000, 100, 0},
			want:       false,
		},
		{
			name:       "already-dead target is excluded",
			aggroRange: 10,
			target:     func() *gateTarget { return &gateTarget{id: 2, dead: true} },
			targetPos:  [3]int{100, 100, 0},
			want:       false,
		},
		{
			name:       "silently moving target is excluded by default",
			aggroRange: 10,
			target:     func() *gateTarget { return &gateTarget{id: 2, silent: true} },
			targetPos:  [3]int{100, 100, 0},
			want:       false,
		},
		{
			name:          "silently moving target is included when the template sees through it",
			aggroRange:    10,
			canSeeThrough: true,
			target:        func() *gateTarget { return &gateTarget{id: 2, silent: true} },
			targetPos:     [3]int{100, 100, 0},
			want:          true,
		},
		{
			name:       "peace-zone target is excluded by default",
			aggroRange: 10,
			target:     func() *gateTarget { return &gateTarget{id: 2, peace: true} },
			targetPos:  [3]int{100, 100, 0},
			want:       false,
		},
		{
			name:          "peace-zone target is included when allowPeaceful is set",
			aggroRange:    10,
			allowPeaceful: true,
			target:        func() *gateTarget { return &gateTarget{id: 2, peace: true} },
			targetPos:     [3]int{100, 100, 0},
			want:          true,
		},
		{
			name:       "a non-aggressive npc excludes a plain target",
			aggroRange: 0,
			target:     func() *gateTarget { return &gateTarget{id: 2} },
			targetPos:  [3]int{100, 100, 0},
			want:       false,
		},
		{
			name:          "a non-aggressive npc still accepts a target when allowPeaceful is set",
			aggroRange:    0,
			allowPeaceful: true,
			target:        func() *gateTarget { return &gateTarget{id: 2} },
			targetPos:     [3]int{100, 100, 0},
			want:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := world.New()
			attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: tc.aggroRange, CanSeeThrough: tc.canSeeThrough})
			state.Spawn(attacker, 100, 100, 0, 0)

			target := tc.target()
			state.Spawn(target, tc.targetPos[0], tc.targetPos[1], tc.targetPos[2], 0)

			if got := attacker.AutoAttackTargetValid(target, rangeVal, tc.allowPeaceful); got != tc.want {
				t.Fatalf("AutoAttackTargetValid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHostileAutoAttackTargetValidExcludesNilAndOtherNPCs(t *testing.T) {
	state := world.New()
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	state.Spawn(attacker, 100, 100, 0, 0)

	var nilTarget attackable.Combatant
	if attacker.AutoAttackTargetValid(nilTarget, 500, true) {
		t.Fatal("AutoAttackTargetValid(nil) = true, want false")
	}

	otherNPC := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster"})
	state.Spawn(otherNPC, 100, 100, 0, 0)
	if attacker.AutoAttackTargetValid(otherNPC, 500, true) {
		t.Fatal("AutoAttackTargetValid(other NPC) = true, want false")
	}
}

func TestHostileAggressive(t *testing.T) {
	if newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 0}).Aggressive() {
		t.Fatal("Aggressive() = true for a zero aggro range, want false")
	}
	if !newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 400}).Aggressive() {
		t.Fatal("Aggressive() = false for a positive aggro range, want true")
	}
}
