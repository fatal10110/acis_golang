package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// gmHolder is a player that knows whether its access level is a game
// master's: the world tracks a player with its connection, which does.
type gmHolder interface{ IsGM() bool }

// IsGM reports whether the player plays under a game master's access
// level. A player the world tracks without its connection is none.
func (p *Player) IsGM() bool {
	g, ok := p.combatant().(gmHolder)
	return ok && g.IsGM()
}

// Visible reports whether the player stands in the world where others can
// see it: spawned and on the world grid, not taken off it by a teleport in
// flight.
func (p *Player) Visible() bool { return p.character().Visible() }

// HasEffect reports whether the player holds an effect of the skill
// skillID, in use or stacked out.
func (p *Player) HasEffect(skillID int32) bool {
	holder, ok := tracked(p).(effectHolder)
	if !ok {
		return false
	}
	_, held := holder.EffectList().ActiveBySkillID(int(skillID))
	return held
}

// ReduceAdena takes count adena from the player, and reports whether it
// did. A player short of count is told so and keeps its adena; a count
// below one is paid at once. A player leaving the world pays nothing and
// reports false.
func (p *Player) ReduceAdena(count int32) bool {
	return p.character().ReduceScriptAdena(int(count))
}

// ShowPage shows the player the datapack page file, named from the server
// root ("./data/html/..."), in a dialog window through the NPC objectID,
// then releases its client. The page's links become the ones the player
// may send back, and %objectId% reads objectID, as on every script page;
// none of the pages sent this way holds %objectId%, so the fill changes
// nothing.
func (p *Player) ShowPage(objectID int32, file string) {
	p.character().ShowDialogPage(objectID, file, "")
}

// PlayerCabal returns the Seven Signs cabal the player objectID signed up
// for, sevensigns.NoCabal when none.
func (s *Script) PlayerCabal(objectID int32) sevensigns.Cabal {
	return s.env.SevenSigns.PlayerCabal(objectID)
}

// WinningCabal returns the Seven Signs cabal ahead on total score,
// sevensigns.NoCabal on a tie.
func (s *Script) WinningCabal() sevensigns.Cabal { return s.env.SevenSigns.WinningCabal() }

// LosingCabal returns the Seven Signs cabal behind on total score,
// sevensigns.NoCabal on a tie.
func (s *Script) LosingCabal() sevensigns.Cabal { return s.env.SevenSigns.LosingCabal() }
