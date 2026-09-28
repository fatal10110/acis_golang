package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// hitAnimationTail is how long an attack's hit animation outlasts its first
// hit landing.
const hitAnimationTail = 300 * time.Millisecond

// startHostileSwing boots a player next to a moving monster with atkSpd
// attack speed, has the monster run one periodic AI cycle, then gives it a
// weak attack desire on the player over a lasting hate entry. The next
// periodic cycle promotes the desire, and the monster's first swing starts
// at once. It returns the monster and its swing's attack time.
func startHostileSwing(t *testing.T, atkSpd int) (*gameservertest.Server, *npc.Hostile, time.Duration) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID).(attackable.Combatant)

	x, y, z := srv.PlayerPosition(t, objID)
	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.AtkSpd = float64(atkSpd)
	tmpl.PAtk = 0.25
	at := location.Location{X: x + 20, Y: y, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	drainUntilQuiet(t, c)

	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	hostile.AddDamageHate(victim, 0, 100)
	hostile.AddAttackDesire(victim, 5)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after promoting the attack desire = %v, want %v", got, ai.IntentionAttack)
	}
	readUntil(t, c, serverpackets.OpcodeAttack, "monster Attack")
	return srv, hostile, time.Duration(formulas.TimeBetweenAttacks(hostile.AttackSpeed())) * time.Millisecond
}

// decayAttackDesire runs one hate-decay step: the weak attack desire drops
// out of the queue while the hate entry keeps the monster out of peace.
func decayAttackDesire(t *testing.T, hostile *npc.Hostile) {
	t.Helper()
	for range 3 {
		hostile.Tick()
	}
	if got := hostile.AI().Desires().Len(); got != 0 {
		t.Fatalf("queued desires after decay = %d, want 0", got)
	}
}

// TestHitAnimationEndIdlesHostileWithNoDesireMidSwing pins NpcAI.runAI(false)
// run when the hit animation ends: a monster whose last desire decayed while
// it swings aborts the swing and idles right then, broadcasting its walk
// stance, instead of finishing the swing and waiting for its next periodic
// cycle.
func TestHitAnimationEndIdlesHostileWithNoDesireMidSwing(t *testing.T) {
	t.Parallel()
	srv, hostile, attackTime := startHostileSwing(t, 300)
	hitAnimationEnd := attackTime/2 + hitAnimationTail
	if hitAnimationEnd >= attackTime {
		t.Fatalf("setup: hit animation ends at %v, want before the swing finishes at %v", hitAnimationEnd, attackTime)
	}
	decayAttackDesire(t, hostile)

	srv.Advance(t, hitAnimationEnd-10*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() inside the hit animation = %v, want %v", got, ai.IntentionAttack)
	}
	srv.Advance(t, 20*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() once the hit animation ended mid-swing = %v, want %v", got, ai.IntentionIdle)
	}
	walk := readUntil(t, srv.Client, serverpackets.OpcodeChangeMoveType, "idle walk stance")
	assertChangeMoveType(t, walk[len(walk)-1], hostile.ObjectID(), false)
}

// TestHitAnimationHoldsHostileDesirePromotion pins NpcAI.runAI's hit
// animation gate: a fast monster whose swing finishes inside its own hit
// animation leaves its attack but does not take up its next desire until the
// hit animation ends.
func TestHitAnimationHoldsHostileDesirePromotion(t *testing.T) {
	t.Parallel()
	srv, hostile, attackTime := startHostileSwing(t, 1000)
	hitAnimationEnd := attackTime/2 + hitAnimationTail
	if hitAnimationEnd <= attackTime {
		t.Fatalf("setup: hit animation ends at %v, want after the swing finishes at %v", hitAnimationEnd, attackTime)
	}
	decayAttackDesire(t, hostile)
	x, y, z := hostile.Position()
	if !hostile.AI().AddMoveToDesire(location.Location{X: x + 200, Y: y, Z: z}, 50) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}

	srv.Advance(t, attackTime+(hitAnimationEnd-attackTime)/2)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after the swing finished inside the hit animation = %v, want %v", got, ai.IntentionIdle)
	}
	srv.Advance(t, hitAnimationEnd-attackTime)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() once the hit animation ended = %v, want %v", got, ai.IntentionMoveTo)
	}
	readUntil(t, srv.Client, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
}
