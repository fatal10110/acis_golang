package network

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// A GameClientLink reaches the players online as a Seven Signs period
// changes.
var _ sevensigns.Online = (*GameClientLink)(nil)

// GiveStrifeSkills implements sevensigns.Online: every player online gets
// the Seal of Strife skill its cabal earns, on its own queue, unstored and
// without a skill list (SevenSignsManager.giveSosEffect); its changed
// maximum CP shows as any stat change does.
func (l *GameClientLink) GiveStrifeSkills() {
	if l.sevenSigns == nil {
		return
	}
	l.onEachLivePlayer(func(live *livePlayer) {
		l.giveStrifeSkill(live, l.sevenSigns.StrifeSkill(live.ObjectID()))
	})
}

// RemoveStrifeSkills implements sevensigns.Online: every player online
// loses both Seal of Strife skills, unstored and without a skill list
// (SevenSignsManager.removeSosEffect).
func (l *GameClientLink) RemoveStrifeSkills() {
	l.onEachLivePlayer(l.removeStrifeSkills)
}

// ExpelFromDungeons implements sevensigns.Online: every player in the world
// standing in a Seven Signs dungeon, game masters aside, whom the period
// change no longer allows there is sent to the nearest town and out of the
// dungeon (SevenSignsManager.teleLosingCabalFromDungeons). A player still
// logging in is left to its login check.
func (l *GameClientLink) ExpelFromDungeons() {
	if l.sevenSigns == nil {
		return
	}
	l.onEachLivePlayer(func(live *livePlayer) {
		if !l.liveInWorld(live) || !live.In7sDungeon() || live.accessLevel().IsGM {
			return
		}
		if l.sevenSigns.ExpelledAtPeriodChange(live.ObjectID()) {
			l.expelFromDungeon(live)
		}
	})
}

// onEachLivePlayer runs fn on the queue of every player online, one player
// after the other, each before the next starts, so what fn sends goes out
// ahead of whatever the caller sends next. A player whose queue has closed
// is skipped. It must not run on a player's queue.
func (l *GameClientLink) onEachLivePlayer(fn func(*livePlayer)) {
	if l.world == nil {
		return
	}
	for _, obj := range l.world.Players() {
		live, ok := obj.(*livePlayer)
		if !ok {
			continue
		}
		done := make(chan struct{})
		if postLive(live, func() {
			defer close(done)
			fn(live)
		}) {
			<-done
		}
	}
}

// giveStrifeSkill gives live the Seal of Strife skill skill names, unstored.
func (l *GameClientLink) giveStrifeSkill(live *livePlayer, skill sevensigns.StrifeSkill) {
	var ref modelskill.Ref
	switch skill {
	case sevensigns.VictorOfWar:
		ref = modelskill.TheVictorOfWar
	case sevensigns.VanquishedOfWar:
		ref = modelskill.TheVanquishedOfWar
	default:
		return
	}
	if l.skills == nil {
		return
	}
	if err := l.skills.GrantTransientSkills(live.Character, []modelskill.Ref{ref}); err != nil {
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("give seal of strife skill")
	}
}

// removeStrifeSkills takes both Seal of Strife skills from live, unstored.
func (l *GameClientLink) removeStrifeSkills(live *livePlayer) {
	if l.skills == nil {
		return
	}
	l.removeLiveSkill(live, int(modelskill.TheVictorOfWar.ID), false)
	l.removeLiveSkill(live, int(modelskill.TheVanquishedOfWar.ID), false)
}

// enterWorldStrifeSkills gives live, entering the world during seal
// validation with the Seal of Strife owned, the skill its cabal earns, and
// otherwise takes both away (EnterWorld, ahead of the spawn). The login
// burst's SkillList shows the result.
func (l *GameClientLink) enterWorldStrifeSkills(live *livePlayer) {
	if l.sevenSigns == nil {
		return
	}
	skill, ok := l.sevenSigns.EnterStrifeSkill(live.ObjectID())
	if !ok {
		l.removeStrifeSkills(live)
		return
	}
	l.giveStrifeSkill(live, skill)
}

// enterWorldSevenSignsDungeon sends live, entering the world in a Seven
// Signs dungeon it is no longer allowed in, to the nearest town and out of
// the dungeon; a game master stays (Player.onPlayerEnter).
func (l *GameClientLink) enterWorldSevenSignsDungeon(live *livePlayer) {
	if l.sevenSigns == nil || !live.In7sDungeon() || live.accessLevel().IsGM {
		return
	}
	if l.sevenSigns.ExpelledAtLogin(live.ObjectID()) {
		l.expelFromDungeon(live)
	}
}

// expelFromDungeon sends live to the nearest town, then clears its Seven
// Signs dungeon membership, in the reference's order.
func (l *GameClientLink) expelFromDungeon(live *livePlayer) {
	if dest, ok := l.restartDestination(live); ok {
		l.teleportLivePlayer(live, dest, restartTeleportOffset)
	}
	live.SetIn7sDungeon(false)
}
