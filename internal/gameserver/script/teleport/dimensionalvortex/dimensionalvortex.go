// Package dimensionalvortex is the three Dimensional Vortexes of the Tower
// of Insolence: each colour's link takes one stone of that colour and
// moves the player to its floor. A player without the stone gets the
// refusal.
package dimensionalvortex

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The stones the links take.
const (
	greenStone = 4401
	blueStone  = 4402
	redStone   = 4403
)

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir:   "teleport",
		Bind:  script.Bindings{script.EventTalked: {30952, 30953, 30954}},
		Hooks: script.Hooks{OnEvent: onAdvEvent},
	}
}

// onAdvEvent takes the stone of the colour the event names, ignoring case,
// and moves the player; any other event answers nothing.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	player := e.Player
	if strings.EqualFold(e.Name, "blue") {
		if player.HasItem(blueStone) {
			s.TakeItems(player, blueStone, 1)
			player.TeleportTo(script.Loc{X: 114097, Y: 19935, Z: 935}, 0)
		} else {
			htmltext = "no-items.htm"
		}
	} else if strings.EqualFold(e.Name, "green") {
		if player.HasItem(greenStone) {
			s.TakeItems(player, greenStone, 1)
			player.TeleportTo(script.Loc{X: 110930, Y: 15963, Z: -4378}, 0)
		} else {
			htmltext = "no-items.htm"
		}
	} else if strings.EqualFold(e.Name, "red") {
		if player.HasItem(redStone) {
			s.TakeItems(player, redStone, 1)
			player.TeleportTo(script.Loc{X: 118558, Y: 16659, Z: 5987}, 0)
		} else {
			htmltext = "no-items.htm"
		}
	}
	return htmltext
}
