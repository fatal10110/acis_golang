package skills

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// holdZone makes c read its zones from a ledger holding only flag, as if it
// stood in a zone raising it.
func holdZone(c *player.Character, flag zone.Flag) {
	f := &zone.Flags{}
	f.Hold(flag, true)
	c.SetZoneStatus(f)
}
