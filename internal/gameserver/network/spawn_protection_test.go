package network

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestSpawnProtectionExpiresAndActionCancelsTimer(t *testing.T) {
	frames := &frameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	var expire func()
	link := &GameClientLink{
		playerConfig: PlayerConfig{SpawnProtection: time.Second},
		afterFunc:    func(_ time.Duration, fn func()) { expire = fn },
	}

	link.activateSpawnProtection(live)
	if !live.SpawnProtected() || expire == nil {
		t.Fatal("spawn protection was not activated and scheduled")
	}
	expire()
	if live.SpawnProtected() {
		t.Fatal("spawn protection remained active after expiry")
	}
	assertSystemMessageStringFrame(t, frames.frames[0], serverpackets.SystemMessageS1, spawnProtectionEnded)

	link.activateSpawnProtection(live)
	link.clearSpawnProtectionOnAction(live)
	if live.SpawnProtected() {
		t.Fatal("action did not clear spawn protection")
	}
	expire()
	if got := len(frames.frames); got != 2 {
		t.Fatalf("messages after cancelled expiry = %d, want 2", got)
	}
	assertSystemMessageStringFrame(t, frames.frames[1], serverpackets.SystemMessageS1, spawnProtectionActed)
}
