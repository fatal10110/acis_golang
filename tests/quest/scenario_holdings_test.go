package quest

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// holdingsProbeID is the NPC id the holdings probe answers the first talk
// of.
const holdingsProbeID = 30002

// holdingsProbe answers its NPC's first talk with a page naming what the
// scenario's hold, equip, standing, pk, cabal and festival lines set up:
// the talker's Ketra/Varka standing, PK count, the count it holds of the
// level 3 Ketra mark (7213) and of adena, its active weapon, its Seven
// Signs cabal and the winning and losing cabals, so a scenario sees those
// lines' effect through packets.
func holdingsProbe() script.Script {
	return script.Script{
		Bind: script.Bindings{script.EventFirstTalk: {holdingsProbeID}},
		Hooks: script.Hooks{OnFirstTalk: func(s *script.Script, e script.FirstTalk) string {
			p := e.Player
			return fmt.Sprintf("<html><body>standing=%d pk=%d mark=%d adena=%d weapon=%d cabal=%s winning=%s losing=%s</body></html>",
				p.VarkaKetraAlliance(), p.PkKills(), p.ItemCount(7213), p.ItemCount(57), p.ActiveWeaponItemID(),
				s.PlayerCabal(p.ObjectID()), s.WinningCabal(), s.LosingCabal())
		}},
	}
}
