// Package elrokiteleporter is the two Elroki teleporters: Orahochin sends
// a player that is not in combat on, Gariachin sends a player back.
package elrokiteleporter

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// The teleporters.
const (
	orahochin = 32111
	gariachin = 32112
)

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir:   "teleport",
		Bind:  script.Bindings{script.EventTalked: {orahochin, gariachin}},
		Hooks: script.Hooks{OnTalk: onTalk},
	}
}

// onTalk moves the player; Orahochin refuses a player in combat.
func onTalk(_ *script.Script, e script.Talk) string {
	if e.NPC.NpcID() == orahochin {
		if e.Player.InCombat() {
			return "32111-no.htm"
		}

		e.Player.TeleportTo(script.Loc{X: 4990, Y: -1879, Z: -3178}, 0)
	} else {
		e.Player.TeleportTo(script.Loc{X: 7557, Y: -5513, Z: -3221}, 0)
	}

	return ""
}
