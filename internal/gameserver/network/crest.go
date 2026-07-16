package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func (l *GameClientLink) framePledgeCrest(req clientpackets.RequestPledgeCrest) wire.Frame {
	data, _ := l.crests.Get(datacache.PledgeCrest, int(req.CrestID))
	return serverpackets.FramePledgeCrest(req.CrestID, data)
}

func (l *GameClientLink) frameAllyCrest(req clientpackets.RequestAllyCrest) (wire.Frame, bool) {
	data, ok := l.crests.Get(datacache.AllyCrest, int(req.CrestID))
	if !ok {
		return wire.Frame{}, false
	}
	return serverpackets.FrameAllyCrest(req.CrestID, data), true
}
