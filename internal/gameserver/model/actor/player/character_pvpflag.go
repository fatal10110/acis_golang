package player

import "github.com/fatal10110/acis_golang/internal/gameserver/task"

var _ task.PvPFlagActor = (*Character)(nil)

// UpdatePvPFlag records the PvP flag state visible to this character's own
// client, mirroring Player.updatePvPFlag(int): a no-op when the state is
// unchanged, otherwise it persists the new state and resends UserInfo so
// the client redraws the name color/PvP icon. The reference also sends a
// RelationChanged packet for an owned summon and broadcasts the relation
// change to nearby observers on every change; neither summon messaging nor
// an observer relation/name-color broadcast exists in this port yet (see
// the #1249 tracking comment), so this only ever covers the self-only
// UserInfo refresh.
func (c *Character) UpdatePvPFlag(flag task.PvPFlagState) {
	c.stateMu.Lock()
	if c.pvpFlag == flag {
		c.stateMu.Unlock()
		return
	}
	c.pvpFlag = flag
	c.stateMu.Unlock()
	c.UpdateUserInfo()
}

// PvPFlagState returns this character's current PvP flag state.
func (c *Character) PvPFlagState() task.PvPFlagState {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.pvpFlag
}

// SetPvPFlagHook records the runtime hook that registers this character
// with the shared PvP flag tracker (task.PvPFlags.AddNormal/AddFlagged).
// notePvPHitFromAttacker calls it on this character once it has decided
// whether the normal or PvP-vs-PvP duration applies.
func (c *Character) SetPvPFlagHook(hook func(useFlaggedDuration bool)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.pvpFlagHook = hook
}

func (c *Character) flagPvP(useFlaggedDuration bool) {
	c.stateMu.RLock()
	hook := c.pvpFlagHook
	c.stateMu.RUnlock()
	if hook != nil {
		hook(useFlaggedDuration)
	}
}

// notePvPHitFromAttacker flags attacker with the PvP flag tracker after it
// damages c, mirroring Player.updatePvPStatus(Creature target)/
// Playable.checkIfPvP: a karma'd victim (c) never flags its attacker —
// hitting a PKer is PK territory, not PvP — and attacking oneself is a
// no-op. When it does flag, the shorter PvP-vs-PvP duration applies only
// when both sides are karma-free and c is already flagged (an ongoing PvP
// fight); otherwise the longer "engaging an innocent" duration applies.
// attacker resolves through *Character, so only a live player attacker can
// ever be flagged — an NPC/monster attacker (which never satisfies this
// assertion) is always a no-op, matching the reference's
// target.getActingPlayer() == null bail. The PvP-zone and duel exemptions,
// the miss-still-flags physical-attack timing, and the non-offensive-skill/
// NPC-target flag branches from the same reference method aren't ported
// here; see the #1249 follow-up (#1256) tracking those.
func (c *Character) notePvPHitFromAttacker(attacker any) {
	pk, ok := attacker.(*Character)
	if !ok || pk == c || c.KarmaPoints != 0 {
		return
	}
	useFlagged := pk.KarmaPoints == 0 && c.PvPFlagState() != task.PvPFlagNone
	pk.flagPvP(useFlagged)
}
