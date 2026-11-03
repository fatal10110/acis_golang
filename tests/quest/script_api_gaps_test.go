package quest

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The player's attack stance (#4025) and the door by id (#4026), read and
// moved from a script invocation in a booted world.

// TestPlayerInCombatReadsAttackStance: a player out of combat reads false;
// a monster's landed hit puts the player in its attack stance and the
// player reads true; once the stance expires on the stance task's sweep it
// reads false again.
func TestPlayerInCombatReadsAttackStance(t *testing.T) {
	t.Parallel()
	var nowMS atomic.Int64
	srv, objID := bootHelpers(t, gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }))
	inCombat := func() bool {
		t.Helper()
		var got bool
		run(t, srv, objID, func(_ *script.Script, p *script.Player) { got = p.InCombat() })
		return got
	}
	if inCombat() {
		t.Fatal("a player nobody fought reads in combat")
	}

	x, y, z := srv.PlayerPosition(t, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: x + 40, Y: y, Z: z})
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from the world")
	}
	stance := obj.(task.AttackStanceActor)
	attacker.DoAttack(t, obj.(attackable.Combatant))
	srv.Settle(t)
	if !srv.AttackStance.InAttackStance(stance) {
		t.Fatal("the landed hit did not put the player in its attack stance")
	}
	if !inCombat() {
		t.Fatal("a player in its attack stance reads out of combat")
	}

	nowMS.Add(task.AttackStancePeriod.Milliseconds() + 1000)
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}
	srv.Settle(t)
	if srv.AttackStance.InAttackStance(stance) {
		t.Fatal("the player's stance did not expire")
	}
	if inCombat() {
		t.Fatal("a player whose stance expired reads in combat")
	}
}

// TestDoorByID: the door found by its id is the door its name finds; it
// opens and closes as the named handle does, each change shown once to the
// players around it, and a door already in that state changes nothing. An
// id no door has finds nothing.
func TestDoorByID(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, gameservertest.WithDoors(scriptGate()))
	gate, ok := srv.WorldObjects.Door(scriptGateID)
	if !ok {
		t.Fatal("door not spawned")
	}
	srv.ReadQueued(t, srv.Client)

	step := func(open, wantOpen bool, wantUpdates ...int32) {
		t.Helper()
		var byName bool
		run(t, srv, objID, func(sc *script.Script, _ *script.Player) {
			d := sc.DoorByID(scriptGateID)
			if d == nil {
				t.Error("DoorByID found no door")
				return
			}
			if open {
				d.Open()
			} else {
				d.Close()
			}
			byName = sc.Door("script_gate").Opened()
		})
		if gate.Opened() != wantOpen || byName != wantOpen {
			t.Fatalf("open=%v: door opened %v, by name %v, want %v", open, gate.Opened(), byName, wantOpen)
		}
		if got := doorUpdates(srv.ReadQueued(t, srv.Client), gate.ObjectID()); !slices.Equal(got, wantUpdates) {
			t.Fatalf("open=%v: DoorStatusUpdate closed flags %v, want %v", open, got, wantUpdates)
		}
	}
	step(true, true, 0)
	step(true, true)
	step(false, false, 1)
	step(false, false)

	run(t, srv, objID, func(sc *script.Script, _ *script.Player) {
		if sc.DoorByID(scriptGateID+1) != nil {
			t.Error("DoorByID of an id no door has found a door")
		}
	})
}
