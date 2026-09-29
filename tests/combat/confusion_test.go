package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestConfusedMonsterTurnsOnNearbyPlayer lands Confusion on a monster whose
// only neighbour within 1000 is a player. A playable is a confusion
// candidate, so the monster takes the player as an overriding attack
// desire; once the effect ends its hate against the player is dropped.
func TestConfusedMonsterTurnsOnNearbyPlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	playerID := srv.SoleObjectID(t)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)

	e, err := effect.New(effect.Skill{ID: 2, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Confusion", Count: 1, Time: 1, Icon: true})
	if err != nil {
		t.Fatalf("effect.New: %v", err)
	}
	e.Effector, e.Effected = hostile, hostile
	done := make(chan struct{})
	if !hostile.Queue().Post(func() { hostile.EffectList().Add(e); close(done) }) {
		t.Fatal("post confusion: queue closed")
	}
	<-done

	most, ok := hostile.AI().Threats().MostHated()
	if !ok || most.Attacker.ObjectID() != playerID {
		t.Fatalf("most hated after confusion = %+v (ok %v), want player %d", most, ok, playerID)
	}
	// The Integer.MAX_VALUE desire lands clamped to the threat table's
	// 999999999 hate ceiling.
	if most.Hate != 999999999 {
		t.Fatalf("confusion hate = %v, want the 999999999 ceiling", most.Hate)
	}
	if got := hostile.AI().CurrentIntention(); got != ai.IntentionAttack {
		t.Fatalf("CurrentIntention() after confusion = %v, want %v", got, ai.IntentionAttack)
	}

	srv.Advance(t, time.Second)
	srv.TickEffects()
	if e.InUse() {
		t.Fatal("confusion still in use after its single tick")
	}
	if got := hostile.AI().Threats().Hate(most.Attacker); got != 0 {
		t.Fatalf("hate against the player after confusion ended = %v, want 0", got)
	}
	drainUntilQuiet(t, c)
}
