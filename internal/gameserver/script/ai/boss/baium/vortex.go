package baium

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// bloodedFabric is what the Angelic Vortex takes for the way to Baium.
const bloodedFabric = 4295

// BaiumAngelicVortex is the Angelic Vortex, the way up to Baium's lair. It
// lets a player holding a Blooded Fabric through, taking one, while Baium
// lives and sleeps; it turns away a player on a wyvern, and answers by
// Baium's state otherwise.
func BaiumAngelicVortex() script.Script {
	return script.Script{
		Dir: dir,
		Bind: script.Bindings{
			script.EventFirstTalk: {31862},
			script.EventTalked:    {31862},
		},
		Hooks: script.Hooks{
			OnFirstTalk: func(_ *script.Script, _ script.FirstTalk) string {
				return "31862-00.htm"
			},
			OnEvent: vortexEvent,
		},
	}
}

// vortexEvent answers the story or runs the entry the event names, ignoring
// case; any other event answers nothing.
func vortexEvent(s *script.Script, e script.Event) string {
	htmltext := ""

	switch {
	case strings.EqualFold(e.Name, "baium_story"):
		htmltext = "31862-02.htm"
	case strings.EqualFold(e.Name, "enter"):
		c0 := s.GlobalMemo().Creature(memoKey)

		player := e.Player
		switch {
		case player.Flying():
			htmltext = "31862-05.htm"
		case c0 != nil && !c0.Dead():
			if c0.(*script.NPC).DBValue() == 0 {
				if player.HasItem(bloodedFabric) {
					s.TakeItems(player, bloodedFabric, 1)
					player.TeleportTo(script.Loc{X: 114077, Y: 15882, Z: 10078}, 0)
				} else {
					htmltext = "31862-03.htm"
				}
			} else {
				htmltext = "31862-01.htm"
			}
		default:
			htmltext = "31862-04.htm"
		}
	}

	return htmltext
}
