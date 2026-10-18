package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const firstCycleBuff = modelskill.ID(9301)

// TestCastEndBeforeFirstCycleTakesUpNoQueuedWalk pins the first-cycle gate
// on a cast end's desire selection: a fresh monster with a cast and a walk
// queued takes the cast up at once on an attacker's attack desire; once that
// attack desire is gone, the cast's end takes up nothing before the
// monster's first periodic cycle, so the walk waits. The first cycle takes
// up nothing either; the next one sets off on the walk.
func TestCastEndBeforeFirstCycleTakesUpNoQueuedWalk(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	player := livePlayer(t, srv, objID).(attackable.Combatant)

	x, y, z := srv.PlayerPosition(t, objID)
	home := location.Location{X: x + 100, Y: y, Z: z}
	monster, _ := srv.SpawnMovingCastingHostileNPC(t, gameservertest.MovingHostileTemplate("Monster"), home, home,
		modelskill.NewTable([]modelskill.Definition{{
			ID: firstCycleBuff, Level: 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetSelf, SkillType: "BUFF",
			HitTime: 500, StaticHitTime: true, StaticReuse: true,
		}}))
	drainUntilQuiet(t, c)

	ref := modelskill.Ref{ID: firstCycleBuff, Level: 1}
	monster.AI().Desires().AddOrUpdate(&ai.Desire{
		Kind: ai.IntentionCast, FinalTarget: monster, Skill: ref,
		Weight: 1_000_000, QueuedAt: monster.Now(),
	})
	dest := location.Location{X: home.X + 200, Y: home.Y, Z: home.Z}
	if !monster.AI().AddMoveToDesire(dest, 50) {
		t.Fatal("AddMoveToDesire() = false, want the walk queued")
	}

	// The attack desire opens selection before the first cycle: the
	// heaviest desire, the cast, is taken up.
	monster.AddAttackDesire(player, 5)
	readUntil(t, c, serverpackets.OpcodeMagicSkillUse, "the monster's MagicSkillUse")
	if got := monster.AI().CurrentIntention(); got != ai.IntentionCast {
		t.Fatalf("CurrentIntention() after the attack desire = %v, want %v", got, ai.IntentionCast)
	}
	monster.AI().Desires().Remove(ai.IntentionAttack, player)

	srv.AdvanceUntil(t, "the monster's cast finishing", func() bool { return !monster.AI().CastController().CastingNow() })
	walk := &ai.Desire{Kind: ai.IntentionMoveTo, Location: dest}
	for _, frame := range c.ReadQueued() {
		if frame[0] == serverpackets.OpcodeMoveToLocation {
			t.Fatal("the cast end set off on the queued walk before the first cycle")
		}
	}
	if got := monster.AI().CurrentIntention(); got == ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the cast end = %v, want the walk left queued", got)
	}
	if !monster.AI().Desires().Has(walk) {
		t.Fatal("walk desire gone after the cast end, want it still queued")
	}

	if err := monster.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	if got := monster.AI().CurrentIntention(); got == ai.IntentionMoveTo {
		t.Fatal("the first cycle took up the walk, want it left for the next cycle")
	}
	if err := monster.TickThink(); err != nil {
		t.Fatalf("second TickThink() error: %v", err)
	}
	readUntil(t, c, serverpackets.OpcodeMoveToLocation, "the walk's MoveToLocation")
	if got := monster.AI().CurrentIntention(); got != ai.IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the second cycle = %v, want %v", got, ai.IntentionMoveTo)
	}
}
