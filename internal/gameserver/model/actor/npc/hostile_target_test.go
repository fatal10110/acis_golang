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
	id              int32
	dead            bool
	silent          bool
	peace           bool
	karma           int
	recentFakeDeath bool
}

func (t *gateTarget) ObjectID() int32       { return t.id }
func (t *gateTarget) SiegeGuard() bool      { return false }
func (t *gateTarget) AlikeDead() bool       { return t.dead }
func (t *gateTarget) SilentMoving() bool    { return t.silent }
func (t *gateTarget) InPeaceZone() bool     { return t.peace }
func (t *gateTarget) Karma() int            { return t.karma }
func (t *gateTarget) RecentFakeDeath() bool { return t.recentFakeDeath }

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
		{
			name:       "a target within its post-fake-death grace period is excluded",
			aggroRange: 10,
			target:     func() *gateTarget { return &gateTarget{id: 2, recentFakeDeath: true} },
			targetPos:  [3]int{100, 100, 0},
			want:       false,
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

func TestHostileRandomizeHateDisplacesTargetGatedByAutoAttackTargetValid(t *testing.T) {
	state := world.New()
	owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	owner.SetRollSource(zeroRoll)
	state.Spawn(owner, 100, 100, 0, 0)

	low := &gateTarget{id: 2}
	high := &gateTarget{id: 3}
	// outOfRange sits well past partyRangeDefault and holds the highest
	// hate, so it becomes mostHated (selection is unfiltered by valid, per
	// AggroList.randomizeAttack) but AutoAttackTargetValid still excludes
	// it from candidacy, so it can never be the chosen displacer.
	outOfRange := &gateTarget{id: 4}
	state.Spawn(low, 100, 100, 0, 0)
	state.Spawn(high, 100, 100, 0, 0)
	state.Spawn(outOfRange, 100+partyRangeDefault+1000, 100, 0, 0)

	owner.AddDamageHate(low, 0, 10)
	owner.AddDamageHate(high, 0, 25)
	owner.AddDamageHate(outOfRange, 0, 999)

	if ok := owner.RandomizeHate(); !ok {
		t.Fatal("RandomizeHate: ok = false, want true")
	}

	// Candidates sort by attacker id: low(2) before high(3), so pick=0
	// (zeroRoll) selects low. new hate = 10 + (999 - 10) + 200 = 1199.
	if got := owner.AI().Threats().Hate(low); got != 1199 {
		t.Fatalf("displaced attacker hate = %v, want 1199", got)
	}
	if got := owner.AI().Threats().Hate(high); got != 25 {
		t.Fatalf("non-candidate hate = %v, want unchanged 25", got)
	}
	if got := owner.AI().Threats().Hate(outOfRange); got != 999 {
		t.Fatalf("mostHated hate = %v, want unchanged 999", got)
	}
}

func TestHostileRandomizeHateNoopWithSingleAttacker(t *testing.T) {
	state := world.New()
	owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	state.Spawn(owner, 100, 100, 0, 0)

	target := &gateTarget{id: 2}
	state.Spawn(target, 100, 100, 0, 0)
	owner.AddDamageHate(target, 0, 10)

	if owner.RandomizeHate() {
		t.Fatal("RandomizeHate: ok = true, want false with a single attacker")
	}
}

func TestHostileReconsiderTargetSwapsFromHateList(t *testing.T) {
	state := world.New()
	owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	state.Spawn(owner, 100, 100, 0, 0)

	low := &gateTarget{id: 2}
	high := &gateTarget{id: 3}
	state.Spawn(low, 100, 100, 0, 0)
	state.Spawn(high, 100, 100, 0, 0)
	owner.AddDamageHate(low, 0, 10)
	owner.AddDamageHate(high, 0, 25)

	chosen, ok := owner.ReconsiderTarget(0)
	if !ok {
		t.Fatal("ReconsiderTarget: ok = false, want true")
	}
	if chosen.ObjectID() != low.ObjectID() {
		t.Fatalf("chosen = %d, want %d (lowest ObjectID candidate)", chosen.ObjectID(), low.ObjectID())
	}
	if got := owner.AI().Threats().Hate(low); got != 10 {
		t.Fatalf("chosen hate = %v, want unchanged 10", got)
	}
	if got := owner.AI().Threats().Hate(high); got != 0 {
		t.Fatalf("previous mostHated hate = %v, want zeroed 0", got)
	}
}

func TestHostileReconsiderTargetGatedByAutoAttackTargetValid(t *testing.T) {
	// low is silently moving and this NPC neither sees through concealment
	// nor is raid-related, so AutoAttackTargetValid excludes it; only high
	// remains, but it's mostHated, so no other candidate qualifies.
	state := world.New()
	owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	state.Spawn(owner, 100, 100, 0, 0)

	low := &gateTarget{id: 2, silent: true}
	high := &gateTarget{id: 3}
	state.Spawn(low, 100, 100, 0, 0)
	state.Spawn(high, 100, 100, 0, 0)
	owner.AddDamageHate(low, 0, 10)
	owner.AddDamageHate(high, 0, 25)

	if _, ok := owner.ReconsiderTarget(0); ok {
		t.Fatal("ReconsiderTarget: ok = true, want false when the only candidate fails AutoAttackTargetValid")
	}
}

func TestHostileReconsiderTargetRangeFilterExcludesOutOfRangeCandidate(t *testing.T) {
	const rangeVal = 500

	state := world.New()
	owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	state.Spawn(owner, 100, 100, 0, 0)

	far := &gateTarget{id: 2}
	near := &gateTarget{id: 3}
	state.Spawn(far, 100+rangeVal+1000, 100, 0, 0)
	state.Spawn(near, 100, 100, 0, 0)
	owner.AddDamageHate(far, 0, 10)
	owner.AddDamageHate(near, 0, 25)

	// near is mostHated (higher hate), leaving far as the sole candidate;
	// the rangeVal filter (independent of AutoAttackTargetValid's own
	// aggro-range check) excludes it for being out of range.
	if _, ok := owner.ReconsiderTarget(rangeVal); ok {
		t.Fatal("ReconsiderTarget: ok = true, want false when the only candidate fails the range filter")
	}
}

func TestHostileReconsiderTargetZeroRangeDisablesDistanceFilter(t *testing.T) {
	state := world.New()
	owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10000})
	state.Spawn(owner, 100, 100, 0, 0)

	far := &gateTarget{id: 2}
	near := &gateTarget{id: 3}
	state.Spawn(far, 100+2000, 100, 0, 0)
	state.Spawn(near, 100, 100, 0, 0)
	owner.AddDamageHate(far, 0, 10)
	owner.AddDamageHate(near, 0, 25)

	if _, ok := owner.ReconsiderTarget(0); !ok {
		t.Fatal("ReconsiderTarget: ok = false, want true with the distance filter disabled (rangeVal = 0)")
	}
}

