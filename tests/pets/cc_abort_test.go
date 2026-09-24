package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestStunnedPetStrikeLandsNothing stuns the pet while its strike is still
// casting. The stun aborts the cast, so the monster keeps its HP once the hit
// would have come due, and the pet's idle returns it to following its owner.
func TestStunnedPetStrikeLandsNothing(t *testing.T) {
	t.Parallel()
	h, petActor, hostile := bootWolfStriker(t)
	startWolfStrike(t, h)

	stun, err := effect.New(
		effect.Skill{ID: 101, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "Stun", Time: 30},
	)
	if err != nil {
		t.Fatalf("effect.New(Stun): %v", err)
	}
	stun.Effector, stun.Effected = petActor, petActor
	done := make(chan struct{})
	if !petActor.Queue().Post(func() { petActor.EffectList().Add(stun); close(done) }) {
		t.Fatal("post stun: queue closed")
	}
	<-done

	h.srv.Advance(t, wolfStrikeHitTime*time.Millisecond+600*time.Millisecond)
	drainUntilQuiet(t, h.client)
	if hp, full := hostile.HP(), float64(hostile.MaxHP()); hp != full {
		t.Fatalf("monster HP = %v after the pet was stunned mid-cast, want untouched %v", hp, full)
	}
	if got := petActor.Intent(); got != summon.IntentFollowOwner {
		t.Fatalf("pet intent = %v after stun, want follow-owner", got)
	}
}
