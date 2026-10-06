package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// scriptDesireMonster boots a player and a monster standing off from it,
// past the monster's spawn cycle, and returns the monster's script handle and
// the player's.
func scriptDesireMonster(t *testing.T, dx, dy int) (*gameservertest.Server, *npc.Hostile, *script.NPC, *script.Player) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	player := livePlayer(t, srv, objID).(attackable.Combatant)

	x, y, z := srv.PlayerPosition(t, objID)
	at := location.Location{X: x + dx, Y: y + dy, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, gameservertest.MovingHostileTemplate("Monster"), at, at)
	drainUntilQuiet(t, c)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	walkInPlace(t, srv, hostile)
	drainUntilQuiet(t, c)
	return srv, hostile, script.NewNPC(hostile), script.PlayerOf(player)
}

// monsterFrames reads every frame until the server stays quiet and returns
// the ones whose leading object id is the monster's.
func monsterFrames(c *scriptedClient, hostile *npc.Hostile) [][]byte {
	var out [][]byte
	for _, f := range readFramesUntilQuiet(c) {
		if len(f) >= 5 && wireReader(f[1:]).ReadInt32() == hostile.ObjectID() {
			out = append(out, f)
		}
	}
	return out
}

// assertNoAttackFrames fails if frames hold a swing or a chase.
func assertNoAttackFrames(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeAttack, serverpackets.OpcodeMoveToPawn:
			t.Fatalf("%s: monster sent opcode %#x, want no attack or chase", what, f[0])
		}
	}
}

func tickMonster(t *testing.T, hostile *npc.Hostile) {
	t.Helper()
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
}

// TestScriptSocialDesireHoldsTheMonster pins a social desire a script
// queues: the next think broadcasts SocialAction with the animation id, and
// the monster takes up a heavier attack desire only once the social's timer
// has run out.
func TestScriptSocialDesireHoldsTheMonster(t *testing.T) {
	t.Parallel()
	srv, hostile, monster, player := scriptDesireMonster(t, 150, 0)
	c := srv.Client
	const socialTimer = 3 * time.Second

	monster.AddSocialDesire(2, int(socialTimer/time.Millisecond), 50)
	tickMonster(t, hostile)
	var social []byte
	for _, f := range monsterFrames(c, hostile) {
		if f[0] == serverpackets.OpcodeSocialAction {
			if social != nil {
				t.Fatal("SocialAction broadcast twice")
			}
			social = f
		}
	}
	if social == nil {
		t.Fatal("no SocialAction from the monster")
	}
	r := wireReader(social[1:])
	r.ReadInt32()
	if got := r.ReadInt32(); got != 2 {
		t.Fatalf("SocialAction id = %d, want 2", got)
	}

	monster.AddAttackDesire(player, 1000)
	tickMonster(t, hostile)
	assertNoAttackFrames(t, monsterFrames(c, hostile), "inside the social timer")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionSocial {
		t.Fatalf("CurrentIntention() inside the social timer = %v, want social", got)
	}

	srv.Advance(t, socialTimer)
	tickMonster(t, hostile)
	assertChasing(t, hostile, "after the social timer")
}

// assertChasing fails unless the monster attacks, running after its target.
func assertChasing(t *testing.T, hostile *npc.Hostile, what string) {
	t.Helper()
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() %s = %v, want attack", what, got)
	}
	if got := hostile.Move().FollowMode(); got != move.FollowOffensive {
		t.Fatalf("FollowMode() %s = %v, want the chase", what, got)
	}
}

