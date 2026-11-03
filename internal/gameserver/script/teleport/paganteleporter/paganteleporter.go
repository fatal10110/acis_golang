// Package paganteleporter is the gatekeepers and mirrors of the Pagan
// Temple: the gatekeepers open the temple's doors, the inner ones only for
// a holder of the right mark, and Triol's mirrors move the player between
// the temple and the town.
package paganteleporter

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// The marks the inner gatekeepers check.
const (
	visitorMark = 8064
	paganMark   = 8067
)

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir:   "teleport",
		Bind:  script.Bindings{script.EventTalked: {32034, 32035, 32036, 32037, 32039, 32040}},
		Hooks: script.Hooks{OnTalk: onTalk},
	}
}

// onTalk opens the doors the gatekeeper keeps or moves the player. A door
// the world does not have aborts the hook.
func onTalk(s *script.Script, e script.Talk) string {
	htmltext := ""

	player := e.Player
	switch e.NPC.NpcID() {
	case 32034:
		if player.HasItem(visitorMark) || player.HasItem(paganMark) {
			s.DoorByID(19160001).Open()
			htmltext = "FadedMark.htm"
		} else {
			htmltext = "32034-1.htm"
		}

	case 32035:
		s.DoorByID(19160001).Open()
		htmltext = "FadedMark.htm"

	case 32036:
		if !player.HasItem(paganMark) {
			htmltext = "32036-1.htm"
		} else {
			s.DoorByID(19160010).Open()
			s.DoorByID(19160011).Open()
			htmltext = "32036-2.htm"
		}

	case 32037:
		s.DoorByID(19160010).Open()
		s.DoorByID(19160011).Open()
		htmltext = "FadedMark.htm"

	case 32039:
		player.TeleportTo(script.Loc{X: 34962, Y: -49758, Z: -763}, 0)

	case 32040:
		player.TeleportTo(script.Loc{X: -12766, Y: -35840, Z: -10856}, 0)
	}
	return htmltext
}
