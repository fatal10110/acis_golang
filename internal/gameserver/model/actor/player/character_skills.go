package player

import (
	"sync"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

type skillState struct {
	mu       sync.Mutex
	effects  []effect.ActiveEffect
	reuses   map[int32]skillReuse
	disabled map[int32]time.Time
}

type skillReuse struct {
	skill     modelskill.Ref
	key       int32
	delay     time.Duration
	expiresAt time.Time
}

// SkillSaveClassIndex returns the persisted class slot for this character's
// active skill state. Subclasses are not modeled yet, so every character
// currently uses the base class slot.
func (c *Character) SkillSaveClassIndex() int32 {
	return 0
}

// AddActiveSkillEffect records an active effect for logout persistence.
func (c *Character) AddActiveSkillEffect(e effect.ActiveEffect) {
	c.skills.mu.Lock()
	defer c.skills.mu.Unlock()
	c.skills.effects = append(c.skills.effects, e)
}

// ActiveSkillEffects returns a snapshot of effects eligible for logout
// persistence.
func (c *Character) ActiveSkillEffects() []effect.ActiveEffect {
	c.skills.mu.Lock()
	defer c.skills.mu.Unlock()
	return append([]effect.ActiveEffect(nil), c.skills.effects...)
}

// RestoreSkillEffect records one effect restored from persisted skill state.
func (c *Character) RestoreSkillEffect(plan effect.EffectPlan, reuseGroup int32) {
	c.AddActiveSkillEffect(effect.ActiveEffect{
		Skill:      plan.Skill,
		ReuseGroup: reuseGroup,
		Count:      plan.Count,
		Time:       plan.Time,
	})
}

// AddSkillReuse records a newly-started reuse timer and disables its reuse
// key until the delay elapses.
func (c *Character) AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration) {
	expiresAt := time.Now().Add(delay)
	c.SetSkillReuse(ref, key, delay, expiresAt)
	c.disableSkillUntil(key, expiresAt)
}

// SetSkillReuse records a reuse timer with an explicit expiration time.
func (c *Character) SetSkillReuse(ref modelskill.Ref, key int32, delay time.Duration, expiresAt time.Time) {
	if delay <= 0 {
		return
	}
	c.skills.mu.Lock()
	defer c.skills.mu.Unlock()
	if c.skills.reuses == nil {
		c.skills.reuses = make(map[int32]skillReuse)
	}
	c.skills.reuses[key] = skillReuse{skill: ref, key: key, delay: delay, expiresAt: expiresAt}
}

// RestoreSkillReuse records a persisted reuse timer and disables its reuse
// key until the stored expiration time.
func (c *Character) RestoreSkillReuse(ref modelskill.Ref, key int32, delay time.Duration, expiresAt time.Time) {
	c.SetSkillReuse(ref, key, delay, expiresAt)
	c.disableSkillUntil(key, expiresAt)
}

// SkillReuseTimers returns pending reuse timers as persistence rows need
// them, dropping timers that have already expired.
func (c *Character) SkillReuseTimers(now time.Time) []effect.ReuseTimer {
	c.skills.mu.Lock()
	defer c.skills.mu.Unlock()
	if len(c.skills.reuses) == 0 {
		return nil
	}
	timers := make([]effect.ReuseTimer, 0, len(c.skills.reuses))
	for key, reuse := range c.skills.reuses {
		if !reuse.expiresAt.After(now) {
			delete(c.skills.reuses, key)
			continue
		}
		timers = append(timers, effect.ReuseTimer{
			Skill:      reuse.skill,
			ReuseGroup: reuse.key,
			Delay:      reuse.delay.Milliseconds(),
			ExpiresAt:  reuse.expiresAt.UnixMilli(),
		})
	}
	return timers
}

// SkillDisabled reports whether key is still waiting for its reuse delay.
func (c *Character) SkillDisabled(key int32) bool {
	c.skills.mu.Lock()
	defer c.skills.mu.Unlock()
	expiresAt, ok := c.skills.disabled[key]
	if !ok {
		return false
	}
	if time.Now().Before(expiresAt) {
		return true
	}
	delete(c.skills.disabled, key)
	return false
}

// DisableSkill marks key unusable until delay elapses.
func (c *Character) DisableSkill(key int32, delay time.Duration) {
	if delay <= 0 {
		return
	}
	c.disableSkillUntil(key, time.Now().Add(delay))
}

func (c *Character) disableSkillUntil(key int32, expiresAt time.Time) {
	c.skills.mu.Lock()
	defer c.skills.mu.Unlock()
	if c.skills.disabled == nil {
		c.skills.disabled = make(map[int32]time.Time)
	}
	c.skills.disabled[key] = expiresAt
}
