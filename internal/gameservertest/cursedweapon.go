package gameservertest

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// cursedLink is the game client link's part the cursed weapon fixtures
// drive.
type cursedLink interface {
	DropCursedWeapon(killer *player.Character, dropperID int32, x, y, z int)
	TickCursedWeapons(now time.Time)
}

// TickCursedWeapons runs the cursed weapons' timers due by now, the way
// the production ticker does.
func (s *Server) TickCursedWeapons(now time.Time) {
	s.cursedLink.TickCursedWeapons(now)
}
