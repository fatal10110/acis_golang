package script

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ClassLevel returns the tier of the class the player plays: 0 for a
// starting class, 1 to 3 for the first to third class change.
func (p *Player) ClassLevel() int32 {
	tier, _ := player.ClassLevel(p.character().ClassID())
	return int32(tier)
}

// IsNewbie reports whether the player is a newbie: at most one class
// change made and level 25 or lower, and, with checkLowLevel, level 6 or
// higher.
func (p *Player) IsNewbie(checkLowLevel bool) bool {
	c := p.character()
	tier, _ := player.ClassLevel(c.ClassID())
	level := c.Level()
	if tier > 1 || level > 25 {
		return false
	}
	return !checkLowLevel || level >= 6
}

// CursedWeaponEquipped reports whether the player wields a cursed weapon.
func (p *Player) CursedWeaponEquipped() bool { return p.character().CursedWeaponEquipped() }

// HasAtLeastOneItem reports whether the player's inventory holds an
// instance of any of itemIDs.
func (p *Player) HasAtLeastOneItem(itemIDs ...int32) bool {
	for _, id := range itemIDs {
		if p.HasItem(id) {
			return true
		}
	}
	return false
}

// TeleportTo moves the player to at, up to offset away on each axis. The
// teleport runs on the player's own queue once the calling task is done,
// whatever queue the hook runs on: the rest of the hook still reads the old
// position.
func (p *Player) TeleportTo(at Place, offset int) {
	loc := at.loc()
	p.character().TeleportTo(loc.X, loc.Y, loc.Z, offset)
}

// After runs fn on the player's queue once d has passed. Nothing runs for
// a player leaving the world: its queue drops what is still pending.
func (p *Player) After(d time.Duration, fn func()) {
	pl, ok := p.combatant().(timerPlayer)
	if !ok || pl.Detaching() {
		return
	}
	pl.Queue().After(max(d, 0), fn)
}

// CallSkill makes caster use ref on target: everyone watching caster sees
// the cast, and the skill's effects land on target with no cast time, cost,
// reuse or target check. The cast runs on caster's own queue once the
// calling task is done, whatever queue the hook runs on: the rest of the
// hook still reads the state from before the cast.
func (s *Script) CallSkill(caster, target *Player, ref skill.Ref) {
	caster.character().CallSkill(target.ObjectID(), ref)
}
