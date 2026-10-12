package network

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

const (
	spawnProtectionEnded = "The spawn protection has ended."
	spawnProtectionActed = "As you acted, you are no longer under spawn protection."
)

func (l *GameClientLink) activateSpawnProtection(live *livePlayer) {
	protection := l.playerConfig().SpawnProtection
	if live == nil || protection <= 0 {
		return
	}
	sim.AssertOwner(live.Queue())
	if live.SpawnProtected() {
		return
	}
	live.spawnProtectionGen++
	gen := live.spawnProtectionGen
	live.SetSpawnProtection(true)
	live.UpdateUserInfo()
	live.after(protection, func() {
		sim.AssertOwner(live.Queue())
		if gen != live.spawnProtectionGen || !live.SpawnProtected() {
			return
		}
		live.spawnProtectionGen++
		live.SetSpawnProtection(false)
		live.UpdateUserInfo()
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, spawnProtectionEnded))
	})
}

func (l *GameClientLink) clearSpawnProtectionOnAction(live *livePlayer) {
	if live == nil {
		return
	}
	sim.AssertOwner(live.Queue())
	if !live.SpawnProtected() {
		return
	}
	// The acted notice goes out ahead of the status that drops the
	// protection; the timer's end reverses the two.
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, spawnProtectionActed))
	live.spawnProtectionGen++
	live.SetSpawnProtection(false)
	live.UpdateUserInfo()
}
