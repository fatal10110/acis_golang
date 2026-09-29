package network

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
)

// deliverChanceCast runs one chance-triggered skill and delivers its result
// the way a finished cast by the same caster delivers one: a player gets its
// messages and its own status change, a summon's messages go to its owner,
// and an NPC's reach only the players they name.
func (l *GameClientLink) deliverChanceCast(caster handlerskill.Creature, apply func() actorcast.EffectResult) {
	var live *livePlayer
	switch c := caster.(type) {
	case *livePlayer:
		live = c
	case *player.Character:
		live, _ = l.livePlayerByID(c.ObjectID())
	case *summon.Actor:
		l.sendSummonSkillResult(c, apply())
		return
	}
	if live == nil {
		l.sendSkillHandlerResult(nil, apply())
		return
	}
	before := live.Vitals()
	result := apply()
	for _, message := range result.Messages {
		if failed, ok := message.(actorcast.ChanceConditionFailed); ok {
			sendSkillConditionFailure(live, failed.Clause, failed.Skill.ID)
		}
	}
	l.sendSkillHandlerResult(live, result)
	sendMagicStatusUpdate(live, before)
}
