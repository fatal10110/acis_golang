package ai

import (
	"testing"
	"time"

	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// fearNPC puts a 300 s fear from the character on h, which leaves it free
// to move.
func (w *warriorWorld) fearNPC(t *testing.T, h *npc.Hostile) {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 9615, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Fear", Count: 1, Time: 300})
	if err != nil {
		t.Fatalf("effect.New(Fear): %v", err)
	}
	e.Effector, e.Effected = w.player.(effect.Actor), h
	w.onQueue(t, h, func() { h.EffectList().Add(e) })
	if !h.Afraid() || h.MovementDisabled() {
		t.Fatalf("setup: afraid %v, movement disabled %v, want afraid and free to move", h.Afraid(), h.MovementDisabled())
	}
}

// TestFearedNPCRefusesCastDesireWhileAnySkillWaits covers the all-skills
// lock in a hostile NPC's reuse check: with any skill tracked as waiting
// for its reuse, a feared NPC refuses a checked cast desire of another
// skill; without the fear, or with nothing tracked, it takes it. The lock
// is read before the skill's own wait, so the DD magic's own wait, expired
// but still tracked, refuses it under fear and not without.
func TestFearedNPCRefusesCastDesireWhileAnySkillWaits(t *testing.T) {
	t.Parallel()
	otherKey := actorcast.ReuseKey(modelskill.Definition{ID: wizardMuteID, Level: 1})
	ownKey := actorcast.ReuseKey(modelskill.Definition{ID: wizardDDID, Level: 1})
	for _, row := range []struct {
		name   string
		feared bool
		key    int32
		wait   time.Duration // key's reuse wait; 0 tracks none
		expire bool          // let the wait run out before the desire
		want   bool
	}{
		{"feared, other skill waiting", true, otherKey, time.Minute, false, false},
		{"not feared, other skill waiting", false, otherKey, time.Minute, false, true},
		{"feared, nothing waiting", true, 0, 0, false, true},
		{"feared, own wait expired but tracked", true, ownKey, time.Millisecond, true, false},
		{"not feared, own wait expired but tracked", false, ownKey, time.Millisecond, true, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w, h := bootWizard(t)
			if row.wait > 0 {
				w.onQueue(t, h, func() { h.DisableSkill(row.key, row.wait) })
			}
			if row.expire {
				w.srv.Advance(t, 10*time.Millisecond)
			}
			if row.feared {
				w.fearNPC(t, h)
			}

			var got bool
			w.onQueue(t, h, func() {
				h.AI().Desires().Clear()
				script.NPCOf(h).AddCastDesire(script.PlayerOf(w.player), modelskill.Ref{ID: wizardDDID, Level: 1}, 1000000)
				_, got = ddCastOn(h, w.player)
			})
			if got != row.want {
				t.Fatalf("DD magic cast desire taken = %v, want %v", got, row.want)
			}
		})
	}
}
