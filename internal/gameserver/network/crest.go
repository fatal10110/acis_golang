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
