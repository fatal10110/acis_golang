package player

import (
	"sync"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestSkillDisabledAndBuffCapCheckDoNotDeadlock races the two paths that
// cross the character's skill lock and its effect list lock. A reuse check
// with a skill on cooldown reads the effect list for the all-skills lock,
// while another actor landing a buff on the character asks it for its buff
// cap (Divine Inspiration's level) under the effect list's lock. Taking the
// skill lock around the first read deadlocked the two.
func TestSkillDisabledAndBuffCapCheckDoNotDeadlock(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.DisableSkill(2, time.Hour)

	const rounds = 2000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range rounds {
			c.SkillDisabled(1)
		}
	}()
	go func() {
		defer wg.Done()
		for range rounds {
			e, err := effect.New(effect.Skill{ID: 1, SkillType: "BUFF"}, modelskill.EffectTemplate{Name: "Buff"})
			if err != nil {
				t.Errorf("effect.New(Buff) error: %v", err)
				return
			}
			c.EffectList().Add(e)
			c.EffectList().Remove(e)
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("SkillDisabled and an effect-list buff insert did not finish: lock-order deadlock between skills.mu and the effect list's lock")
	}
}
