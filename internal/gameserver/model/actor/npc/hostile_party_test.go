package npc

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func partyAI(partyType, loyalty int) *commons.StatSet {
	set := commons.NewStatSet()
	set.Set("Party_Type", partyType)
	if loyalty != 0 {
		set.Set("Party_Loyalty", loyalty)
	}
	return set
}

func partyHostile(t *testing.T, id int32, partyType int, move ai.MoveController) *Hostile {
	t.Helper()
	tpl := &Template{
		ID: int(id), Type: "Monster", HPMax: 1000, CanMove: true, RunSpeed: 120,
		AIParams: partyAI(partyType, 0),
	}
	h, err := NewHostile(&Instance{ObjectID: id, Template: tpl, Kind: "Monster"}, newHostileLive(t), move, &hostileAttack{})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestMinionAssistsWhenMasterTakesDamage(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	master.TakeDamage(40, attacker)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire = (%v, %v), want attack on attacker", ok, d)
	}
	if got := d.Weight; got != 40 {
		t.Fatalf("minion attack weight = %v, want 40 (damage * party weight 1)", got)
	}
	if !d.MoveToTarget {
		t.Fatal("moving party assist MoveToTarget = false, want true")
	}
}

// TestMinionAssistsWhenMasterReduceHP repeats
// TestMinionAssistsWhenMasterTakesDamage's minion fan-out for a skill hit
// (ReduceHP) instead of melee (TakeDamage), pinning that
// registerHit's propagatePartyAttacked call — added in #2328, previously
// missing from ReduceHP/ReduceHPByDOT entirely — actually fires.
func TestMinionAssistsWhenMasterReduceHP(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	master.ReduceHP(40, attacker, skill.Definition{})

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire = (%v, %v), want attack on attacker", ok, d)
	}
	if got := d.Weight; got != 40 {
		t.Fatalf("minion attack weight = %v, want 40 (damage * party weight 1)", got)
	}
}

// TestMinionAssistsWhenMasterReduceHPByDOT repeats the same fan-out for a
// DOT tick (ReduceHPByDOT), the one direct-damage path #2328 actually made
// reachable (see PR #2332's reachability note on damageBlocked).
func TestMinionAssistsWhenMasterReduceHPByDOT(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	master.ReduceHPByDOT(40, attacker, true)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire = (%v, %v), want attack on attacker", ok, d)
	}
	if got := d.Weight; got != 40 {
		t.Fatalf("minion attack weight = %v, want 40 (damage * party weight 1)", got)
	}
}

func TestMasterDoesNotGainPartyDesireWhenMinionTakesDamage(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	minion.TakeDamage(40, attacker)

	if got := master.AI().Desires().Len(); got != 0 {
		t.Fatalf("master desires = %d, want 0 (Party_Type 2 without a master does not assist)", got)
	}
	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack {
		t.Fatalf("damaged minion desire = (%v, %v), want its own attack desire", ok, d)
	}
}

func TestSiblingMinionAssistsWhenPartyMemberTakesDamage(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	one := partyHostile(t, 2, 1, &hostileMove{})
	two := partyHostile(t, 3, 1, &hostileMove{})
	attacker := partyHostile(t, 4, 0, &hostileMove{})
	master.AddMinion(one)
	master.AddMinion(two)
	one.SetMaster(master)
	two.SetMaster(master)

	one.TakeDamage(25, attacker)

	d, ok := two.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("sibling desire = (%v, %v), want attack on attacker", ok, d)
	}
}

func TestLoyaltyTwoMinionAssistsOnlyWhenMasterIsCaller(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	loyal := partyHostile(t, 2, 1, &hostileMove{})
	sibling := partyHostile(t, 3, 1, &hostileMove{})
	attacker := partyHostile(t, 4, 0, &hostileMove{})
	loyal.Instance.Template.AIParams = partyAI(1, 2)
	master.AddMinion(loyal)
	master.AddMinion(sibling)
	loyal.SetMaster(master)
	sibling.SetMaster(master)

	sibling.TakeDamage(25, attacker)
	if got := loyal.AI().Desires().Len(); got != 0 {
		t.Fatalf("loyalty-2 minion desires after sibling hit = %d, want 0", got)
	}

	master.TakeDamage(25, attacker)
	d, ok := loyal.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("loyalty-2 minion desire after master hit = (%v, %v), want attack on attacker", ok, d)
	}
}

func TestNotifyAggressionFansOutToMinions(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	master.NotifyAggression(attacker, 80)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire after NotifyAggression = (%v, %v), want attack on attacker", ok, d)
	}
}

