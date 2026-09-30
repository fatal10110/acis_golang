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
// happened), a summon's messages go to its owner, and an NPC's reach only
// the players they name. Each skill-handler message goes out the moment the
// handler produces it, so it keeps its place among the frames the skill's
// own state changes send at once (the target's status, death and kill
// rewards).
//
// A player's proc set off by its own hit or cast (ownHit) runs on its own
// queue, so, like a regular cast's hit, it first runs the PvP flag changes
// pending for the player before each message and before the result: a PK
// kill the proc just made takes its items off and resets its flag before
// anything else the proc sends. A proc of the player that was hit runs on
// its attacker's queue, not its own, and leaves them to the player's queue.
func (l *GameClientLink) deliverChanceCast(caster handlerskill.Creature, ownHit bool, apply func(handlerskill.MessageSink) actorcast.EffectResult) {
	var live *livePlayer
	var deliver func(actorcast.EffectResult)
	switch c := caster.(type) {
	case *livePlayer:
		live = c
		deliver = l.chanceResultTo(c)
	case *player.Character:
		live, _ = l.livePlayerByID(c.ObjectID())
		deliver = l.chanceResultTo(live)
	case *summon.Actor:
		deliver = func(result actorcast.EffectResult) { l.sendSummonSkillResult(c, result) }
	default:
		deliver = l.chanceResultTo(nil)
	}
	settle := func() {}
	if ownHit && live != nil {
		settle = func() { l.settlePvPChanges(live) }
	}
	result := apply(func(message any) {
		settle()
		deliver(actorcast.EffectResult{Messages: []any{message}})
	})
	settle()
	deliver(result)
}

// chanceResultTo delivers a chance-triggered skill's result for the player
// caster live: the proc's own refusal and activation notices, then the
// skill-handler messages. With a nil live (an NPC caster, or a player
// offline) only the messages addressed to the players they name go out.
func (l *GameClientLink) chanceResultTo(live *livePlayer) func(actorcast.EffectResult) {
	return func(result actorcast.EffectResult) {
		if live != nil {
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
		}
		l.sendSkillHandlerResult(live, result)
	}
}
