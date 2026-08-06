package player

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"

// SetLackHPNotifier records the packet-layer hook for a toggle DOT effect
// removed because its tick would exceed the target's remaining HP.
func (c *Character) SetLackHPNotifier(send func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendLackHPNotice = send
}

// NotifyEffectRemovedDueLackHP sends this player's SKILL_REMOVED_DUE_LACK_HP
// system message. e is unused: the reference message carries no skill-name
// parameter (EffectDamOverTime.java:32-36).
func (c *Character) NotifyEffectRemovedDueLackHP(*effect.Effect) {
	c.stateMu.RLock()
	send := c.sendLackHPNotice
	c.stateMu.RUnlock()
	if send != nil {
		send()
	}
}

// SetLackMPNotifier records the packet-layer hook for a toggle mana-DOT
// effect removed because its tick would exceed the target's remaining MP.
func (c *Character) SetLackMPNotifier(send func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendLackMPNotice = send
}

// NotifyEffectRemovedDueLackMP sends this player's SKILL_REMOVED_DUE_LACK_MP
// system message. e is unused: the reference message carries no skill-name
// parameter (EffectManaDamOverTime.java:29-33).
func (c *Character) NotifyEffectRemovedDueLackMP(*effect.Effect) {
	c.stateMu.RLock()
	send := c.sendLackMPNotice
	c.stateMu.RUnlock()
	if send != nil {
		send()
	}
}
