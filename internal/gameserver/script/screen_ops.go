package script

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// ShowScreenMessage shows the player text at the top center of its screen
// for durationMs milliseconds.
func (p *Player) ShowScreenMessage(durationMs int32, text string) {
	p.character().ShowScreenMessage(text, durationMs)
}

// Earthquake shakes the player's screen around where it stands, with
// intensity, for duration seconds; no one else feels it.
func (p *Player) Earthquake(intensity, duration int32) {
	p.character().ShakeScreen(intensity, duration)
}

// Earthquake shakes the screens of the players that know the NPC, around
// where it stands, with intensity, for duration seconds. Only a hostile NPC
// does it; any other NPC panics.
func (n *NPC) Earthquake(intensity, duration int32) {
	n.hostile("an earthquake").ShakeScreens(intensity, duration)
}

// ShowHTML shows the player html in a dialog window through the NPC
// objectID (0 for none), then releases its client: a page a script read
// and filled in itself. The page's links become the ones the player may
// send back, and %objectId% reads objectID, as on every script page.
func (p *Player) ShowHTML(objectID int32, html string) {
	p.character().ShowDialogPage(objectID, "", html)
}

// ShowMultisell shows the player the multisell list name as the civilian
// NPC n offers it, every item of the list and not only those the player
// holds, with the tax of n's castle when the list applies it. A list n may
// not offer shows nothing. Any other NPC panics.
func (p *Player) ShowMultisell(n *NPC, name string) {
	if _, ok := n.combatant().(*npc.Folk); !ok {
		panic(fmt.Sprintf("script: a multisell of %T: only a civilian npc offers one", n.combatant()))
	}
	p.character().ShowMultisell(n.ObjectID(), name)
}
