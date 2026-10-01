package network

import (
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// teleportMode is how a player's move clicks travel.
type teleportMode int32

const (
	// teleportModeNone walks to the clicked point.
	teleportModeNone teleportMode = iota
	// teleportModeOnce teleports to the next clicked point, then walks
	// again.
	teleportModeOnce
	// teleportModeAlways teleports to every clicked point.
	teleportModeAlways
)

// moveByTeleport carries out a move click under live's teleport mode,
// target already at head height: live jumps there instead of walking. It
// reports false, doing nothing, when live walks.
func (l *GameClientLink) moveByTeleport(live *livePlayer, target location.Location) bool {
	switch live.teleportMode {
	case teleportModeOnce:
		live.teleportMode = teleportModeNone
	case teleportModeAlways:
	default:
		return false
	}
	live.SendFrame(serverpackets.FrameActionFailed())
	l.teleportLivePlayer(live, target, 0)
	return true
}

// adminTeleport answers the teleport commands:
//
//	//tele                  the teleport panel
//	//teleport X Y [Z]      gm to those coordinates, Z the ground's when left out
//	//teleportto name       gm to that player
//	//recall [party|clan] name   that player to gm
//	//sendhome [name]       that player (the selected one, else gm) to the nearest town
//	//instant_move [0|1|2]  gm's move clicks walk, teleport once, or always teleport
func (l *GameClientLink) adminTeleport(gm *livePlayer, line string) {
	if line == "admin_tele" {
		l.sendAdminFile(gm, "teleports.htm")
		return
	}
	args := handleradmin.Args(line)
	switch handleradmin.Word(line) {
	case "admin_instant_move":
		l.adminInstantMove(gm, args)
	case "admin_recall":
		l.adminRecall(gm, args)
	case "admin_sendhome":
		l.adminSendHome(gm, args)
	case "admin_teleportto":
		if len(args) == 0 {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
			return
		}
		target, ok := l.livePlayerByName(args[0])
		if !ok {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
			return
		}
		l.teleportLivePlayer(gm, target.CurrentLocation(), 0)
	case "admin_teleport":
		l.adminTeleportTo(gm, args)
	}
}

func (l *GameClientLink) adminInstantMove(gm *livePlayer, args []string) {
	if len(args) == 0 {
		gm.teleportMode = teleportModeOnce
		return
	}
	mode, ok := parseJavaInt(args[0])
	if !ok || mode < int32(teleportModeNone) || mode > int32(teleportModeAlways) {
		sendText(gm, "Usage: //instant_move [0|1|2]")
		return
	}
	gm.teleportMode = teleportMode(mode)
}

// adminRecall brings the named player to gm and drops gm's selection. A
// party or clan recall brings the named player's group; while groups are
// not modeled, every player is alone in one.
func (l *GameClientLink) adminRecall(gm *livePlayer, args []string) {
	if len(args) == 0 {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	name := args[0]
	if name == "clan" || name == "party" {
		if len(args) < 2 {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
			return
		}
		name = args[1]
	}
	target, ok := l.livePlayerByName(name)
	if !ok {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	destination := gm.CurrentLocation()
	onPlayer(gm, target, func() { l.teleportLivePlayer(target, destination, 0) })
	// The recalled player leaves gm's sight, so the selection goes with it.
	old := gm.Target()
	gm.StoreTarget(nil)
	l.announceTargetCleared(gm, old)
}

// adminSendHome sends the named player, else the selected one, else gm, to
// the nearest town.
func (l *GameClientLink) adminSendHome(gm *livePlayer, args []string) {
	target := adminTargetPlayer(gm, true)
	if len(args) > 0 {
		named, ok := l.livePlayerByName(args[0])
		if !ok {
			gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
			return
		}
		target = named
	}
	onPlayer(gm, target, func() {
		if dest, ok := l.restartDestination(target); ok {
			l.teleportLivePlayer(target, dest, restartTeleportOffset)
		}
	})
}

// adminTeleportTo moves gm to X Y [Z]; coordinates that do not parse open
// the teleport panel instead.
func (l *GameClientLink) adminTeleportTo(gm *livePlayer, args []string) {
	var coords [3]int32
	n := min(len(args), 3)
	for i := range n {
		v, ok := parseJavaInt(args[i])
		if !ok {
			l.sendAdminFile(gm, "teleports.htm")
			return
		}
		coords[i] = v
	}
	if n < 2 {
		l.sendAdminFile(gm, "teleports.htm")
		return
	}
	target := location.Location{X: int(coords[0]), Y: int(coords[1]), Z: int(coords[2])}
	if n == 2 {
		target.Z = gm.CurrentLocation().Z
		if l.geo != nil {
			target.Z = int(l.geo.Height(target.X, target.Y, target.Z))
		}
	}
	l.teleportLivePlayer(gm, target, 0)
}
