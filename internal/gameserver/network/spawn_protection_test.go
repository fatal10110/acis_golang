package network

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestSpawnProtectionExpiresAndActionCancelsTimer(t *testing.T) {
	frames := &testsupport.FrameCapture{}
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
	assertSystemMessageStringFrame(t, frames.Frames()[0], serverpackets.SystemMessageS1, spawnProtectionEnded)

	link.activateSpawnProtection(live)
	link.clearSpawnProtectionOnAction(live)
	if live.SpawnProtected() {
		t.Fatal("action did not clear spawn protection")
	}
	expire()
	if got := len(frames.Frames()); got != 2 {
		t.Fatalf("messages after cancelled expiry = %d, want 2", got)
	}
	assertSystemMessageStringFrame(t, frames.Frames()[1], serverpackets.SystemMessageS1, spawnProtectionActed)
}

func TestSpawnProtectionActionExemptions(t *testing.T) {
	for _, opcode := range []byte{
		clientpackets.OpcodeEnterWorld,
		clientpackets.OpcodeAction,
		clientpackets.OpcodeRequestPledgeCrest,
		clientpackets.OpcodeAppearing,
	} {
		if clearsSpawnProtection(opcode) {
			t.Fatalf("clearsSpawnProtection(%#x) = true, want false", opcode)
		}
	}
}
