package script

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// CurrentTick returns the game tick: the whole seconds the server has been
// up. A script stores it to measure elapsed time against later.
func (s *Script) CurrentTick() int32 {
	return int32(s.env.Uptime() / time.Second)
}

// SevenSignsPeriod returns the active Seven Signs period.
func (s *Script) SevenSignsPeriod() sevensigns.Period {
	return s.env.SevenSigns.CurrentPeriod()
}

// SealOwner returns the cabal owning seal, sevensigns.NoCabal while none
// does or for a seal that is not contested.
func (s *Script) SealOwner(seal sevensigns.Seal) sevensigns.Cabal {
	owners := s.env.SevenSigns.SealOwners()
	for i, contested := range sevensigns.Seals {
		if contested == seal {
			return owners[i]
		}
	}
	return sevensigns.NoCabal
}

// Page returns the text of the datapack page at path, named from the
// server root ("data/html/..."), as stored; false when there is none. A
// hook returning it answers with that whole page.
func (s *Script) Page(path string) (string, bool) {
	if s.env.Page == nil {
		return "", false
	}
	return s.env.Page(path)
}
