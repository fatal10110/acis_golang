package combat

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPvPFlagRefreshWhileBlinkingKeepsBlinkingUntilTheTick has a flagged
// player hit an innocent player again after its flag started blinking. The
// hit only moves the flag's deadline: the flag keeps blinking and the
// attacker reads no UserInfo until the next one-second tick turns it steady
// again from the refreshed deadline.
func TestPvPFlagRefreshWhileBlinkingKeepsBlinkingUntilTheTick(t *testing.T) {
	t.Parallel()
	var now atomic.Int64
	now.Store(time.UnixMilli(1_000).UnixNano())
	flags := task.NewPvPFlags(task.DefaultPvPFlagOptions(), func() time.Time { return time.Unix(0, now.Load()) })
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Attacker", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithPvPFlags(flags),
	)
	if !srv.DrivesClock() {
		t.Skip("holding the swings between the flag ticks needs the driven clock")
	}
	c, attackerID := srv.Client, srv.SoleObjectID(t)
	// A level 40 victim survives the hits: killing an innocent player
	// would make the attacker a PKer, which ends its PvP flag.
	victim := srv.SeedCharacterFor(t, "victim", "Victim", 40, 0)
	vc := srv.DialClient(t, "victim", 1)
	startInWorld(t, vc)
	startInWorld(t, c)
	drainUntilQuiet(t, vc)
	drainUntilQuiet(t, c)
	setPlayerRoll(t, srv, attackerID, 0)
	obj, ok := srv.State.Player(attackerID)
	if !ok {
		t.Fatal("attacker missing from world state")
	}
	attacker := obj.(interface{ PvPFlagState() task.PvPFlagState })

	selectPlayerTarget(t, c, victim.ID)
	c.Send(encodeAttackRequest(victim.ID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	srv.AdvanceUntil(t, "attacker PvP-flagged by its attack", func() bool {
		return attacker.PvPFlagState() == task.PvPFlagOn
	})

	// 36s into the 40s window the tick blinks the flag.
	now.Add(int64(36 * time.Second))
	flags.Tick()
	srv.Settle(t)
	if got := attacker.PvPFlagState(); got != task.PvPFlagBlinking {
		t.Fatalf("flag after the tick 4s before expiry = %v, want blinking", got)
	}
	drainUntilQuiet(t, c)

	// The victim is healed so the second hit cannot kill it, and the
	// attacker swings at it again.
	victimObj, ok := srv.State.Player(victim.ID)
	if !ok {
		t.Fatal("victim missing from world state")
	}
	healed := make(chan struct{})
	if !srv.PlayerQueue(t, victim.ID).Post(func() {
		defer close(healed)
		v := victimObj.(interface {
			MaxHPValue() float64
			SetHP(float64)
		})
		v.SetHP(v.MaxHPValue())
	}) {
		t.Fatal("victim queue closed")
	}
	<-healed
	drainUntilQuiet(t, vc)
	c.Send(encodeAttackRequest(victim.ID, int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	var after [][]byte
	srv.AdvanceUntil(t, "the next hit on the victim", func() bool {
		after = append(after, readQuiet(c)...)
		return frameIndex(after, serverpackets.OpcodeSystemMessage, serverpackets.SystemMessageYouDidS1Dmg) >= 0
	})
	srv.Settle(t)
	if got := attacker.PvPFlagState(); got != task.PvPFlagBlinking {
		t.Fatalf("flag after a hit refreshed it while blinking = %v, want still blinking", got)
	}
	if n := countByID(append(after, readQuiet(c)...), serverpackets.OpcodeUserInfo, -1); n != 0 {
		t.Fatalf("attacker UserInfo after the refreshing hit = %d, want 0", n)
	}

	flags.Tick()
	srv.Settle(t)
	if got := attacker.PvPFlagState(); got != task.PvPFlagOn {
		t.Fatalf("flag after the tick following the refresh = %v, want steady", got)
	}
	if n := countByID(readQuiet(c), serverpackets.OpcodeUserInfo, -1); n != 1 {
		t.Fatalf("attacker UserInfo after the tick = %d, want 1", n)
	}
}
