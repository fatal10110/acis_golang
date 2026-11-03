package npc

import (
	"math"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func partyAI(partyType, loyalty int) AIParams {
	params := AIParams{"Party_Type": strconv.Itoa(partyType)}
	if loyalty != 0 {
		params["Party_Loyalty"] = strconv.Itoa(loyalty)
	}
	return params
}

func partyHostile(t *testing.T, id int32, partyType int, move ai.AttackableMoveController) *Hostile {
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

func spawnPartyWorld(t *testing.T, actors ...*Hostile) *world.State {
	t.Helper()
	state := world.New()
	for i, actor := range actors {
		actor.Attach(Runtime{World: state})
		state.Spawn(actor, i*100, 0, 0, 0)
	}
	return state
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
