package network

import (
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// deliverChanceCast runs one chance-triggered skill and delivers its result
// the way a finished cast by the same caster delivers one: a player gets its
// messages (a change to its own vitals reported its status where it
// happened), a summon's messages go to its owner,
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
	result := apply()
	for _, message := range result.Messages {
		switch m := message.(type) {
		case actorcast.ChanceConditionFailed:
			sendSkillConditionFailure(live, m.Clause, m.Skill.ID)
		case actorcast.ChanceWeaponNotAllowed:
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(m.Skill.ID), int32(m.Skill.Level)))
		case actorcast.WeaponSkillActivated:
			live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1HasBeenActivated, int32(m.Skill.ID), int32(m.Skill.Level)))
		}
	}
	l.sendSkillHandlerResult(live, result)
}
