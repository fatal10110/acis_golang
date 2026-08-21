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

// SetRelaxHPFullNotifier records the packet-layer hook for Relax ending at full HP.
func (c *Character) SetRelaxHPFullNotifier(send func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendRelaxHPFullNotice = send
}

// NotifyRelaxDeactivatedHPFull sends SKILL_DEACTIVATED_HP_FULL.
func (c *Character) NotifyRelaxDeactivatedHPFull(*effect.Effect) {
	c.stateMu.RLock()
	send := c.sendRelaxHPFullNotice
	c.stateMu.RUnlock()
	if send != nil {
		send()
	}
}

// SetSpoilNotifiers records packet-layer hooks for Spoil outcomes.
func (c *Character) SetSpoilNotifiers(already, success func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendSpoilAlreadyNotice, c.sendSpoilSuccessNotice = already, success
}

func (c *Character) NotifySpoilAlready() {
	c.stateMu.RLock()
	send := c.sendSpoilAlreadyNotice
	c.stateMu.RUnlock()
	if send != nil {
		send()
	}
}

func (c *Character) NotifySpoilSuccess() {
	c.stateMu.RLock()
	send := c.sendSpoilSuccessNotice
	c.stateMu.RUnlock()
	if send != nil {
		send()
	}
}