// TestScriptFleeDesireHoldsTheMonster pins a flee desire a script queues:
// the next think switches the monster to run stance and walks it the flee
// distance straight away from the player; a heavier attack desire waits
// until the run ends, and the next think then takes it up.
func TestScriptFleeDesireHoldsTheMonster(t *testing.T) {
	t.Parallel()
	srv, hostile, monster, player := scriptDesireMonster(t, 100, 60)
	c := srv.Client
	const distance = 300
	px, py, _ := srv.PlayerPosition(t, srv.SoleObjectID(t))
	x, y, z := hostile.Position()
	from := location.Location{X: x, Y: y, Z: z}

	// The flee switches a walking monster to run stance.
	hostile.ForceWalkStance()
	drainUntilQuiet(t, c)

	monster.AddFleeDesire(player, distance, 1000)
	tickMonster(t, hostile)
	runAt, moveAt := -1, -1
	var dest location.Location
	for i, f := range monsterFrames(c, hostile) {
		switch f[0] {
		case serverpackets.OpcodeChangeMoveType:
			assertChangeMoveType(t, f, hostile.ObjectID(), true)
			runAt = i
		case serverpackets.OpcodeMoveToLocation:
			_, dest, _ = moveToLocationCoords(t, f)
			moveAt = i
		}
	}
	if moveAt < 0 || runAt < 0 || runAt > moveAt {
		t.Fatalf("ChangeMoveType(run) at %d, MoveToLocation at %d: want the run stance, then the flee", runAt, moveAt)
	}
	if want := from.FleeFrom(px, py, distance); dest.X != want.X || dest.Y != want.Y {
		t.Fatalf("flee destination = %+v, want %+v", dest, want)
	}

	monster.AddAttackDesire(player, 1_000_000)
	tickMonster(t, hostile)
	assertNoAttackFrames(t, monsterFrames(c, hostile), "while fleeing")
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionFlee {
		t.Fatalf("CurrentIntention() with a heavier attack queued = %v, want flee", got)
	}

	for i := 0; hostile.Move().Moving(); i++ {
		if i >= int(5*time.Second/move.PositionUpdateInterval) {
			t.Fatal("flee never arrived")
		}
		srv.TickPositions()
	}
	if got := hostile.AI().Desires().Len(); got != 1 {
		t.Fatalf("queued desires after the flee arrived = %d, want the attack only", got)
	}
	tickMonster(t, hostile)
	assertChasing(t, hostile, "after the flee arrived")
}

// TestScriptSocialDesireRefusedWhileAISleeps pins the social desire's sleep
// gate on a real monster: queued while its region is active, refused once
// the last player's logout puts the region to sleep, and refused for a dead
// monster.
func TestScriptSocialDesireRefusedWhileAISleeps(t *testing.T) {
	t.Run("inactive region", func(t *testing.T) {
		t.Parallel()
		srv, hostile, monster, _ := scriptDesireMonster(t, 150, 0)
		monster.AddSocialDesire(2, 1000, 50)
		if got := hostile.AI().Desires().Len(); got != 1 {
			t.Fatalf("queued desires in an active region = %d, want 1", got)
		}
		hostile.AI().Desires().Clear()

		srv.Client.Send(encodeLogout())
		srv.AdvanceUntil(t, "the region going to sleep", func() bool {
			_, active := srv.State.RegionActivity(hostile)
			return !active
		})
		monster.AddSocialDesire(2, 1000, 50)
		if got := hostile.AI().Desires().Len(); got != 0 {
			t.Fatalf("queued desires in an inactive region = %d, want 0", got)
		}
	})
	t.Run("dead", func(t *testing.T) {
		t.Parallel()
		srv, hostile, monster, player := scriptDesireMonster(t, 150, 0)
		killer := livePlayer(t, srv, player.ObjectID()).(attackable.Combatant)
		if !hostile.TakeDamage(int(hostile.MaxHP())+1, killer) {
			t.Fatal("TakeDamage() did not kill the monster")
		}
		monster.AddSocialDesire(2, 1000, 50)
		for _, d := range hostile.AI().Desires().Snapshot() {
			if d.Kind == ai.IntentionSocial {
				t.Fatal("social desire queued on a dead monster")
			}
		}
	})
}
