// Package charmteleporter is the two gatekeepers that send a player to the
// Village of Gludin for a token or charm: Whirpy takes a dwarf gatekeeper
// token, Tamil an orc gatekeeper charm. A player without one gets the
// gatekeeper's refusal.
package charmteleporter

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// The gatekeepers.
const (
	whirpy = 30540
	tamil  = 30576
)

// The token and charm each gatekeeper takes.
const (
	orcGatekeeperCharm   = 1658
	dwarfGatekeeperToken = 1659
)

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir:   "teleport",
		Bind:  script.Bindings{script.EventTalked: {whirpy, tamil}},
		Hooks: script.Hooks{OnTalk: onTalk},
	}
}

// onTalk takes one token or charm and moves the player, else answers the
// gatekeeper's refusal.
func onTalk(s *script.Script, e script.Talk) string {
	htmltext := ""

	player := e.Player
	switch e.NPC.NpcID() {
	case whirpy:
		if player.HasItem(dwarfGatekeeperToken) {
			s.TakeItems(player, dwarfGatekeeperToken, 1)
			player.TeleportTo(script.Loc{X: -80826, Y: 149775, Z: -3043}, 0)
		} else {
			htmltext = "30540-01.htm"
		}
	case tamil:
		if player.HasItem(orcGatekeeperCharm) {
			s.TakeItems(player, orcGatekeeperCharm, 1)
			player.TeleportTo(script.Loc{X: -80826, Y: 149775, Z: -3043}, 0)
		} else {
			htmltext = "30576-01.htm"
		}
	}

	return htmltext
}