func TestHostileReconsiderTargetFallsBackToKnownlistWhenHateListEmpty(t *testing.T) {
	state := world.New()
	owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 400})
	owner.SetWorld(state)
	state.Spawn(owner, 100, 100, 0, 0)

	bystander := &gateTarget{id: 2}
	state.Spawn(bystander, 100, 100, 0, 0)

	chosen, ok := owner.ReconsiderTarget(0)
	if !ok {
		t.Fatal("ReconsiderTarget: ok = false, want true from the knownlist fallback")
	}
	if chosen.ObjectID() != bystander.ObjectID() {
		t.Fatalf("chosen = %d, want %d", chosen.ObjectID(), bystander.ObjectID())
	}
	if got := owner.AI().Threats().Hate(bystander); got != 1 {
		t.Fatalf("fallback candidate hate = %v, want 1 (simulated aggro-range entrance)", got)
	}
}

func TestHostileReconsiderTargetKnownlistFallbackNoopForSiegeGuardOrNonAggressive(t *testing.T) {
	t.Run("SiegeGuard never uses the knownlist fallback", func(t *testing.T) {
		state := world.New()
		guard := newKindHostile(t, 1, &Template{ID: 1, Type: "SiegeGuard", AggroRange: 400}, "SiegeGuard")
		guard.SetWorld(state)
		state.Spawn(guard, 100, 100, 0, 0)

		bystander := &gateTarget{id: 2}
		state.Spawn(bystander, 100, 100, 0, 0)

		if _, ok := guard.ReconsiderTarget(0); ok {
			t.Fatal("ReconsiderTarget: ok = true, want false for a SiegeGuard with no hate-list candidate")
		}
	})

	t.Run("non-aggressive owner never uses the knownlist fallback", func(t *testing.T) {
		state := world.New()
		owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 0})
		owner.SetWorld(state)
		state.Spawn(owner, 100, 100, 0, 0)

		bystander := &gateTarget{id: 2}
		state.Spawn(bystander, 100, 100, 0, 0)

		if _, ok := owner.ReconsiderTarget(0); ok {
			t.Fatal("ReconsiderTarget: ok = true, want false for a non-aggressive NPC (zero aggro range)")
		}
	})
}
