// Package elmoredencemeteryteleporter is the two gatekeepers of the
// Elmoreden cemetery that send a player to the Four Sepulchers or the
// Imperial Tomb. A player holding an antique brooch goes for free; one
// without takes a used grave pass; one with neither gets the gatekeeper's
// refusal.
package elmoredencemeteryteleporter

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The brooch and the pass.
const (
	usedGravePass = 7261
	antiqueBrooch = 7262
)

// fourSepulchersLoc is where the Four Sepulchers link lands.
func fourSepulchersLoc() script.Loc { return script.Loc{X: 178127, Y: -84435, Z: -7215} }

// imperialTombLoc is where the Imperial Tomb link lands.
func imperialTombLoc() script.Loc { return script.Loc{X: 186699, Y: -75915, Z: -2826} }

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir:   "teleport",
		Bind:  script.Bindings{script.EventTalked: {31919, 31920}},
		Hooks: script.Hooks{OnEvent: onAdvEvent},
	}
}

// onAdvEvent moves the player to the destination the event names, ignoring
// case; any other event answers nothing.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	player := e.Player
	if strings.EqualFold(e.Name, "4S") {
		if player.HasItem(antiqueBrooch) {
			player.TeleportTo(fourSepulchersLoc(), 0)
		} else if player.HasItem(usedGravePass) {
			s.TakeItems(player, usedGravePass, 1)
			player.TeleportTo(fourSepulchersLoc(), 0)
		} else {
			htmltext = strconv.Itoa(int(e.NPC.NpcID())) + "-1.htm"
		}
	} else if strings.EqualFold(e.Name, "IT") {
		if player.HasItem(antiqueBrooch) {
			player.TeleportTo(imperialTombLoc(), 0)
		} else if player.HasItem(usedGravePass) {
			s.TakeItems(player, usedGravePass, 1)
			player.TeleportTo(imperialTombLoc(), 0)
		} else {
			htmltext = strconv.Itoa(int(e.NPC.NpcID())) + "-1.htm"
		}
	}

	return htmltext
}
