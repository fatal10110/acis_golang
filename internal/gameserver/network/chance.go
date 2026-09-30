package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
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
// A player's or summon's proc set off by its own hit or cast (ownHit) runs
// on the queue of the player it acts for, so, like a regular cast's hit, it
// first runs the PvP flag changes pending for that player before each
// message and before the result: a PK kill the proc just made takes the
// player's items off and resets its flag before anything else the proc
// sends. A proc of the creature that was hit runs on its attacker's queue,
// not its own, and leaves them to the player's queue; every frame it sends
// that player waits behind them instead (sendBehindPvPChanges), so the
// player still reads the kill's side effects first.
func (l *GameClientLink) deliverChanceCast(caster handlerskill.Creature, ownHit bool, apply func(handlerskill.MessageSink) actorcast.EffectResult) {
	var live, actsFor *livePlayer
	var summonCaster *summon.Actor
	switch c := caster.(type) {
	case *livePlayer:
		live, actsFor = c, c
	case *player.Character:
		live, _ = l.livePlayerByID(c.ObjectID())
		actsFor = live
	case *summon.Actor:
		summonCaster = c
		actsFor, _ = liveSummonOwner(c)
	}
	settle := func() {}
	send := frameSender(sendFrameTo)
	switch {
	case ownHit && live != nil:
		settle = func() { l.settlePvPChanges(live) }
	case ownHit && summonCaster != nil:
		settle = func() { l.settleSummonOwnerPvPChanges(summonCaster) }
	case !ownHit && actsFor != nil:
		send = func(recipient *livePlayer, frame wire.Frame) {
			if recipient == actsFor {
				l.sendBehindPvPChanges(recipient, frame)
				return
			}
			recipient.SendFrame(frame)
		}
	}
	deliver := l.chanceResultTo(live, send)
	if summonCaster != nil {
		deliver = func(result actorcast.EffectResult) { l.sendSummonSkillResultVia(send, summonCaster, result) }
	}
	result := apply(func(message any) {
		settle()
		deliver(actorcast.EffectResult{Messages: []any{message}})
	})
	settle()
	deliver(result)
}

// chanceResultTo delivers a chance-triggered skill's result for the player
// caster live through send: the proc's own refusal and activation notices,
// then the skill-handler messages. With a nil live (an NPC caster, or a
// player offline) only the messages addressed to the players they name go
// out.
func (l *GameClientLink) chanceResultTo(live *livePlayer, send frameSender) func(actorcast.EffectResult) {
	return func(result actorcast.EffectResult) {
		if live != nil {
			for _, message := range result.Messages {
				switch m := message.(type) {
				case actorcast.ChanceConditionFailed:
					sendSkillConditionFailureVia(send, live, m.Clause, m.Skill.ID)
				case actorcast.ChanceWeaponNotAllowed:
					send(live, serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1CannotBeUsed, int32(m.Skill.ID), int32(m.Skill.Level)))
				case actorcast.WeaponSkillActivated:
					send(live, serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageS1HasBeenActivated, int32(m.Skill.ID), int32(m.Skill.Level)))
				}
			}
		}
		l.sendSkillHandlerResultVia(send, live, result)
	}
}
