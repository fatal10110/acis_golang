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
	if live == nil || l.playerConfig.SpawnProtection <= 0 {
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
	live.after(l.playerConfig.SpawnProtection, func() {
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
	live.spawnProtectionGen++
	live.SetSpawnProtection(false)
	live.UpdateUserInfo()
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, spawnProtectionActed))
}
