package skills

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The door clan hook follows Door.reduceCurrentHp: after the hit has done
// whatever its residence's siege lets it do to the door's HP, every NPC
// known within 600 of the door (3D, inclusive, widened by the door's
// collision radius of 16 and the NPC's own), alive and of the clan
// "door_clan", runs the static-object clan-attacked hook of every script
// bound to its template, with the attacker and the hit's damage truncated
// to an int. The door calls its clan on every hit, a harmless one too.

// The door clan NPCs: the guard and the clerk stand by the door, the edge
// guard exactly at the reach of the call, the far guard one step beyond it;
// the dead guard is dead and the stranger is of another clan.
const (
	doorGuardID     = 20901
	doorEdgeGuardID = 20902
	doorFarGuardID  = 20903
	doorDeadGuardID = 20904
	doorStrangerID  = 20905
	doorClerkID     = 30901

	// doorClanNPCRadius is each door clan NPC's collision radius.
	doorClanNPCRadius = 8
	// doorClanReach is how far from the door's centre an NPC's centre may
	// stand and still hear the door: the call's range of 600 widened by
	// the door's radius and the NPC's.
	doorClanReach = 600 + 16 + doorClanNPCRadius
)

// doorClanCall is one static-object clan-attacked hook run: the NPC called,
// the attacker's name, the damage, the door's HP read inside the hook and
// whether the attacker was a siege summon.
type doorClanCall struct {
	called      string
	attacker    string
	damage      int32
	doorHP      int
	siegeSummon bool
}

type doorClanLog struct {
	mu    sync.Mutex
	names map[int32]string
	gate  *door.Object
	calls []doorClanCall
}

func newDoorClanLog() *doorClanLog { return &doorClanLog{names: map[int32]string{}} }

func (l *doorClanLog) name(objectID int32, role string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.names[objectID] = role
}

func (l *doorClanLog) watch(gate *door.Object) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gate = gate
}

// behavior is a behavior bound to ids that records its static-object
// clan-attacked hook.
func (l *doorClanLog) behavior(ids ...int32) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: ids, Hooks: script.Hooks{
			OnStaticObjectClanAttacked: func(_ *script.Script, e script.StaticObjectClanAttacked) {
				l.mu.Lock()
				defer l.mu.Unlock()
				hp := -1
				if l.gate != nil {
					hp = l.gate.HP()
				}
				l.calls = append(l.calls, doorClanCall{
					called: l.names[e.Called.ObjectID()], attacker: e.Attacker.Name(), damage: e.Damage,
					doorHP: hp, siegeSummon: e.Attacker.SiegeSummon(),
				})
			},
		}}
	}
}

// take returns the calls recorded since the last take, in the order they
// ran.
func (l *doorClanLog) take() []doorClanCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.calls
	l.calls = nil
	return out
}

// byCalled sorts calls by the NPC called: the world hands the NPCs in range
// in no set order.
func byCalled(calls []doorClanCall) []doorClanCall {
	slices.SortFunc(calls, func(a, b doorClanCall) int { return strings.Compare(a.called, b.called) })
	return calls
}

func doorClanTemplate(id int, clan string) *npc.Template {
	return &npc.Template{
		ID: id, TemplateID: id, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: doorClanNPCRadius, CollisionHeight: 20,
		Clans: []string{clan}, ClanRange: 300,
	}
}