func spawnPartyWorld(t *testing.T, actors ...*Hostile) *world.State {
	t.Helper()
	state := world.New()
	for i, actor := range actors {
		actor.Attach(Runtime{World: state})
		state.Spawn(actor, i*100, 0, 0, 0)
	}
	return state
}

func TestStationaryMinionHoldsAttackWhenPlayableInRange(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	minion.Instance.Template.AIParams.Set("MovingAttack", 0)
	minion.Instance.Template.AggroRange = 500
	master.AddMinion(minion)
	minion.SetMaster(master)
	state := world.New()
	master.Attach(Runtime{World: state})
	minion.Attach(Runtime{World: state})
	state.Spawn(master, 0, 0, 0, 0)
	state.Spawn(minion, 10, 0, 0, 0)
	attacker := &hostileTarget{id: 99}
	state.Spawn(attacker, 20, 0, 0, 0)

	master.TakeDamage(40, attacker)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget.ObjectID() != attacker.ObjectID() {
		t.Fatalf("minion desire = (%v, %+v), want hold attack on playable", ok, d)
	}
	if d.MoveToTarget {
		t.Fatal("stationary party assist MoveToTarget = true, want false")
	}
	if got := d.Weight; got != 40 {
		t.Fatalf("hold attack weight = %v, want 40", got)
	}
}

func TestStationaryMinionDropsAttackWhenPlayableOutOfRangeAndIsTopDesire(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	minion.Instance.Template.AIParams.Set("MovingAttack", 0)
	minion.Instance.Template.AggroRange = 20
	master.AddMinion(minion)
	minion.SetMaster(master)
	state := world.New()
	master.Attach(Runtime{World: state})
	minion.Attach(Runtime{World: state})
	state.Spawn(master, 0, 0, 0, 0)
	state.Spawn(minion, 0, 0, 0, 0)
	attacker := &hostileTarget{id: 99}
	state.Spawn(attacker, 400, 0, 0, 0)

	minion.AddCombatDamageHate(attacker, 10)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() before assist = %v, want Attack so the playable is top desire", got)
	}

	master.TakeDamage(40, attacker)

	if got := minion.AI().Desires().Len(); got != 0 {
		t.Fatalf("desires after out-of-range hold assist = %d, want 0 (top desire dropped)", got)
	}
}

func TestMovingMinionTeleportsToTargetAfterGeoPathFails(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)
	spawnPartyWorld(t, master, minion, attacker)

	minion.AddCombatDamageHate(attacker, 10)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() before assist = %v, want Attack", got)
	}
	minion.SetHP(minion.MaxHPValue() / 2)
	for i := 0; i < 11; i++ {
		minion.AddGeoPathFailCount()
	}

	master.TakeDamage(40, attacker)

	mx, my, _ := minion.Position()
	ax, ay, _ := attacker.Position()
	if mx != ax || my != ay {
		t.Fatalf("minion position = (%d,%d), want attacker (%d,%d) after geo-fail teleport", mx, my, ax, ay)
	}
	if got := minion.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after teleport = %d, want 0", got)
	}
}

func TestMovingMinionRootedRetryRequeuesAttack(t *testing.T) {
	move := &hostileMove{}
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, move)
	attacker := partyHostile(t, 3, 0, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)
	spawnPartyWorld(t, master, minion, attacker)

	minion.AddCombatDamageHate(attacker, 10)
	if got := minion.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() before assist = %v, want Attack", got)
	}
	addHostileEffect(t, minion, "Root")
	if !minion.Rooted() {
		t.Fatal("Rooted() = false after Root effect, want true")
	}

	master.TakeDamage(40, attacker)

	d, ok := minion.AI().Desires().Peek()
	if !ok || d.Kind != ai.IntentionAttack || d.FinalTarget != attacker {
		t.Fatalf("minion desire after rooted retry = (%v, %v), want requeued attack", ok, d)
	}
	if got := d.Weight; got != 40 {
		t.Fatalf("rooted retry weight = %v, want 40 (cleared then requeued at party damage)", got)
	}
	if move.stopCount == 0 {
		t.Fatal("move.Stop() count = 0, want stop when dropping the out-of-range top desire")
	}
}

