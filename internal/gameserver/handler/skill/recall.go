package skill

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// recallCaster is the state a player casting a recall is refused on.
type recallCaster interface {
	InBossZone() bool
}

// recallTraveler is a player a recall may send away.
type recallTraveler interface {
	InBossZone() bool
	Jailed() bool
	InDuel() bool
	Riding() bool
	Flying() bool
	SetIn7sDungeon(in bool)
	TeleportTo(x, y, z, radius int)
	TeleportOnQueue(x, y, z, radius int)
	Recall(dest modelskill.RecallType)
	RecallOnQueue(dest modelskill.RecallType)
}

// recallScatter is the random offset around a recall's destination.
const recallScatter = 20

type recallHandler struct{}

func (recallHandler) Types() []string { return []string{"RECALL", "TELEPORT"} }

// Use sends each player target to the skill's fixed coordinates or, without
// them, to the town, castle or clan hall its recall type names. A player
// caster that is afraid, at an Olympiad match or in a boss zone recalls
// nobody and keeps its spiritshot. A target at the festival, jailed,
// duelling, riding or flying stays; so does one other than the caster at an
// Olympiad match or in a boss zone. Every player sent away leaves any Seven
// Signs dungeon. The caster's spiritshot is spent whoever moved. The cast
// runs on the caster's queue: the caster moves inside it, any other target
// on its own queue once the cast's task is done.
func (recallHandler) Use(cast Cast) {
	if p, ok := asPlayer(cast.Caster); ok {
		c, ok := p.(recallCaster)
		if !ok || p.Afraid() || p.OlympiadMode() || c.InBossZone() {
			return
		}
	}
	_, bsps := spiritshotCharges(cast.Caster)

	for _, obj := range cast.Targets {
		target, ok := asPlayer(obj)
		if !ok {
			continue
		}
		traveler, ok := target.(recallTraveler)
		if !ok {
			continue
		}
		if target.FestivalParticipant() || traveler.Jailed() || traveler.InDuel() || traveler.Riding() || traveler.Flying() {
			continue
		}
		self := cast.Caster != nil && target.ObjectID() == cast.Caster.ObjectID()
		if !self && (target.OlympiadMode() || traveler.InBossZone()) {
			continue
		}
		traveler.SetIn7sDungeon(false)
		switch at := cast.Skill.TeleCoords; {
		case at != nil && self:
			traveler.TeleportOnQueue(at.X, at.Y, at.Z, recallScatter)
		case at != nil:
			traveler.TeleportTo(at.X, at.Y, at.Z, recallScatter)
		case self:
			traveler.RecallOnQueue(cast.Skill.RecallType)
		default:
			traveler.Recall(cast.Skill.RecallType)
		}
	}

	writeSpiritshot(cast.Caster, bsps, cast.Skill.StaticReuse)
}
