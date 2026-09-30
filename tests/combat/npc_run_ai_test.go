package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// hitAnimationTail is how long an attack's hit animation outlasts its first
// hit landing.
const hitAnimationTail = 300 * time.Millisecond

// startHostileSwing boots a player next to a moving monster with atkSpd
// attack speed, has the monster run one periodic AI cycle and walk in place,
// then gives it a weak attack desire on the player over a lasting hate
// entry. The next periodic cycle promotes the desire, and the monster's
// first swing starts at once. Its last desire was a walk, so that swing is
// not the one-pass latched attack. It returns the monster and its swing's
// attack time.
func startHostileSwing(t *testing.T, atkSpd int) (*gameservertest.Server, *npc.Hostile, time.Duration) {
	t.Helper()
	srv, hostile, _, attackTime := startHostileAttack(t, atkSpd, 0)
	return srv, hostile, attackTime
}

// startHostileAttack is startHostileSwing for a monster holding the
// rightHand item (0 for none). It also returns the player it attacks.
func startHostileAttack(t *testing.T, atkSpd, rightHand int) (*gameservertest.Server, *npc.Hostile, attackable.Combatant, time.Duration) {
	t.Helper()
	return startHostileAttackAfter(t, atkSpd, rightHand, walkInPlace)
}

// hostilePrelude runs between a monster's spawn cycle and its first attack
// desire. It sets the desire the monster last executed, which decides
// whether its first attack is latched.
type hostilePrelude func(t *testing.T, srv *gameservertest.Server, hostile *npc.Hostile)

// walkInPlace runs a MOVE_TO desire to the monster's own position. It
// finishes in the same cycle with no packet, and leaves a walk as the last
// executed desire, so the next attack is not latched.
func walkInPlace(t *testing.T, _ *gameservertest.Server, hostile *npc.Hostile) {
	t.Helper()
	x, y, z := hostile.Position()
	if !hostile.AI().AddMoveToDesire(location.Location{X: x, Y: y, Z: z}, 1) {
		t.Fatal("AddMoveToDesire() to own position = false, want the walk queued")
	}
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("walk-in-place TickThink() error: %v", err)
	}
	if got := hostile.AI().Desires().Len(); got != 0 {
		t.Fatalf("queued desires after walking in place = %d, want 0", got)
	}
}

// startHostileAttackAfter is startHostileAttack with prelude run before the
// monster gets its attack desire.
func startHostileAttackAfter(t *testing.T, atkSpd, rightHand int, prelude hostilePrelude) (*gameservertest.Server, *npc.Hostile, attackable.Combatant, time.Duration) {
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
	tmpl.RightHand = rightHand
	// In reach, so the promoting pass swings at once: an arrival never
	// thinks, and a chase leg would leave the swing to a later cycle.
	tmpl.BaseAttackRange = 40
	at := location.Location{X: x + 20, Y: y, Z: z}
	hostile := srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
	drainUntilQuiet(t, c)

	if err := hostile.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	prelude(t, srv, hostile)
	drainUntilQuiet(t, c)
	hostile.AddDamageHate(victim, 0, 100)
	hostile.AddAttackDesire(victim, 5)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after promoting the attack desire = %v, want %v", got, ai.IntentionAttack)
	}
	readUntil(t, c, serverpackets.OpcodeAttack, "monster Attack")
	return srv, hostile, victim, time.Duration(formulas.TimeBetweenAttacks(hostile.AttackSpeed())) * time.Millisecond
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

// TestSwingFinishPromotesHostileDesireBeforeHitAnimationTimer pins
// CreatureAttack.onFinishedAttack: a fast monster whose swing finishes before
// its hit animation's 300ms timer closes the window as the swing finishes,
// so the runAI(false) that follows takes up its next desire right then.
func TestSwingFinishPromotesHostileDesireBeforeHitAnimationTimer(t *testing.T) {
	t.Parallel()
	srv, hostile, attackTime := startHostileSwing(t, 2000)
	hitAnimationEnd := attackTime/2 + hitAnimationTail
	if hitAnimationEnd <= attackTime+50*time.Millisecond {
		t.Fatalf("setup: hit animation ends at %v, want well after the swing finishes at %v", hitAnimationEnd, attackTime)
	}
	decayAttackDesire(t, hostile)
	x, y, z := hostile.Position()
	if !hostile.AI().AddMoveToDesire(location.Location{X: x + 200, Y: y, Z: z}, 50) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}

	srv.Advance(t, attackTime-10*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() before the swing finished = %v, want %v", got, ai.IntentionAttack)
	}
	srv.Advance(t, 20*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() once the swing finished inside the hit animation = %v, want %v", got, ai.IntentionMoveTo)
	}
	readUntil(t, srv.Client, serverpackets.OpcodeMoveToLocation, "MoveToLocation")
}

// bowItemID is the test item table's bow; its 1500ms reuse delay outlasts
// the hit animation.
const bowItemID = 14

// TestBowShotIdlesHostileWithNoDesire pins NpcAI.runAI(false) run from
// CreatureAttack.onFinishedAttackBow: a bow monster whose last desire decayed
// while it drew idles as its arrow lands, broadcasting its walk stance then
// rather than 300ms later, and the abort cancels its bow reuse so it can
// shoot again at once.
func TestBowShotIdlesHostileWithNoDesire(t *testing.T) {
	t.Parallel()
	srv, hostile, victim, attackTime := startHostileAttack(t, 300, bowItemID)
	if got := hostile.AttackType(); got != item.WeaponBow {
		t.Fatalf("setup: AttackType() = %v, want bow", got)
	}
	decayAttackDesire(t, hostile)

	srv.Advance(t, attackTime-10*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() while drawing = %v, want %v", got, ai.IntentionAttack)
	}
	srv.Advance(t, 20*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() once the arrow landed = %v, want %v", got, ai.IntentionIdle)
	}
	walk := readUntil(t, srv.Client, serverpackets.OpcodeChangeMoveType, "idle walk stance")
	assertChangeMoveType(t, walk[len(walk)-1], hostile.ObjectID(), false)

	// Still inside the shot's reuse delay: a fresh attack desire fires again
	// only if the idle abort cleared the bow cooldown.
	hostile.AddAttackDesire(victim, 1000)
	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	readUntil(t, srv.Client, serverpackets.OpcodeAttack, "second bow Attack inside the cancelled reuse")
}

// TestBowReuseEndContinuesHostileWithoutIdle pins
// AttackableAI.onEvtBowAttackReuse: a bow's reuse ending only THINKs, so a
// monster whose last desire decayed during the reuse delay does not run
// runAI's empty-queue idle abort then.
func TestBowReuseEndContinuesHostileWithoutIdle(t *testing.T) {
	t.Parallel()
	srv, hostile, _, attackTime := startHostileAttack(t, 300, bowItemID)
	reuse := 1500 * time.Millisecond * 345 / time.Duration(hostile.AttackSpeed())

	srv.Advance(t, attackTime+hitAnimationTail+10*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the shot's hit animation = %v, want %v", got, ai.IntentionAttack)
	}
	decayAttackDesire(t, hostile)
	drainUntilQuiet(t, srv.Client)

	srv.Advance(t, reuse-hitAnimationTail)
	for {
		frame := srv.Client.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			break
		}
		if frame[0] == serverpackets.OpcodeChangeMoveType {
			t.Fatal("ChangeMoveType at the bow reuse end, want no idle abort")
		}
	}
}
