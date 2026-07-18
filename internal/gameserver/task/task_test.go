package task

import "github.com/fatal10110/acis_golang/internal/gameserver/world"

type playerStub struct {
	world.Presence

	id int32
}

func (p *playerStub) ObjectID() int32 { return p.id }

func (p *playerStub) WorldPlayer() {}
