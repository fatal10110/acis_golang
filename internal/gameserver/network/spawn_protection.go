package network

import "github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"

const (
	spawnProtectionEnded = "The spawn protection has ended."
	spawnProtectionActed = "As you acted, you are no longer under spawn protection."
)

func (l *GameClientLink) activateSpawnProtection(live *livePlayer) {
	if live == nil || l.playerConfig.SpawnProtection <= 0 {
		return
	}
	live.spawnProtectionMu.Lock()
	if live.SpawnProtected() {
		live.spawnProtectionMu.Unlock()
		return
	}
	live.spawnProtectionGen++
	gen := live.spawnProtectionGen
	live.SetSpawnProtection(true)
	live.spawnProtectionMu.Unlock()
	live.UpdateUserInfo()
	l.scheduleAfter(l.playerConfig.SpawnProtection, func() {
		live.spawnProtectionMu.Lock()
		if gen != live.spawnProtectionGen || !live.SpawnProtected() {
			live.spawnProtectionMu.Unlock()
			return
		}
		live.spawnProtectionGen++
		live.SetSpawnProtection(false)
		live.spawnProtectionMu.Unlock()
		live.UpdateUserInfo()
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, spawnProtectionEnded))
	})
}

func (l *GameClientLink) clearSpawnProtectionOnAction(live *livePlayer) {
	if live == nil {
		return
	}
	live.spawnProtectionMu.Lock()
	if !live.SpawnProtected() {
		live.spawnProtectionMu.Unlock()
		return
	}
	live.spawnProtectionGen++
	live.SetSpawnProtection(false)
	live.spawnProtectionMu.Unlock()
	live.UpdateUserInfo()
	live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, spawnProtectionActed))
}
