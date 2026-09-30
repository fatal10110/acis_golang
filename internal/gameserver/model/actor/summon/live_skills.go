package summon

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// SkillDisabled reports whether key is still waiting for its reuse delay.
// While any skill is tracked as disabled, a summon under AllSkillsDisabled
// reports every key disabled; with nothing tracked at all, that lock has no
// effect here.
func (a *Actor) SkillDisabled(key int32) bool {
	a.skillsMu.Lock()
	empty := len(a.disabledSkills) == 0
	a.skillsMu.Unlock()
	if empty {
		return false
	}
	// AllSkillsDisabled takes stateMu and the effect list's lock, so it runs
	// outside skillsMu to keep that lock a leaf.
	if a.AllSkillsDisabled() {
		return true
	}
	a.skillsMu.Lock()
	defer a.skillsMu.Unlock()
	expiresAt, ok := a.disabledSkills[key]
	if !ok {
		return false
	}
	if a.Now().Before(expiresAt) {
		return true
	}
	delete(a.disabledSkills, key)
	return false
}

// AllSkillsDisabled reports whether crowd control keeps the summon from
// using any skill: stun, sleep, paralysis (effect or transient lock), fear,
// or an immobile-until-attacked hold. See effectHeld for the effects it
// counts while AbortAll runs.
func (a *Actor) AllSkillsDisabled() bool {
	return a.paralyzedLock() || a.effectHeld(effect.AIDenyFlags)
}

// DisableSkill marks key unusable until delay elapses.
func (a *Actor) DisableSkill(key int32, delay time.Duration) {
	if delay <= 0 {
		return
	}
	a.skillsMu.Lock()
	defer a.skillsMu.Unlock()
	if a.disabledSkills == nil {
		a.disabledSkills = make(map[int32]time.Time)
	}
	a.disabledSkills[key] = a.Now().Add(delay)
}

// AddSkillReuse installs a summon-local item-skill reuse delay. Summons do
// not persist skill reuse timers, so ref is intentionally not stored.
func (a *Actor) AddSkillReuse(_ modelskill.Ref, key int32, delay time.Duration) {
	a.DisableSkill(key, delay)
}

// ShortBuffTaskSkillID returns zero because a summon has no item-window HUD.
func (*Actor) ShortBuffTaskSkillID() int32 { return 0 }
