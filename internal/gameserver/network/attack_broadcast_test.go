package network

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/rs/zerolog"
)

// captureOutboundRejects points the process-wide reporter at a fresh log for
// the rest of the test, with its rate limit cleared, and restores it after.
// It swaps only the atomic log and the mutex-guarded window, so it never
// races a concurrent reader of outboundRejects.
func captureOutboundRejects(t *testing.T) *rejectLog {
	t.Helper()
	r := &rejectLog{}
	prevLog := outboundRejects.log.Load()
	outboundRejects.mu.Lock()
	prevNext, prevSuppressed := outboundRejects.next, outboundRejects.suppressed
	outboundRejects.next, outboundRejects.suppressed = time.Time{}, 0
	outboundRejects.mu.Unlock()
	outboundRejects.setLog(zerolog.New(r))
	t.Cleanup(func() {
		outboundRejects.log.Store(prevLog)
		outboundRejects.mu.Lock()
		outboundRejects.next, outboundRejects.suppressed = prevNext, prevSuppressed
		outboundRejects.mu.Unlock()
	})
	return r
}

// TestBroadcastAttackGoesThroughSharedFanout pins the attack broadcast to the
// shared fan-out (#1549, #1819): a valid Attack reaches the attacker and each
// known observer with the same bytes, and an Attack whose frame cannot be
// built is reported once at the broadcast boundary with the builder's own
// cause and reaches no one, rather than each recipient getting an empty frame.
func TestBroadcastAttackGoesThroughSharedFanout(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		r := captureOutboundRejects(t)
		f := newDetachFilterFixture(t)
		snapshot := event.Attack{AttackerID: 1, Hits: []event.AttackHit{{TargetID: 2, Damage: 7}}}
		f.live.Emit(snapshot)

		want := serverpackets.FrameAttack(snapshot)
		defer want.Release()
		own, seen := f.ownFrames.Frames(), f.seenFrames.Frames()
		if len(own) != 1 || len(seen) != 1 {
			t.Fatalf("frames: attacker %d, observer %d, want 1 each", len(own), len(seen))
		}
		for name, got := range map[string][]byte{"attacker": own[0], "observer": seen[0]} {
			if !bytes.Equal(got, want.Bytes()[2:]) {
				t.Errorf("%s frame = %x, want %x", name, got, want.Bytes()[2:])
			}
		}
		if lines := r.lines(t); len(lines) != 0 {
			t.Fatalf("diagnostics = %+v, want none", lines)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		r := captureOutboundRejects(t)
		f := newDetachFilterFixture(t)
		// One hit more than the packet's uint16 extra-hit count can carry.
		snapshot := event.Attack{AttackerID: 1, Hits: make([]event.AttackHit, 1<<16+1)}
		cause := serverpackets.FrameAttack(snapshot).Err()
		if cause == nil {
			t.Fatal("oversized Attack built a valid frame")
		}
		f.live.Emit(snapshot)

		if got := f.effects(); got != noDetachFilterEffects {
			t.Fatalf("invalid Attack: %s, want none", got)
		}
		lines := r.lines(t)
		want := "wire: copy invalid frame: " + cause.Error()
		if len(lines) != 1 || lines[0].Boundary != "broadcast" || lines[0].Error != want {
			t.Fatalf("diagnostics = %+v, want one broadcast line with cause %q", lines, want)
		}
	})
}
