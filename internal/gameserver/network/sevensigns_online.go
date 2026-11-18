package network

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// A GameClientLink reaches the players online as a Seven Signs period
// changes.
var _ sevensigns.Online = (*GameClientLink)(nil)

// GiveStrifeSkills implements sevensigns.Online: every player online gets the
// Seal of Strife skill its cabal earns, on its own queue, unstored and without
// a skill list; its changed maximum CP shows as any stat change does.
func (l *GameClientLink) GiveStrifeSkills(then func()) {
	if l.sevenSigns == nil {
		then()
		return
	}
	l.onEachLivePlayerThen(func(live *livePlayer) {
		l.giveStrifeSkill(live, l.sevenSigns.StrifeSkill(live.ObjectID()))
	}, then)
}

// RemoveStrifeSkills implements sevensigns.Online: every player online
// loses both Seal of Strife skills, unstored and without a skill list.
func (l *GameClientLink) RemoveStrifeSkills(then func()) {
	l.onEachLivePlayerThen(l.removeStrifeSkills, then)
}

// ExpelFromDungeons implements sevensigns.Online: every player in the world
// standing in a Seven Signs dungeon, game masters aside, whom the period change
// no longer allows there is sent to the nearest town and out of the dungeon. A
// player still logging in is left to its login check.
func (l *GameClientLink) ExpelFromDungeons(then func()) {
	if l.sevenSigns == nil {
		then()
		return
	}
	l.onEachLivePlayerThen(func(live *livePlayer) {
		if !l.liveInWorld(live) || !live.In7sDungeon() || live.accessLevel().IsGM {
			return
		}
		if l.sevenSigns.ExpelledAtPeriodChange(live.ObjectID()) {
			l.expelFromDungeon(live)
		}
	}, then)
}

// onEachLivePlayerThen runs fn on the queue of every player online, one
// player after the other: the task running fn for one posts the next, and
// the last runs then on its queue, so what fn sends goes out ahead of
// whatever then sends. A player whose queue has closed is skipped; with
// nobody left, then runs at once on the caller.
func (l *GameClientLink) onEachLivePlayerThen(fn func(*livePlayer), then func()) {
	var lives []*livePlayer
	if l.world != nil {
		for _, obj := range l.world.Players() {
			if live, ok := obj.(*livePlayer); ok {
				lives = append(lives, live)
			}
		}
	}
	eachLiveThen(lives, fn, then)
}

// eachLiveThen runs fn on the queue of each of lives in turn, then then.
func eachLiveThen(lives []*livePlayer, fn func(*livePlayer), then func()) {
	for i, live := range lives {
		rest := lives[i+1:]
		if postLive(live, func() {
			fn(live)
			eachLiveThen(rest, fn, then)
		}) {
			return
		}
	}
	then()
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
// the dungeon; a game master stays.
func (l *GameClientLink) enterWorldSevenSignsDungeon(live *livePlayer) {
	if l.sevenSigns == nil || !live.In7sDungeon() || live.accessLevel().IsGM {
		return
	}
	if l.sevenSigns.ExpelledAtLogin(live.ObjectID()) {
		l.expelFromDungeon(live)
	}
}

// expelFromDungeon sends live to the nearest town, then clears its Seven
// Signs dungeon membership, in that order.
func (l *GameClientLink) expelFromDungeon(live *livePlayer) {
	if dest, ok := l.restartDestination(live); ok {
		l.teleportLivePlayer(live, dest, restartTeleportOffset)
	}
	live.SetIn7sDungeon(false)
}
