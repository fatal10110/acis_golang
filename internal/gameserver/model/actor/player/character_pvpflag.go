package player

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

var _ task.PvPFlagActor = (*Character)(nil)

// UpdatePvPFlag records the PvP flag state visible to this character's own
// client, mirroring Player.updatePvPFlag(int): a no-op when the state is
// unchanged, otherwise it persists the new state, resends UserInfo so the
// client redraws the name color/PvP icon, sends an owned summon a
// self-view RelationChanged, and broadcasts the relation change to nearby
// observers.
func (c *Character) UpdatePvPFlag(flag task.PvPFlagState) {
	c.stateMu.Lock()
	if c.pvpFlag == flag {
		c.stateMu.Unlock()
		return
	}
	c.pvpFlag = flag
	c.stateMu.Unlock()
	c.UpdateUserInfo()
	c.BroadcastRelations()
}

// PvPFlagState returns this character's current PvP flag state.
func (c *Character) PvPFlagState() task.PvPFlagState {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.pvpFlag
}

func (c *Character) flagPvP(useFlaggedDuration bool) {
	c.emit(event.PvPFlagged{UseFlaggedDuration: useFlaggedDuration})
}

// NotePvPAttack records one resolved physical attack against target.
func (c *Character) NotePvPAttack(target attackable.Combatant) {
	if victim := pvpTargetPlayer(target); victim != nil {
		victim.notePvPHitFromAttacker(c)
	}
}

// NotePvPSkillTargets records a resolved skill cast against targets.
func (c *Character) NotePvPSkillTargets(targets []attackable.Combatant, offensive bool, skillType string) {
	for _, target := range targets {
		if offensive {
			c.NotePvPAttack(target)
			continue
		}
		if c.skillTargetFlagsPvP(target, skillType) {
			c.flagPvP(false)
		}
	}
}

func (c *Character) skillTargetFlagsPvP(target attackable.Combatant, skillType string) bool {
	if c.InPvPZone() {
		return false
	}
	if victim := pvpTargetPlayer(target); victim != nil {
		return victim != c && (victim.PvPFlagState() != task.PvPFlagNone || victim.Karma() > 0)
	}
	if skillType == "SUMMON" || skillType == "BEAST_FEED" || skillType == "UNLOCK" || skillType == "UNLOCK_SPECIAL" || skillType == "DELUXE_KEY_UNLOCK" {
		return false
	}
	return target.Kind() == actor.KindNPC && !target.Guard()
}

func pvpTargetPlayer(target attackable.Combatant) *Character {
	if target != nil && target.Kind() == actor.KindSummon {
		target, _ = target.Owner()
	}
	player, _ := target.(*Character)
	return player
}

// notePvPHitFromAttacker flags attacker with the PvP flag tracker after a
// resolved physical or offensive-skill attack, mirroring Player.updatePvPStatus(Creature target)/
// Playable.checkIfPvP: a karma'd victim (c) never flags its attacker —
// hitting a PKer is PK territory, not PvP — and attacking oneself is a
// no-op. When it does flag, the shorter PvP-vs-PvP duration applies only
// when both sides are karma-free and c is already flagged (an ongoing PvP
// fight); otherwise the longer "engaging an innocent" duration applies.
// attacker resolves through *Character, so only a live player attacker can
// ever be flagged — an NPC/monster attacker (which never satisfies this
// assertion) is always a no-op, matching the reference's
// target.getActingPlayer() == null bail.
func (c *Character) notePvPHitFromAttacker(attacker any) {
	pk, ok := attacker.(*Character)
	if !ok || pk == c || c.Karma() != 0 {
		return
	}
	if pk.InPvPZone() && c.InPvPZone() {
		return
	}
	useFlagged := pk.Karma() == 0 && c.PvPFlagState() != task.PvPFlagNone
	pk.flagPvP(useFlagged)
}
