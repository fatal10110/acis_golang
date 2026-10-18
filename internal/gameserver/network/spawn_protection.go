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
		// A running window keeps its timer; the player is still shown anew.
		l.refreshSpawnProtection(live)
		return
	}
	live.spawnProtectionGen++
	gen := live.spawnProtectionGen
	live.SetSpawnProtection(true)
	l.refreshSpawnProtection(live)
	live.after(protection, func() {
		sim.AssertOwner(live.Queue())
		if gen != live.spawnProtectionGen || !live.SpawnProtected() {
			return
		}
		live.spawnProtectionGen++
		live.SetSpawnProtection(false)
		l.refreshSpawnProtection(live)
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
	l.refreshSpawnProtection(live)
}

// refreshSpawnProtection shows live's protection change: its own UserInfo
// to live, while its session is attached, and its CharInfo to every player
// around it, so they see the blue team marker come and go.
func (l *GameClientLink) refreshSpawnProtection(live *livePlayer) {
	live.UpdateUserInfo()
	l.broadcastCharInfo(live, live.inventoryItems())
}

// spawnProtectedTeam reports whether live is drawn in the blue team: it is
// spawn protected and a protection window is configured.
func (l *GameClientLink) spawnProtectedTeam(live *livePlayer) bool {
	return l != nil && l.playerConfig().SpawnProtection > 0 && live.SpawnProtected()
}
