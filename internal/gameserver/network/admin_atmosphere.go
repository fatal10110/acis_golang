package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// redSkyDuration is how many seconds //atmosphere sky red keeps the sky red.
const redSkyDuration = 10

// adminAtmosphere answers //atmosphere <ssqinfo|sky> <state>: the named sky
// is shown to every player in the world, the game master included. An
// unknown type or state, or a missing argument, gets the two usage lines
// instead.
func (l *GameClientLink) adminAtmosphere(gm *livePlayer, line string) {
	build := atmosphereFrame(handleradmin.Args(line))
	if build == nil {
		sendText(gm, "Usage: //atmosphere <ssqinfo dawn|dusk|red|regular>")
		sendText(gm, "Usage: //atmosphere <sky day|night|red>")
		return
	}
	if l.world == nil {
		return
	}
	broadcastFrame(build, func(send func(frameReceiver)) {
		for _, p := range l.world.Players() {
			if live, ok := p.(*livePlayer); ok {
				send(live)
			}
		}
	})
}

// atmosphereFrame returns the builder of the sky packet args name, nil when
// they name none. Arguments past the state are ignored.
func atmosphereFrame(args []string) func() wire.Frame {
	if len(args) < 2 {
		return nil
	}
	switch kind, state := args[0], args[1]; kind {
	case "ssqinfo":
		var sky uint16
		switch state {
		case "dawn":
			sky = serverpackets.SSQSkyDawn
		case "dusk":
			sky = serverpackets.SSQSkyDusk
		case "red":
			sky = serverpackets.SSQSkyRed
		case "regular":
			sky = serverpackets.SSQSkyRegular
		default:
			return nil
		}
		return func() wire.Frame { return serverpackets.FrameSSQInfoSky(sky) }
	case "sky":
		switch state {
		case "night":
			return serverpackets.FrameSunSet
		case "day":
			return serverpackets.FrameSunRise
		case "red":
			return func() wire.Frame { return serverpackets.FrameExRedSky(redSkyDuration) }
		}
	}
	return nil
}
