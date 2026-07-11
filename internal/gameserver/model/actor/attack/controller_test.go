package attack

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

func TestCreatureAttackBroadcastsSimpleHitAndTracksState(t *testing.T) {
	actor := attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		attackType:  item.WeaponSword,
		attackSpeed: 500,
		soulshot:    true,
		weaponGrade: 2,
		hit: Hit{
			TargetID: 200,
			Damage:   37,
			Crit:     true,
			Shield:   formulas.ShieldSuccess,
		},
	}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewCreature(&actor)

	ctrl.DoAttack(&target)
	defer ctrl.Stop()

	if !ctrl.AttackingNow() {
		t.Fatal("AttackingNow() = false after DoAttack")
	}
	if ctrl.BowCoolingDown() {
		t.Fatal("BowCoolingDown() = true for a sword attack")
	}
	if !ctrl.InHitAnimation() {
		t.Fatal("InHitAnimation() = false after DoAttack")
	}
	if actor.headingTarget != &target {
		t.Fatalf("heading target = %v, want attack target", actor.headingTarget)
	}
	wantFlags := uint8(serverpackets.AttackHitSoulshot | 2 | serverpackets.AttackHitCritical | serverpackets.AttackHitShield)
	if len(actor.broadcasts) != 1 {
		t.Fatalf("broadcast count = %d, want 1", len(actor.broadcasts))
	}
	got := actor.broadcasts[0]
	if got.AttackerID != 100 || got.X != 10 || got.Y != 20 || got.Z != -30 {
		t.Fatalf("broadcast origin = %+v, want attacker id and position", got)
	}
	if len(got.Hits) != 1 || got.Hits[0] != (serverpackets.AttackHit{TargetID: 200, Damage: 37, Flags: wantFlags}) {
		t.Fatalf("broadcast hits = %+v, want one critical soulshot shield hit", got.Hits)
	}
}

func TestPlayableAttackRejectsPeaceZoneForPlayableTargets(t *testing.T) {
	actor := attackActor{known: map[int32]bool{200: true}, canSee: true, canReach: true, playable: true, peace: true}
	target := attackTarget{id: 200, attackable: true, playable: true}

	if NewPlayable(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for playable attacker in peace zone")
	}

	actor.peace = false
	target.peace = true
	if NewPlayable(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for playable target in peace zone")
	}
}

func TestPlayerAttackRejectsFishingRodAndBowRequirements(t *testing.T) {
	target := attackTarget{id: 200, attackable: true}

	actor := playerAttackActor{attackActor: attackActor{
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		playable:    true,
		attackType:  item.WeaponFishingRod,
		attackSpeed: 500,
	}}
	if NewPlayer(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true with a fishing rod")
	}

	actor.attackType = item.WeaponBow
	actor.arrows = false
	if NewPlayer(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for bow without arrows")
	}

	actor.arrows = true
	actor.mpConsume = 5
	actor.mp = 4
	if NewPlayer(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for bow without enough MP")
	}
}

func TestPlayerAttackClearsFakeDeathAndStopSendsActionFailure(t *testing.T) {
	actor := playerAttackActor{attackActor: attackActor{
		id:          100,
		known:       map[int32]bool{200: true},
		canSee:      true,
		canReach:    true,
		playable:    true,
		attackType:  item.WeaponSword,
		attackSpeed: 500,
		hit:         Hit{TargetID: 200, Damage: 1},
	}}
	target := attackTarget{id: 200, attackable: true}
	ctrl := NewPlayer(&actor)

	ctrl.DoAttack(&target)
	ctrl.Stop()

	if actor.clearFakeDeathCalls != 1 {
		t.Fatalf("ClearRecentFakeDeath calls = %d, want 1", actor.clearFakeDeathCalls)
	}
	if actor.idleCalls != 1 {
		t.Fatalf("TryToIdle calls = %d, want 1", actor.idleCalls)
	}
	if actor.actionFailedCalls != 1 {
		t.Fatalf("ClientActionFailed calls = %d, want 1", actor.actionFailedCalls)
	}
}

