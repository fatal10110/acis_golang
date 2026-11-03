package sevensigns

// DungeonEntry is a Seven Signs dungeon gatekeeper's verdict on a player
// asking to go down into a necropolis or a catacomb.
type DungeonEntry int

const (
	// DungeonAdmitted lets the player down.
	DungeonAdmitted DungeonEntry = iota
	// DungeonRefused turns the player away without a reason.
	DungeonRefused
	// DungeonDawnOnly and DungeonDuskOnly turn the player away during seal
	// validation, saying only the winning cabal, Dawn or Dusk, may enter.
	DungeonDawnOnly
	DungeonDuskOnly
)

// DungeonEntry judges the player r was read for asking to enter a dungeon
// governed by seal: the Seal of Avarice for a necropolis, the Seal of Gnosis
// for a catacomb. During seal validation a winning cabal admits only its own
// members, and only while it owns seal; after a tie any cabal member may
// enter. In the other periods any cabal member may enter. A player in no
// cabal is never admitted.
func (r Record) DungeonEntry(seal Seal) DungeonEntry {
	if r.Period == SealValidation {
		owner := NoCabal
		if i, ok := sealIndex(seal); ok {
			owner = r.Seals[i].Owner
		}
		switch r.Winner {
		case Dawn:
			if r.PlayerCabal != Dawn || owner != Dawn {
				return DungeonDawnOnly
			}
		case Dusk:
			if r.PlayerCabal != Dusk || owner != Dusk {
				return DungeonDuskOnly
			}
		}
	}
	if r.PlayerCabal == NoCabal {
		return DungeonRefused
	}
	return DungeonAdmitted
}
