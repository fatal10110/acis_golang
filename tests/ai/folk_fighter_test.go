package ai

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// folkFighterTemplate is the fixture civilian NPC of id that fights: it
// can move, reaches 80 and hits, lightly.
func folkFighterTemplate(id int32) *npc.Template {
	tmpl := gameservertest.FolkTemplate("Folk", int(id))
	tmpl.CanMove = true
	tmpl.BaseAttackRange = 80
	tmpl.PAtk = 1
	tmpl.Level = 1
	tmpl.STR = 40
	tmpl.Undying = false
	return tmpl
}

// characterOf returns the character booted into srv's world.
func characterOf(t *testing.T, srv *gameservertest.Server) attackable.Combatant {
	t.Helper()
	obj, ok := srv.State.Object(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("the character is not in the world")
	}
	return obj.(attackable.Combatant)
}

// hitFolk lands a hit of damage on f from the character, on its queue.
func hitFolk(t *testing.T, srv *gameservertest.Server, f *npc.Folk, from attackable.Combatant, damage int) {
	t.Helper()
	done := make(chan struct{})
	if !srv.PlayerQueue(t, from.ObjectID()).Post(func() {
		defer close(done)
		f.TakeDamage(damage, from)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	srv.Settle(t)
}

// attackTarget reads the main target id of an Attack frame.
func attackTarget(f []byte) int32 { return int32(binary.LittleEndian.Uint32(f[5:9])) }

// TestFolkGuardStrikesBackAtItsAttacker runs a civilian border patrol under
// a behavior shaped like its reference Guard: its attacked hook asks it to
// attack the attacker with weight 2000. The patrol hates no one yet, so it
// takes the attack up at once, before its first AI tick is over: it swings
// at the character, and the swing lands for the damage it showed. Its aggro
// list holds the character as the creature it hates most.
func TestFolkGuardStrikesBackAtItsAttacker(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	guard := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkPatrolID}, Hooks: script.Hooks{
			OnAttacked: func(_ *script.Script, e script.Attacked) {
				log.add("ATTACKED by=%d damage=%d", e.Attacker.ObjectID(), e.Damage)
				e.NPC.AddAttackDesire(e.Attacker, 2000)
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkGuard": guard})
	player := characterOf(t, srv)
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), location.Location{X: at.X + 40, Y: at.Y, Z: at.Z})
	id := f.ObjectID()
	srv.ReadQueued(t, srv.Client)
	vitals := player.(interface{ CurrentHP() int })
	hpBefore := vitals.CurrentHP()

	hitFolk(t, srv, f, player, 10)
	if got, want := log.take(), []string{fmt.Sprintf("ATTACKED by=%d damage=10", player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("hooks = %q, want %q", got, want)
	}
	swings := framesOf(srv.ReadQueued(t, srv.Client), serverpackets.OpcodeAttack, id)
	if len(swings) != 1 || attackTarget(swings[0]) != player.ObjectID() {
		t.Fatalf("swings = %d %v, want one at the character", len(swings), swings)
	}
	if got := f.CurrentIntention(); got != actorai.IntentionAttack {
		t.Fatalf("intention = %v, want attack", got)
	}
	if c, ok := f.MostHated(); !ok || c.ObjectID() != player.ObjectID() {
		t.Fatalf("most hated = %v, %v, want the character", c, ok)
	}

	// The swing lands: the character loses the damage it showed, none
	// for a miss.
	damage := int(int32(binary.LittleEndian.Uint32(swings[0][9:13])))
	if swings[0][13]&serverpackets.AttackHitMiss != 0 {
		damage = 0
	}
	srv.Advance(t, 3*time.Second)
	if got, want := vitals.CurrentHP(), max(hpBefore-damage, 0); got != want {
		t.Fatalf("character HP = %d after the swing of %d, want %d", got, damage, want)
	}
}

// TestFolkFleesFromItsAttacker runs a civilian NPC under a behavior that
// flees 300 from whoever hits it. The flee runs straight away from the
// character, in run stance; while it is current and queued, a heavier
// desire waits. The arrival opens the move-finished hook at the end of the
// run and drops the flee, and the next AI tick takes up the waiting desire.
func TestFolkFleesFromItsAttacker(t *testing.T) {
	t.Parallel()
	log := &folkLog{}
	coward := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{folkOfferingID}, Hooks: script.Hooks{
			OnAttacked: func(_ *script.Script, e script.Attacked) {
				e.NPC.AddFleeDesire(e.Attacker, 300, 1000)
			},
			OnMoveToFinished: func(_ *script.Script, e script.MoveToFinished) {
				log.add("MOVE_TO_FINISHED at=%d,%d,%d", e.X, e.Y, e.Z)
			},
		}}
	}
	srv, at := bootFolkDesires(t, script.Catalog{"ai.FolkCoward": coward})
	player := characterOf(t, srv)
	start := location.Location{X: at.X + 100, Y: at.Y, Z: at.Z}
	f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkOfferingID), start)
	id := f.ObjectID()
	srv.ReadQueued(t, srv.Client)
	cycle(t, srv)
	cycle(t, srv)
	log.take()

	hitFolk(t, srv, f, player, 10)
	srv.ReadQueued(t, srv.Client)
	moves := framesOf(cycle(t, srv), serverpackets.OpcodeMoveToLocation, id)
	want := start.FleeFrom(at.X, at.Y, 300)
	if len(moves) != 1 || moveDestination(moves[0]) != want {
		t.Fatalf("flee frames = %d (%v), want one MoveToLocation to %+v", len(moves), moves, want)
	}
	if got := f.CurrentIntention(); got != actorai.IntentionFlee {
		t.Fatalf("intention = %v, want flee", got)
	}
	if !f.Running() {
		t.Fatal("the flee runs in walk stance, want run")
	}

	f.AddDoNothingDesire(40, 1e6)
	cycle(t, srv)
	if got := f.CurrentIntention(); got != actorai.IntentionFlee {
		t.Fatalf("intention = %v while fleeing, want the flee to hold the selection", got)
	}
	walkUntilArrived(t, srv, f)
	if got, want := log.take(), []string{fmt.Sprintf("MOVE_TO_FINISHED at=%d,%d,%d", want.X, want.Y, want.Z)}; !slices.Equal(got, want) {
		t.Fatalf("hooks on the flee's arrival = %q, want %q", got, want)
	}
	cycle(t, srv)
	if got := f.CurrentIntention(); got != actorai.IntentionNothing {
		t.Fatalf("intention = %v after the flee, want the waiting do-nothing", got)
	}
}