// bootDoorClan boots the door combat world with the recording behavior
// bound to every door clan NPC id, spawns those NPCs around the door and
// returns the server, the player and the door.
func bootDoorClan(t *testing.T, log *doorClanLog) (*gameservertest.Server, int32, *door.Object) {
	t.Helper()
	hostile := []int32{doorGuardID, doorEdgeGuardID, doorFarGuardID, doorDeadGuardID, doorStrangerID}
	kinds := map[int32]script.NPCKind{doorClerkID: script.KindFolk}
	for _, id := range hostile {
		kinds[id] = script.KindHostile
	}
	srv, objID, gate := bootDoorCombat(t, combatDoorTemplate(door.KindDoor), nil,
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "ai.DoorClanRecorder"}},
			script.Catalog{"ai.DoorClanRecorder": log.behavior(append(hostile, doorClerkID)...)}))
	log.watch(gate)
	x, y, z := gate.Position()
	at := func(dx, dy int) location.Location { return location.Location{X: x + dx, Y: y + dy, Z: z} }
	for _, n := range []struct {
		role  string
		id    int
		clan  string
		where location.Location
	}{
		{"guard", doorGuardID, "door_clan", at(0, 150)},
		{"edge", doorEdgeGuardID, "door_clan", at(0, doorClanReach)},
		{"far", doorFarGuardID, "door_clan", at(0, -doorClanReach-1)},
		{"dead", doorDeadGuardID, "door_clan", at(150, 150)},
		{"stranger", doorStrangerID, "other_clan", at(-150, 150)},
	} {
		h := srv.SpawnHostileNPCTemplateAt(t, doorClanTemplate(n.id, n.clan), n.where)
		log.name(h.ObjectID(), n.role)
		if n.role == "dead" {
			h.MarkDead()
		}
	}
	clerkTmpl := gameservertest.FolkTemplate("Folk", doorClerkID)
	clerkTmpl.Clans = []string{"door_clan"}
	clerk := srv.SpawnFolkNPCAt(t, clerkTmpl, at(150, -150))
	log.name(clerk.ObjectID(), "clerk")
	return srv, objID, gate
}

// TestDoorHitCallsDoorClan pins the call of a harmless hit: with no siege
// the door keeps its HP, and the living door clan NPCs within reach, the
// civilian clerk and the guard standing exactly at the reach included, run
// their hooks with the player as the attacker and the damage truncated.
func TestDoorHitCallsDoorClan(t *testing.T) {
	t.Parallel()
	log := newDoorClanLog()
	srv, objID, gate := bootDoorClan(t, log)

	srv.HitDoor(t, gate.DoorID(), objID, 37.9)

	if gate.CurrentHP() != combatDoorMaxHP {
		t.Fatalf("door HP = %v after a hit outside any siege, want %d", gate.CurrentHP(), combatDoorMaxHP)
	}
	want := []doorClanCall{
		{called: "clerk", attacker: "Newbie", damage: 37, doorHP: combatDoorMaxHP},
		{called: "edge", attacker: "Newbie", damage: 37, doorHP: combatDoorMaxHP},
		{called: "guard", attacker: "Newbie", damage: 37, doorHP: combatDoorMaxHP},
	}
	if got := byCalled(log.take()); !slices.Equal(got, want) {
		t.Fatalf("door clan calls = %+v, want %+v", got, want)
	}
}

// TestDoorAutoAttackCallsDoorClanAfterHP pins the call of a landed
// auto-attack under a siege: the hit lowers the door's HP first, and the
// hooks then see the lowered HP and the hit's damage.
func TestDoorAutoAttackCallsDoorClanAfterHP(t *testing.T) {
	t.Parallel()
	log := newDoorClanLog()
	srv, objID, gate := bootDoorClan(t, log)
	srv.BeginDoorSiege(t, gate.DoorID())
	steadyHits(t, srv, objID)

	attackDoor(t, srv.Client, gate)
	hit := readDoorHit(t, srv.Client, objID, gate)

	// The attack goes on swinging; the first three calls are the first
	// hit's, all made inside its landing.
	var got []doorClanCall
	srv.AdvanceUntil(t, "the first hit's door clan calls", func() bool {
		got = append(got, log.take()...)
		return len(got) >= 3
	})
	got = byCalled(got[:3])
	want := []doorClanCall{
		{called: "clerk", attacker: "Newbie", damage: hit.swing, doorHP: int(hit.hp)},
		{called: "edge", attacker: "Newbie", damage: hit.swing, doorHP: int(hit.hp)},
		{called: "guard", attacker: "Newbie", damage: hit.swing, doorHP: int(hit.hp)},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("door clan calls of the first hit = %+v, want %+v", got, want)
	}
}
