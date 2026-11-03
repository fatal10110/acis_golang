package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestForcedAttackRerequestAfterWalkingOff: a player force-attacks an
// unflagged player, walks off once the swing has ended, and force-attacks
// the still-selected victim again. The second request is accepted: the
// attacker closes in and swings, and nothing before that swing answers
// ActionFailed.
func TestForcedAttackRerequestAfterWalkingOff(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	selectPlayerTarget(t, p.c, p.victimID)
	drainUntilQuiet(t, p.c)
	full := p.victimHealth(t)

	p.c.Send(encodeAttackRequest(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	p.srv.AdvanceUntil(t, "the first forced swing landing", func() bool { return p.victimHealth(t) < full })
	p.srv.Advance(t, 3*time.Second)
	drainUntilQuiet(t, p.c)

	x, y, _ := p.srv.PlayerPosition(t, p.attackerID)
	p.c.Send(encodeMoveBackwardToLocation(int32(x+200), int32(y), int32(playerOrigin.Z)))
	p.srv.AdvanceUntil(t, "the attacker walking off", func() bool {
		px, _, _ := p.srv.PlayerPosition(t, p.attackerID)
		return px == x+200
	})
	drainUntilQuiet(t, p.c)

	hit := p.victimHealth(t)
	p.c.Send(encodeAttackRequest(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	p.tickUntil(t, "the second forced swing landing", func() bool { return p.victimHealth(t) < hit })
	p.srv.Advance(t, 3*time.Second)

	frames := readQuiet(p.c)
	swing := frameIndex(frames, serverpackets.OpcodeAttack, 0)
	if swing < 0 || attacksBy(frames, p.attackerID) != 1 {
		t.Fatalf("second forced attack swings = %d, want 1: opcodes %v", attacksBy(frames, p.attackerID), opcodes(frames))
	}
	if i := frameIndex(frames[:swing], serverpackets.OpcodeActionFailed, 0); i >= 0 {
		t.Fatalf("second forced attack answered ActionFailed before its swing: opcodes %v", opcodes(frames))
	}
	if i := frameIndex(frames[:swing], serverpackets.OpcodeMoveToPawn, 0); i < 0 {
		t.Fatalf("second forced attack swung without closing in: opcodes %v", opcodes(frames))
	}
}

// TestForcedAttackRerequestMidSwingReplacesQueuedWalk: a walk and a second
// forced attack sent while the first swing is still in flight are each
// answered ActionFailed with no swing (after the walk's own ground-click
// ActionFailed), the attack replacing the walk queued
// behind the swing. The swing's end runs the queued attack, so the attacker
// swings again where it stands and never walks.
func TestForcedAttackRerequestMidSwingReplacesQueuedWalk(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	selectPlayerTarget(t, p.c, p.victimID)
	drainUntilQuiet(t, p.c)
	full := p.victimHealth(t)

	p.c.Send(encodeAttackRequest(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	p.srv.AdvanceUntil(t, "the first forced swing landing", func() bool { return p.victimHealth(t) < full })
	drainUntilQuiet(t, p.c)

	x, y, _ := p.srv.PlayerPosition(t, p.attackerID)
	p.c.Send(encodeMoveBackwardToLocation(int32(x+200), int32(y), int32(playerOrigin.Z)))
	p.c.Send(encodeAttackRequest(p.victimID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	p.srv.Settle(t)
	queued := readQuiet(p.c)
	// The ground click's own ActionFailed, then one for each request queued
	// behind the swing.
	if n := countOpcode(queued, len(queued), serverpackets.OpcodeActionFailed); len(queued) != 3 || n != 3 {
		t.Fatalf("mid-swing walk and attack answered %v, want three ActionFailed", opcodes(queued))
	}

	hit := p.victimHealth(t)
	p.srv.AdvanceUntil(t, "the queued forced swing landing", func() bool { return p.victimHealth(t) < hit })
	p.srv.Advance(t, 3*time.Second)
	frames := readQuiet(p.c)
	if n := attacksBy(frames, p.attackerID); n != 1 {
		t.Fatalf("queued forced attack swings = %d, want 1: opcodes %v", n, opcodes(frames))
	}
	if i := frameIndex(frames, serverpackets.OpcodeMoveToLocation, 0); i >= 0 {
		t.Fatalf("the walk the queued attack replaced still ran: opcodes %v", opcodes(frames))
	}
	if px, _, _ := p.srv.PlayerPosition(t, p.attackerID); px != x {
		t.Fatalf("attacker x = %d, want %d: the replaced walk moved it", px, x)
	}
}
