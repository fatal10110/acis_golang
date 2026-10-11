package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// CameraShot is one cutscene camera move around an NPC: distance, yaw,
// pitch, time to reach the shot and its duration in milliseconds, turn,
// rise, widescreen (1 letterboxes it) and a last field passed as given.
type CameraShot = event.CameraShot

// SpecialCamera plays shot, centred on the NPC, for the players that know
// it, once each and in call order with the NPC's other broadcasts. A handle
// on nothing plays nothing.
func (n *NPC) SpecialCamera(shot CameraShot) {
	switch o := n.combatant().(type) {
	case *npc.Hostile:
		o.PlayCameraShot(shot)
	case *npc.Folk:
		o.PlayCameraShot(shot)
	}
}

// PlayScene plays the client's built-in scene for the player alone.
func (p *Player) PlayScene() { p.character().PlayScene() }
