package summon

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// SkillDisabled reports whether key is still waiting for its reuse delay.
func (a *Actor) SkillDisabled(key int32) bool {
	a.skillMu.Lock()
	defer a.skillMu.Unlock()
	expiresAt, ok := a.disabledSkills[key]
	if !ok {
		return false
	}
	if time.Now().Before(expiresAt) {
		return true
	}
	delete(a.disabledSkills, key)
	return false
}

// DisableSkill marks key unusable until delay elapses.
func (a *Actor) DisableSkill(key int32, delay time.Duration) {
	if delay <= 0 {
		return
	}
	a.skillMu.Lock()
	defer a.skillMu.Unlock()
	if a.disabledSkills == nil {
		a.disabledSkills = make(map[int32]time.Time)
	}
	a.disabledSkills[key] = time.Now().Add(delay)
}

// AddSkillReuse installs a summon-local item-skill reuse delay. Summons do
// not persist skill reuse timers, so ref is intentionally not stored.
func (a *Actor) AddSkillReuse(_ modelskill.Ref, key int32, delay time.Duration) {
	a.DisableSkill(key, delay)
}

// ShortBuffTaskSkillID returns zero because a summon has no item-window HUD.
func (*Actor) ShortBuffTaskSkillID() int32 { return 0 }