func TestMinionThinkFollowMovesToEscortSlot(t *testing.T) {
	masterMove := &hostileMove{}
	minionMove := &hostileMove{}
	master := partyHostile(t, 1, 2, masterMove)
	minion := partyHostile(t, 2, 1, minionMove)
	state := world.New()
	master.Attach(Runtime{World: state})
	minion.Attach(Runtime{World: state})
	state.Spawn(master, 1000, 1000, 0, 0)
	state.Spawn(minion, 0, 0, 0, 0)
	master.AddMinion(minion)
	minion.SetMaster(master)
	minion.roll = func(n int) int { return 0 }

	if minion.ThinkFollow(master, false) {
		t.Fatal("ThinkFollow() clearDesire = true, want follow to continue")
	}
	if len(minionMove.locations) != 1 {
		t.Fatalf("escort MoveToLocation count = %d, want 1", len(minionMove.locations))
	}
	got := minionMove.locations[0]
	if math.Abs(got.Distance2D(location.Location{X: 1000, Y: 1000, Z: 0})-150) > 1 {
		t.Fatalf("escort dest %v is not 150 from master", got)
	}
}

// A minion scans the escort slots after releasing the master's lock, so
// another minion can claim a slot in between; the write-back keeps that
// claim and still applies this minion's changes to untouched slots.
func TestFollowSlotCommitKeepsAConcurrentClaim(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	var snapshot [escortSlotCount]int32
	snapshot[3] = 7
	claimed := snapshot
	claimed[3] = 0
	claimed[5] = 2
	master.followSlots = snapshot
	master.followSlots[5] = 9

	master.commitFollowSlots(snapshot, claimed)

	if got := master.followSlots[3]; got != 0 {
		t.Fatalf("slot 3 = %d, want 0 (cleared, unchanged since the snapshot)", got)
	}
	if got := master.followSlots[5]; got != 9 {
		t.Fatalf("slot 5 = %d, want 9 (the concurrent claim stands)", got)
	}
}

func TestMinionThinkFollowLooseMovesTowardNonMaster(t *testing.T) {
	move := &hostileMove{}
	follower := partyHostile(t, 1, 1, move)
	target := partyHostile(t, 2, 0, &hostileMove{})
	state := world.New()
	follower.Attach(Runtime{World: state})
	target.Attach(Runtime{World: state})
	state.Spawn(follower, 0, 0, 0, 0)
	state.Spawn(target, 400, 0, 0, 0)
	n := 0
	follower.roll = func(bound int) int {
		n++
		switch n {
		case 1:
			return 51
		case 2:
			return 250000
		case 3:
			return 0
		default:
			t.Fatalf("unexpected roll call %d bound %d", n, bound)
			return 0
		}
	}

	if follower.ThinkFollow(target, false) {
		t.Fatal("ThinkFollow() clearDesire = true, want follow to continue")
	}
	if len(move.locations) != 1 {
		t.Fatalf("loose-follow MoveToLocation count = %d, want 1", len(move.locations))
	}
	got := move.locations[0]
	want := location.Location{X: 550, Y: 0, Z: 0}
	if got != want {
		t.Fatalf("loose-follow dest = %v, want %v (sqrt(0.25)*300 at angle 0 from target)", got, want)
	}
}

func TestMinionThinkFollowTeleportsAfterGeoPathFails(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	state := world.New()
	master.Attach(Runtime{World: state})
	minion.Attach(Runtime{World: state})
	state.Spawn(master, 500, 0, 0, 0)
	state.Spawn(minion, 0, 0, 0, 0)
	master.AddMinion(minion)
	minion.SetMaster(master)
	for i := 0; i < 10; i++ {
		minion.AddGeoPathFailCount()
	}

	if minion.ThinkFollow(master, true) {
		t.Fatal("ThinkFollow() clearDesire = true after geo fail, want follow kept")
	}
	x, y, _ := minion.Position()
	if x == 0 && y == 0 {
		t.Fatal("minion still at origin after geo-fail teleport, want near master")
	}
	if got := minion.GeoPathFailCount(); got != 0 {
		t.Fatalf("GeoPathFailCount() after teleport = %d, want 0", got)
	}
}

func TestIdlePartyPrivateQueuesFollowOnThink(t *testing.T) {
	master := partyHostile(t, 1, 2, &hostileMove{})
	minion := partyHostile(t, 2, 1, &hostileMove{})
	master.AddMinion(minion)
	minion.SetMaster(master)

	if err := minion.TickThink(); err != nil {
		t.Fatal(err)
	}
	if err := minion.TickThink(); err != nil {
		t.Fatal(err)
	}
	if err := minion.TickThink(); err != nil {
		t.Fatal(err)
	}
	if got := minion.AI().CurrentIntention(); got != ai.IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionFollow)
	}
}
