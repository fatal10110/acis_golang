package effect

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type spoilTarget struct {
	*liveEffectTarget
	pool *item.SpoilPool
}

func (t spoilTarget) SpoilPool() *item.SpoilPool { return t.pool }

// TestSpoilStartRequiresPlayerEffector pins that only a player spoils: a
// Spoil effect from any other effector fails to start and leaves the pool
// unmarked, whatever the success roll would have been.
func TestSpoilStartRequiresPlayerEffector(t *testing.T) {
	e, err := New(Skill{MagicLevel: 99}, modelskill.EffectTemplate{Name: "Spoil"})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	target := spoilTarget{liveEffectTarget: &liveEffectTarget{}, pool: &item.SpoilPool{}}
	e.Effector = &growEffectTarget{}
	e.Effected = target

	for range 50 {
		if e.OnStart(e) {
			t.Fatal("Spoil OnStart() from a non-player effector = true, want false")
		}
	}
	if target.pool.IsSpoiled() {
		t.Fatal("non-player effector marked the spoil pool")
	}
}
