package network

import (
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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
	// teleportModeCamera walks to the clicked point, while the position
	// the client reports is taken as is; see adminCamera.
	teleportModeCamera
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

// adminCamera answers //camera: gm's free camera turns on, gm going
// invisible, or off, gm visible again. CameraMode tells the client which,
// then gm is teleported where it stands. While the camera is on, the
// position gm's client reports is taken as is (adoptCameraPosition).
func (l *GameClientLink) adminCamera(gm *livePlayer, _ string) {
	if gm.teleportMode != teleportModeCamera {
		gm.teleportMode = teleportModeCamera
		gm.SetInvisible(true)
		gm.SendFrame(serverpackets.FrameCameraMode(serverpackets.CameraModeFirstPerson))
	} else {
		gm.teleportMode = teleportModeNone
		gm.SetInvisible(false)
		gm.SendFrame(serverpackets.FrameCameraMode(serverpackets.CameraModeThirdPerson))
	}
	l.teleportLivePlayer(gm, gm.CurrentLocation(), 0)
}

// adoptCameraPosition takes the position live's client reports under the
// free camera as live's own, the world around it following; the heading
// stays. A report outside the world changes nothing.
func (l *GameClientLink) adoptCameraPosition(live *livePlayer, reported location.Location) {
	if world.RegionKey(reported.X, reported.Y) == 0 {
		return
	}
	l.updateLivePlayerPosition(live, reported, live.CurrentHeading())
}

// adminRecall brings the named player to gm and drops gm's selection. A
// party recall brings every member of the named player's party, a clan
// recall every online member of its clan; out of a party or clan, the
// named player comes alone.
func (l *GameClientLink) adminRecall(gm *livePlayer, args []string) {
	if len(args) == 0 {
		gm.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageInvalidTarget))
		return
	}
	group := args[0]
	name := group
	if group == "clan" || group == "party" {
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
	for _, member := range l.recallGroup(group, target) {
		onPlayer(gm, member, func() { l.teleportLivePlayer(member, destination, 0) })
	}
	// The recalled player leaves gm's sight, so the selection goes with it.
	old := gm.Target()
	gm.StoreTarget(nil)
	l.announceTargetCleared(gm, old)
}

// recallGroup returns who a recall of target brings: target's party
// members for "party", the online members of its clan for "clan", else
// target alone.
func (l *GameClientLink) recallGroup(group string, target *livePlayer) []*livePlayer {
	switch group {
	case "party":
		if l.parties != nil {
			if view, ok := l.parties.View(target.ObjectID()); ok {
				return view.Members
			}
		}
	case "clan":
		if cl, ok := l.clanService().ClanOf(target.Character); ok {
			return l.onlineClanMembers(cl, 0)
		}
	}
	return []*livePlayer{target}
}

// adminSendHome sends the named player, else the selected one, else gm, to
// the nearest town, out of any Seven Signs dungeon.
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
		target.SetIn7sDungeon(false)
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
