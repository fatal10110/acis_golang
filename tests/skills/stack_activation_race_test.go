package skills

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestRacingPetrificationsLeaveTargetInvulnerable pins #5190 on a player:
// two petrifications of one stack group, cast by a monster, land at once.
// Whatever the
// interleaving, the stronger one ends up heading the group and the player
// stays invulnerable under it: a start of the weaker one that the stronger
// one overtook is undone before the stronger one starts. Removing the
// stronger one ends the invulnerability.
func TestRacingPetrificationsLeaveTargetInvulnerable(t *testing.T) {
	t.Parallel()
	p := bootAppearancePair(t)
	px, py, pz := p.srv.PlayerPosition(t, p.targetID)
	hostile := p.srv.SpawnHostileNPCAt(t, location.Location{X: px + 20, Y: py, Z: pz})
	invul := func() bool {
		var on bool
		p.onQueue(t, func() { on = p.target.(interface{ Invul() bool }).Invul() })
		return on
	}
	petrification := func(id modelskill.ID, order float64) *effect.Effect {
		e, err := effect.New(effect.Skill{ID: id, Level: 1, Debuff: true},
			modelskill.EffectTemplate{Name: "Petrification", Count: 1, Time: 30, StackType: "race_petrification", StackOrder: order})
		if err != nil {
			t.Fatalf("effect.New(Petrification): %v", err)
		}
		e.Effector, e.Effected = hostile, p.target
		return e
	}

	for i := range 50 {
		weak, strong := petrification(4101, 1), petrification(4102, 2)
		// Effects land from whichever goroutine runs their caster; two plain
		// goroutines stand for two casters' queues running at once.
		var wg sync.WaitGroup
		gate := make(chan struct{})
		for _, e := range []*effect.Effect{weak, strong} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				p.target.EffectList().Add(e)
			}()
		}
		close(gate)
		wg.Wait()

		if !strong.InUse() || weak.InUse() {
			t.Fatalf("race %d: weak/strong in use = %v/%v, want the stronger one alone active", i, weak.InUse(), strong.InUse())
		}
		if !invul() {
			t.Fatalf("race %d: player not invulnerable under the stronger petrification", i)
		}
		p.onQueue(t, func() {
			p.target.EffectList().Remove(strong)
			p.target.EffectList().Remove(weak)
		})
		if invul() {
			t.Fatalf("race %d: player still invulnerable once both petrifications left", i)
		}
	}
}
