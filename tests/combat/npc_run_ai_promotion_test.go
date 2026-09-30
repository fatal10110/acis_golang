package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// attackTargetID reads the first hit's target id from an Attack frame.
func attackTargetID(frame []byte) int32 {
	r := wireReader(frame[1:])
	r.ReadInt32() // attacker id
	return r.ReadInt32()
}

// TestHeavierAttackDesireSwitchesRunningAttackTarget pins NpcAI.runAI
// running the heaviest queued desire over a running attack: a monster
// swinging at one player that gets a heavier attack desire on a second
// player swings at the second one on the runAI its current swing's finish
// runs, while the first player's desire is still queued.
func TestHeavierAttackDesireSwitchesRunningAttackTarget(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	first := livePlayer(t, srv, objID).(attackable.Combatant)

	rivalRow := srv.SeedCharacterFor(t, "rival", "Rival", 5, 0)
	rivalClient := srv.DialClient(t, "rival", 1)
	startInWorld(t, rivalClient)
	rivalObj, ok := srv.State.Player(rivalRow.ID)
	if !ok {
		t.Fatal("rival missing from world")
	}
	rival := rivalObj.(attackable.Combatant)
	drainUntilQuiet(t, c)

	x, y, z := srv.PlayerPosition(t, objID)
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.AtkSpd = latchAtkSpd
	tmpl.PAtk = 0.25
	// In reach, so the promoting pass swings at once.
	tmpl.BaseAttackRange = 40
	at := location.Location{X: x + 20, Y: y, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	drainUntilQuiet(t, c)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	walkInPlace(t, srv, hostile)
	drainUntilQuiet(t, c)

	hostile.AddDamageHate(first, 0, 100)
	hostile.AddAttackDesire(first, 5)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	frames := readUntil(t, c, serverpackets.OpcodeAttack, "Attack on the first player")
	if got := attackTargetID(frames[len(frames)-1]); got != first.ObjectID() {
		t.Fatalf("first swing target = %d, want first player %d", got, first.ObjectID())
	}
	attackTime := time.Duration(formulas.TimeBetweenAttacks(hostile.AttackSpeed())) * time.Millisecond

	hostile.AddAttackDesire(rival, 500)
	if got := hostile.AI().Desires().Len(); got != 2 {
		t.Fatalf("queued desires = %d, want both attack desires", got)
	}

	// The 10 ms windows around the swing's end hold only on the driven
	// clock: on the wall clock the swing began before its Attack frame
	// arrived, by however long delivery took.
	if srv.DrivesClock() {
		srv.Advance(t, attackTime-10*time.Millisecond)
		if got := hostile.AI().TopDesireTarget(); got == nil || got.ObjectID() != first.ObjectID() {
			t.Fatalf("TopDesireTarget() inside the first swing = %v, want the first player", got)
		}

		srv.Advance(t, 20*time.Millisecond)
		if got := hostile.AI().TopDesireTarget(); got == nil || got.ObjectID() != rival.ObjectID() {
			t.Fatalf("TopDesireTarget() after the swing finished = %v, want the rival", got)
		}
	}
	frames = readUntil(t, c, serverpackets.OpcodeAttack, "Attack on the rival")
	if got := attackTargetID(frames[len(frames)-1]); got != rival.ObjectID() {
		t.Fatalf("second swing target = %d, want rival %d (first player %d)", got, rival.ObjectID(), first.ObjectID())
	}
	if got := hostile.AI().Desires().Len(); got != 2 {
		t.Fatalf("queued desires after the switch = %d, want the first player's desire still queued", got)
	}
}

// TestHeavierMoveToTakesOverRunningAttack pins that a heavier queued
// MOVE_TO takes over a running attack on the runAI outside the hit
// animation, even though the attack's own desire is still queued.
func TestHeavierMoveToTakesOverRunningAttack(t *testing.T) {
	t.Parallel()
	srv, hostile, attackTime := startHostileSwing(t, latchAtkSpd)
	x, y, z := hostile.Position()
	dest := location.Location{X: x + 200, Y: y, Z: z}
	if !hostile.AI().AddMoveToDesire(dest, 50) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}

	srv.Advance(t, attackTime-10*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() inside the swing = %v, want %v", got, ai.IntentionAttack)
	}

	srv.Advance(t, 20*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() once the swing finished = %v, want %v", got, ai.IntentionMoveTo)
	}
	if got := hostile.AI().Desires().Len(); got != 2 {
		t.Fatalf("queued desires = %d, want the attack desire still queued behind the walk", got)
	}
	readUntil(t, srv.Client, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	if got := hostile.Move().Destination(); got.X != dest.X || got.Y != dest.Y {
		t.Fatalf("walk destination = %v, want %v", got, dest)
	}
}

// TestMoveToOverChaseDropsOffensiveFollow pins that the intention change
// drops the old attack's chase: a monster running after a player that takes
// up a heavier MOVE_TO walks to it and is not pulled back toward the player
// by the chase's follow re-checks.
func TestMoveToOverChaseDropsOffensiveFollow(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID).(attackable.Combatant)

	x, y, z := srv.PlayerPosition(t, objID)
	at := location.Location{X: x + 400, Y: y, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, gameservertest.MovingHostileTemplate("Monster"), at, at)
	drainUntilQuiet(t, c)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	walkInPlace(t, srv, hostile)
	drainUntilQuiet(t, c)

	hostile.AddDamageHate(victim, 0, 100)
	hostile.AddAttackDesire(victim, 5)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the attack desire = %v, want %v", got, ai.IntentionAttack)
	}
	if got := hostile.Move().FollowMode(); got != move.FollowOffensive {
		t.Fatalf("FollowMode() while chasing = %v, want offensive follow", got)
	}
	drainUntilQuiet(t, c)

	dest := location.Location{X: at.X + 300, Y: at.Y + 300, Z: at.Z}
	if !hostile.AI().AddMoveToDesire(dest, 50) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the heavier walk = %v, want %v", got, ai.IntentionMoveTo)
	}
	if got := hostile.Move().FollowMode(); got != move.FollowNone {
		t.Fatalf("FollowMode() after the walk took over = %v, want none", got)
	}

	for range time.Second / move.PositionUpdateInterval {
		srv.TickPositions()
	}
	if got := hostile.Move().Destination(); got.X != dest.X || got.Y != dest.Y {
		t.Fatalf("walk destination after a second of movement ticks = %v, want %v", got, dest)
	}
	if !hostile.Move().Moving() {
		t.Fatal("Moving() after a second of movement ticks = false, want the walk still under way")
	}
}
