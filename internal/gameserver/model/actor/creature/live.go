package creature

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Live owns per-creature runtime state shared by live actor wrappers.
type Live struct {
	movement move.CreatureMove
}

// NewLive creates runtime state at origin with speed and geodata-bound
// movement validation.
func NewLive(origin location.Location, speed float64, geo move.Geo) (*Live, error) {
	live := &Live{}
	if err := live.movement.Init(origin, speed, geo); err != nil {
		return nil, err
	}
	return live, nil
}

// Move returns this live creature's lifetime movement state.
func (l *Live) Move() *move.CreatureMove {
	return &l.movement
}
