package npc

import (
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
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

type ownedGateTarget struct {
	*gateTarget
	owner attackable.Combatant
}

func (t *ownedGateTarget) OwnerCombatant() attackable.Combatant { return t.owner }

// siegeGateTarget is a minimal player-alike attackable.Combatant double for
// SiegeGuard.canAutoAttack (SiegeGuard.java:82-96): dead, silent, and
// attackableBy stand in for the acting player's alike-dead state, silent
// movement, and target.isAttackableBy(this).
type siegeGateTarget struct {
	world.Presence
	id           int32
	dead         bool
	silent       bool
	attackableBy bool
}

func (t *siegeGateTarget) ObjectID() int32                        { return t.id }
func (t *siegeGateTarget) SiegeGuard() bool                       { return false }
func (t *siegeGateTarget) AlikeDead() bool                        { return t.dead }
func (t *siegeGateTarget) SilentMoving() bool                     { return t.silent }
func (t *siegeGateTarget) AttackableBy(skilltarget.Creature) bool { return t.attackableBy }

// ownedSiegeGateTarget is a Summon/Pet-alike siegeGateTarget: its own
// AlikeDead/SilentMoving reflect its own state, while OwnerCombatant
// resolves to the owning player whose state SiegeGuard.canAutoAttack
// actually gates on (SiegeGuard.java:84-93 reads target.getActingPlayer()
// once, then checks every gate against that resolved player).
type ownedSiegeGateTarget struct {
	*siegeGateTarget
	owner attackable.Combatant
}

func (t *ownedSiegeGateTarget) OwnerCombatant() attackable.Combatant { return t.owner }

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
			name:       "target exactly at range is excluded",
			aggroRange: 10,
			target:     func() *gateTarget { return &gateTarget{id: 2} },
			targetPos:  [3]int{100 + rangeVal, 100, 0},
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

func TestHostileAutoAttackTargetValidExcludesNegativeRange(t *testing.T) {
	state := world.New()
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	state.Spawn(attacker, 100, 100, 0, 0)
	target := &gateTarget{id: 2}
	state.Spawn(target, 100, 100, 0, 0)

	if attacker.AutoAttackTargetValid(target, -1, false) {
		t.Fatal("AutoAttackTargetValid() = true with a negative range, want false")
	}
}

func TestHostileAutoAttackTargetValidExcludesOwnedSummonWhileOwnerRecentFakeDeath(t *testing.T) {
	state := world.New()
	attacker := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	state.Spawn(attacker, 100, 100, 0, 0)

	target := &ownedGateTarget{
		gateTarget: &gateTarget{id: 3},
		owner:      &gateTarget{id: 2, recentFakeDeath: true},
	}
	state.Spawn(target, 100, 100, 0, 0)

	if attacker.AutoAttackTargetValid(target, 500, true) {
		t.Fatal("AutoAttackTargetValid() = true for summon with a recently revived owner, want false")
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

// newFollowGateHostile builds a Hostile whose move controller is a
// hostileMove fake, returned alongside it so a test can script whether
// MaybeStartOffensiveFollow reports an active follow.
func newFollowGateHostile(t testing.TB, id int32, tpl *Template) (*Hostile, *hostileMove) {
	t.Helper()
	move := &hostileMove{}
	h, err := NewHostile(&Instance{ObjectID: id, Template: tpl, Kind: "Monster"}, newHostileLive(t), move, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return h, move
}

func TestHostileAutoAttackTargetValidQueuedAttackDesireFollowGate(t *testing.T) {
	// This ports Npc.java:2107-2110: a candidate already the FinalTarget of
	// a queued, non-moving ATTACK desire is excluded from re-validation
	// whenever that desire's follow either starts or is already under way
	// (maybeStartOffensiveFollow reporting true, e.g. because the target
	// sits out of melee range and a follow task got issued). A candidate
	// already close enough that no follow is needed (maybeStartOffensiveFollow
	// reporting false) falls through to the normal targeting rule instead.
	tests := []struct {
		name         string
		followResult bool
		moveToTarget bool
		want         bool
		wantFollowed bool
	}{
		{
			name:         "out-of-range queued attack desire starts a follow and is excluded",
			followResult: true,
			want:         false,
			wantFollowed: true,
		},
		{
			name:         "in-range queued attack desire needs no follow and falls through to the normal rule",
			followResult: false,
			want:         true,
			wantFollowed: true,
		},
		{
			name:         "an already-moving queued attack desire skips the gate entirely",
			followResult: true,
			moveToTarget: true,
			want:         true,
			wantFollowed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := world.New()
			attacker, move := newFollowGateHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
			move.followResult = tc.followResult
			state.Spawn(attacker, 100, 100, 0, 0)

			target := &gateTarget{id: 2}
			state.Spawn(target, 100, 100, 0, 0)

			attacker.AI().Desires().AddOrUpdate(&ai.Desire{
				Kind:         ai.IntentionAttack,
				FinalTarget:  target,
				MoveToTarget: tc.moveToTarget,
			})

			if got := attacker.AutoAttackTargetValid(target, 500, false); got != tc.want {
				t.Fatalf("AutoAttackTargetValid() = %v, want %v", got, tc.want)
			}
			if followed := move.followTarget != nil; followed != tc.wantFollowed {
				t.Fatalf("MaybeStartOffensiveFollow called = %v, want %v", followed, tc.wantFollowed)
			}
		})
	}
}

func TestHostileAutoAttackTargetValidNoQueuedDesireSkipsFollowGate(t *testing.T) {
	state := world.New()
	attacker, move := newFollowGateHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	move.followResult = true
	state.Spawn(attacker, 100, 100, 0, 0)

	target := &gateTarget{id: 2}
	state.Spawn(target, 100, 100, 0, 0)

	if !attacker.AutoAttackTargetValid(target, 500, false) {
		t.Fatal("AutoAttackTargetValid() = false, want true: no queued attack desire means the follow gate must not apply")
	}
	if move.followTarget != nil {
		t.Fatal("MaybeStartOffensiveFollow: called, want the gate to skip it with no matching queued desire")
	}
}

func TestHostileRandomizeHateDoesNotExcludeOrdinaryHateListCandidate(t *testing.T) {
	// Regression for #1710: AddDamageHate now sets Desire.MoveToTarget = true
	// (NpcAI.java:698-711's default), so an ordinary hate-list candidate's
	// queued ATTACK desire never matches NonMovingAttack and #1593's follow
	// gate never fires for it — even when MaybeStartOffensiveFollow would
	// report an active follow. Pre-fix, the always-false MoveToTarget
	// default made the gate wrongly exclude this candidate from candidacy.
	state := world.New()
	owner, move := newFollowGateHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 10})
	owner.SetRollSource(zeroRoll)
	move.followResult = true
	state.Spawn(owner, 100, 100, 0, 0)

	mostHated := &gateTarget{id: 2}
	candidate := &gateTarget{id: 3}
	state.Spawn(mostHated, 100, 100, 0, 0)
	state.Spawn(candidate, 100, 100, 0, 0)

	owner.AddDamageHate(mostHated, 0, 999)
	owner.AddDamageHate(candidate, 0, 25)

	if ok := owner.RandomizeHate(); !ok {
		t.Fatal("RandomizeHate: ok = false, want true: an ordinary hate-list candidate must not be excluded by the follow gate")
	}
	if got := owner.AI().Threats().Hate(candidate); got != 1199 {
		t.Fatalf("displaced candidate hate = %v, want 999 + 200 = 1199", got)
	}
	if got := owner.AI().Threats().Hate(mostHated); got != 999 {
		t.Fatalf("mostHated hate = %v, want unchanged 999", got)
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

	boundary := &gateTarget{id: 2}
	near := &gateTarget{id: 3}
	state.Spawn(boundary, 100+rangeVal, 100, 0, 0)
	state.Spawn(near, 100, 100, 0, 0)
	owner.AddDamageHate(boundary, 0, 10)
	owner.AddDamageHate(near, 0, 25)

	// near is mostHated (higher hate), leaving boundary as the sole candidate;
	// the strict rangeVal filter excludes a target exactly at the boundary.
	if _, ok := owner.ReconsiderTarget(rangeVal); ok {
		t.Fatal("ReconsiderTarget: ok = true, want false when the only candidate is exactly at range")
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

func TestHostileReconsiderTargetKnownlistExcludesExactRangeBoundary(t *testing.T) {
	const rangeVal = 500

	state := world.New()
	owner := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", AggroRange: 1000})
	owner.SetWorld(state)
	state.Spawn(owner, 100, 100, 0, 0)

	boundary := &gateTarget{id: 2}
	state.Spawn(boundary, 100+rangeVal, 100, 0, 0)

	if _, ok := owner.ReconsiderTarget(rangeVal); ok {
		t.Fatal("ReconsiderTarget: ok = true, want false for a known-list target exactly at range")
	}
}

func TestHostileSiegeGuardAutoAttackTargetValid(t *testing.T) {
	const originX = 100

	tests := []struct {
		name         string
		dead         bool
		silent       bool
		attackableBy bool
		dx           int
		want         bool
	}{
		{name: "attackable player-alike target is valid", attackableBy: true, want: true},
		{name: "alike-dead acting player is excluded", dead: true, attackableBy: true, want: false},
		{name: "silently moving target beyond 250 units is excluded", silent: true, attackableBy: true, dx: 300, want: false},
		{name: "silently moving target within 250 units is included", silent: true, attackableBy: true, dx: 100, want: true},
		{name: "a target the reference marks unattackable is excluded", attackableBy: false, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := world.New()
			guard := newKindHostile(t, 1, &Template{ID: 1, Type: "SiegeGuard"}, "SiegeGuard")
			state.Spawn(guard, originX, 100, 0, 0)

			target := &siegeGateTarget{id: 2, dead: tc.dead, silent: tc.silent, attackableBy: tc.attackableBy}
			state.Spawn(target, originX+tc.dx, 100, 0, 0)

			if got := guard.siegeGuardAutoAttackTargetValid(target); got != tc.want {
				t.Fatalf("siegeGuardAutoAttackTargetValid() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestHostileSiegeGuardAutoAttackTargetValidChecksOwningPlayerNotSummon
// proves the alike-dead/silent-moving/distance gates run against the
// summon's owning player, not the summon's own state (SiegeGuard.java:84-93
// resolves target.getActingPlayer() once and checks every gate on that
// player). A living summon owned by an alike-dead player must be rejected
// even though the summon itself isn't dead.
func TestHostileSiegeGuardAutoAttackTargetValidChecksOwningPlayerNotSummon(t *testing.T) {
	state := world.New()
	guard := newKindHostile(t, 1, &Template{ID: 1, Type: "SiegeGuard"}, "SiegeGuard")
	state.Spawn(guard, 100, 100, 0, 0)

	owner := &siegeGateTarget{id: 3, dead: true, attackableBy: true}
	summon := &ownedSiegeGateTarget{
		siegeGateTarget: &siegeGateTarget{id: 2, dead: false, attackableBy: true},
		owner:           owner,
	}
	state.Spawn(summon, 100, 100, 0, 0)

	if guard.siegeGuardAutoAttackTargetValid(summon) {
		t.Fatal("siegeGuardAutoAttackTargetValid(summon) = true, want false: owning player is alike-dead")
	}
}

// TestHostileSiegeGuardAutoAttackTargetValidExcludesNPC proves the
// dedicated SiegeGuard rule requires an acting player (SiegeGuard.java:84:
// target.getActingPlayer() == null rejects), unlike the general rule's
// Guard/FriendlyMonster karma check.
func TestHostileSiegeGuardAutoAttackTargetValidExcludesNPC(t *testing.T) {
	state := world.New()
	guard := newKindHostile(t, 1, &Template{ID: 1, Type: "SiegeGuard"}, "SiegeGuard")
	state.Spawn(guard, 100, 100, 0, 0)

	other := newCombatHostile(t, 2, &Template{ID: 2, Type: "Monster"})
	state.Spawn(other, 100, 100, 0, 0)

	if guard.siegeGuardAutoAttackTargetValid(other) {
		t.Fatal("siegeGuardAutoAttackTargetValid(NPC target) = true, want false")
	}
}

// TestHostileAutoAttackTargetValidSiegeGuardGeneralPathUnchanged proves the
// three-argument RandomizeHate path (AutoAttackTargetValid) keeps SiegeGuard
// on the same default rule every other non-Guard/FriendlyMonster kind uses,
// unaffected by the new one-argument siegeGuardAutoAttackTargetValid gate: a
// target this NPC rejects for isAttackableBy still passes the general rule,
// and a plain NPC target is still excluded (SiegeGuard isn't confused).
func TestHostileAutoAttackTargetValidSiegeGuardGeneralPathUnchanged(t *testing.T) {
	state := world.New()
	guard := newKindHostile(t, 1, &Template{ID: 1, Type: "SiegeGuard", AggroRange: 10}, "SiegeGuard")
	state.Spawn(guard, 100, 100, 0, 0)

	// Not attackable under the dedicated siege rule, yet the general rule
	// never consults AttackableBy and still accepts it.
	target := &siegeGateTarget{id: 2, attackableBy: false}
	state.Spawn(target, 100, 100, 0, 0)

	if !guard.AutoAttackTargetValid(target, 500, false) {
		t.Fatal("AutoAttackTargetValid() = false, want true: the general path must ignore isAttackableBy")
	}

	otherNPC := newCombatHostile(t, 3, &Template{ID: 3, Type: "Monster"})
	state.Spawn(otherNPC, 100, 100, 0, 0)

	if guard.AutoAttackTargetValid(otherNPC, 500, false) {
		t.Fatal("AutoAttackTargetValid(other NPC) = true, want false: SiegeGuard isn't confused")
	}
}

// TestHostileReconsiderTargetUsesSiegeGuardRuleFromHateList proves
// ReconsiderTarget's hate-list branch gates a SiegeGuard's candidates
// through siegeGuardAutoAttackTargetValid rather than the general rule: a
// hate-list entry the reference marks unattackable is excluded even though
// it would pass the general rule's default (non-Guard) branch.
func TestHostileReconsiderTargetUsesSiegeGuardRuleFromHateList(t *testing.T) {
	state := world.New()
	guard := newKindHostile(t, 1, &Template{ID: 1, Type: "SiegeGuard", AggroRange: 10}, "SiegeGuard")
	state.Spawn(guard, 100, 100, 0, 0)

	unattackable := &siegeGateTarget{id: 2, attackableBy: false}
	attackable := &siegeGateTarget{id: 3, attackableBy: true}
	state.Spawn(unattackable, 100, 100, 0, 0)
	state.Spawn(attackable, 100, 100, 0, 0)
	guard.AddDamageHate(unattackable, 0, 10)
	guard.AddDamageHate(attackable, 0, 5)

	chosen, ok := guard.ReconsiderTarget(0)
	if !ok {
		t.Fatal("ReconsiderTarget: ok = false, want true")
	}
	if chosen.ObjectID() != attackable.ObjectID() {
		t.Fatalf("chosen = %d, want %d: the unattackable, higher-hate entry must be skipped", chosen.ObjectID(), attackable.ObjectID())
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