func TestAttackableAttackRejectsFakeDeathTarget(t *testing.T) {
	actor := attackActor{known: map[int32]bool{200: true}, canSee: true, canReach: true}
	target := attackTarget{id: 200, attackable: true, fakeDeath: true}

	if NewAttackable(&actor).CanAttack(&target) {
		t.Fatal("CanAttack() = true for fake-death target")
	}
}

type attackActor struct {
	id int32

	known    map[int32]bool
	canSee   bool
	canReach bool

	attackDisabled   bool
	movementDisabled bool
	attackType       item.WeaponType
	attackSpeed      int
	weaponReuse      time.Duration
	weaponGrade      int
	soulshot         bool
	hit              Hit
	playable         bool
	peace            bool

	headingTarget attackable.Combatant
	broadcasts    []serverpackets.AttackSnapshot
	idleCalls     int
}

func (a *attackActor) ObjectID() int32 {
	if a.id == 0 {
		return 100
	}
	return a.id
}

func (a *attackActor) SiegeGuard() bool { return false }
func (a *attackActor) AlikeDead() bool  { return false }
func (a *attackActor) AttackDisabled() bool {
	return a.attackDisabled
}
func (a *attackActor) MovementDisabled() bool {
	return a.movementDisabled
}
func (a *attackActor) InAttackRange(attackable.Combatant) bool {
	return a.canReach
}
func (a *attackActor) Knows(target attackable.Combatant) bool {
	return a.known[target.ObjectID()]
}
func (a *attackActor) CanSee(attackable.Combatant) bool { return a.canSee }
func (a *attackActor) AttackType() item.WeaponType      { return a.attackType }
func (a *attackActor) AttackSpeed() int {
	if a.attackSpeed == 0 {
		return 500
	}
	return a.attackSpeed
}
func (a *attackActor) WeaponReuseDelay() time.Duration { return a.weaponReuse }
func (a *attackActor) WeaponGrade() int                { return a.weaponGrade }
func (a *attackActor) SoulshotCharged() bool           { return a.soulshot }
func (a *attackActor) Position() (int, int, int)       { return 10, 20, -30 }
func (a *attackActor) SetHeadingTo(target attackable.Combatant) {
	a.headingTarget = target
}
func (a *attackActor) MakeAttackHit(target attackable.Combatant, split bool) Hit {
	hit := a.hit
	hit.Target = target
	if hit.TargetID == 0 {
		hit.TargetID = target.ObjectID()
	}
	if split {
		hit.Damage /= 2
	}
	return hit
}
func (a *attackActor) BroadcastAttack(snapshot serverpackets.AttackSnapshot) {
	a.broadcasts = append(a.broadcasts, snapshot)
}
func (a *attackActor) InPeaceZone() bool { return a.peace }
func (a *attackActor) TryToIdle()        { a.idleCalls++ }

type playerAttackActor struct {
	attackActor

	arrows              bool
	mpConsume           int
	mp                  int
	clearFakeDeathCalls int
	actionFailedCalls   int
}

func (a *playerAttackActor) CheckAndEquipArrows() bool { return a.arrows }
func (a *playerAttackActor) WeaponMPConsume() int      { return a.mpConsume }
func (a *playerAttackActor) MP() int                   { return a.mp }
func (a *playerAttackActor) ClearRecentFakeDeath()     { a.clearFakeDeathCalls++ }
func (a *playerAttackActor) ClientActionFailed()       { a.actionFailedCalls++ }

type attackTarget struct {
	id         int32
	attackable bool
	siegeGuard bool
	alikeDead  bool
	playable   bool
	peace      bool
	fakeDeath  bool
}

func (t *attackTarget) ObjectID() int32  { return t.id }
func (t *attackTarget) SiegeGuard() bool { return t.siegeGuard }
func (t *attackTarget) AlikeDead() bool  { return t.alikeDead }
func (t *attackTarget) AttackableBy(CreatureActor) bool {
	return t.attackable
}
func (t *attackTarget) Playable() bool    { return t.playable }
func (t *attackTarget) InPeaceZone() bool { return t.peace }
func (t *attackTarget) FakeDeath() bool   { return t.fakeDeath }
