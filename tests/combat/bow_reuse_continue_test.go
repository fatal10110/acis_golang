package combat

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestBowReuseEndShootsAgainWithoutDesire pins the continue pass a bow's
// reuse ending runs (AttackableAI.onEvtBowAttackReuse -> THINK ->
// CreatureAI.thinkAttack): it never reads the desire queue, so a monster
// whose attack desire decayed during the reuse delay shoots its current
// target again, with no idle abort. Only the next periodic cycle, which
// finds the queue empty, idles it.
func TestBowReuseEndShootsAgainWithoutDesire(t *testing.T) {
	t.Parallel()
	srv, hostile, victim, attackTime := startHostileAttack(t, 300, bowItemID)
	reuse := 1500 * time.Millisecond * 345 / time.Duration(hostile.AttackSpeed())

	srv.Advance(t, attackTime+hitAnimationTail+10*time.Millisecond)
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the shot's hit animation = %v, want %v", got, ai.IntentionAttack)
	}
	decayAttackDesire(t, hostile)
	drainUntilQuiet(t, srv.Client)

	srv.Advance(t, reuse-hitAnimationTail)
	for i := 0; ; i++ {
		if i == 100 {
			t.Fatal("no second bow Attack within 100 frames of the reuse end")
		}
		frame := srv.Client.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatal("no second bow Attack at the reuse end, want the monster to shoot again")
		}
		if frame[0] == serverpackets.OpcodeChangeMoveType {
			t.Fatal("ChangeMoveType at the bow reuse end, want no idle abort")
		}
		if frame[0] != serverpackets.OpcodeAttack || int32(binary.LittleEndian.Uint32(frame[1:5])) != hostile.ObjectID() {
			continue
		}
		if got := int32(binary.LittleEndian.Uint32(frame[5:9])); got != victim.ObjectID() {
			t.Fatalf("second bow Attack target = %d, want the player %d", got, victim.ObjectID())
		}
		break
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after the second shot = %v, want %v", got, ai.IntentionAttack)
	}
	for _, frame := range srv.Client.ReadQueued() {
		if frame[0] == serverpackets.OpcodeChangeMoveType {
			t.Fatal("ChangeMoveType after the second shot, want none before the next periodic cycle")
		}
	}

	if err := hostile.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionIdle {
		t.Fatalf("CurrentIntention() after the next periodic cycle = %v, want %v", got, ai.IntentionIdle)
	}
}
