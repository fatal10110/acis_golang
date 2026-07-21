package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// gateTarget is a minimal attackable.Combatant double whose gate-relevant
// state (dead, silently moving, standing in a peace zone, karma) is set
// directly by each test case, rather than derived from a live effect list,
// zone, or player record.
type gateTarget struct {
	world.Presence
	id     int32
	dead   bool
	silent bool
	peace  bool
	karma  int
}

func (t *gateTarget) ObjectID() int32    { return t.id }
func (t *gateTarget) SiegeGuard() bool   { return false }
func (t *gateTarget) AlikeDead() bool    { return t.dead }
func (t *gateTarget) SilentMoving() bool { return t.silent }
func (t *gateTarget) InPeaceZone() bool  { return t.peace }
func (t *gateTarget) Karma() int         { return t.karma }

func newKindHostile(t testing.TB, id int32, tpl *Template, kind InstanceKind) *Hostile {
	t.Helper()
	h, err := NewHostile(&Instance{ObjectID: id, Template: tpl, Kind: kind}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

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

func TestHostileAutoAttackTargetValidGuardAndFriendlyMonster(t *testing.T) {
	tests := []struct {
		name  string
		kind  InstanceKind
		karma int
		want  bool
	}{
		{"guard attacks karma-positive target", "Guard", 1, true},
		{"guard ignores non-karma target", "Guard", 0, false},
		{"friendly monster attacks karma-positive target", "FriendlyMonster", 1, true},
		{"friendly monster ignores non-karma target", "FriendlyMonster", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := world.New()
			attacker := newKindHostile(t, 1, &Template{ID: 1, Type: string(tc.kind)}, tc.kind)
			state.Spawn(attacker, 100, 100, 0, 0)

			target := &gateTarget{id: 2, karma: tc.karma}
			state.Spawn(target, 100, 100, 0, 0)

			if got := attacker.AutoAttackTargetValid(target, 500, false); got != tc.want {
				t.Fatalf("AutoAttackTargetValid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHostileAutoAttackTargetValidGuardIgnoresAggressiveMonster(t *testing.T) {
	// GuardAttackAggroMob ships disabled by default in the reference
	// config; a Guard must not attack a nearby aggressive monster until
	// that toggle is wired.
	state := world.New()
	guard := newKindHostile(t, 1, &Template{ID: 1, Type: "Guard"}, "Guard")
	state.Spawn(guard, 100, 100, 0, 0)

	monster := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster", AggroRange: 400})
	state.Spawn(monster, 100, 100, 0, 0)

	if guard.AutoAttackTargetValid(monster, 500, true) {
		t.Fatal("AutoAttackTargetValid(aggressive monster) = true, want false")
	}
}

func TestHostileAutoAttackTargetValidConfusedActorTargetsAnyNPC(t *testing.T) {
	state := world.New()
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster"})
	state.Spawn(attacker, 100, 100, 0, 0)

	other := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster"})
	state.Spawn(other, 100, 100, 0, 0)

	if attacker.AutoAttackTargetValid(other, 500, false) {
		t.Fatal("AutoAttackTargetValid(other NPC) = true before confusion, want false")
	}

	addHostileEffect(t, attacker, "Confusion")

	if !attacker.AutoAttackTargetValid(other, 500, false) {
		t.Fatal("AutoAttackTargetValid(other NPC) = false while confused, want true")
	}
}

func TestHostileAutoAttackTargetValidRaidRelatedSeesThroughSilentMove(t *testing.T) {
	state := world.New()
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	state.Spawn(attacker, 100, 100, 0, 0)

	target := &gateTarget{id: 2, silent: true}
	state.Spawn(target, 100, 100, 0, 0)

	if attacker.AutoAttackTargetValid(target, 500, false) {
		t.Fatal("AutoAttackTargetValid(silent target) = true before RaidRelated, want false")
	}

	attacker.SetRaidRelated(true)

	if !attacker.AutoAttackTargetValid(target, 500, false) {
		t.Fatal("AutoAttackTargetValid(silent target) = false once RaidRelated, want true")
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
