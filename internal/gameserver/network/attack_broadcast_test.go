package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestBroadcastAttackGoesThroughSharedFanout pins the attack broadcast to the
// shared fan-out (#1549, #1819): a valid Attack reaches the attacker and each
// known observer with the same bytes, and an Attack whose frame cannot be
// built is reported once at the broadcast boundary with the builder's own
// cause and reaches no one, rather than each recipient getting an empty frame.
func TestBroadcastAttackGoesThroughSharedFanout(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		r := newRejectLog()
		f := newDetachFilterFixture(t)
		f.live.link.rejects = r.reporter
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
		r := newRejectLog()
		f := newDetachFilterFixture(t)
		f.live.link.rejects = r.reporter
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
