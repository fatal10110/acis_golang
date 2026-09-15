package worldtest

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Player is a test-only world player marker.
type Player struct {
	world.Presence

	ID int32
}

// ObjectID returns the test player's object id.
func (p *Player) ObjectID() int32 { return p.ID }

// CharacterName returns an empty name; test players are not looked up by name.
func (p *Player) CharacterName() string { return "" }

// SpawnPlayer places a test player into state and returns it.
func SpawnPlayer(state *world.State, id int32, x, y, z int) *Player {
	player := &Player{ID: id}
	state.Spawn(player, x, y, z, 0)
	return player
}

// Kind reports KindPlayer.
func (p *Player) Kind() actor.Kind { return actor.KindPlayer }
