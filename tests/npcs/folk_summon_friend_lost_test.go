package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// folkSummonFriendSkill is a SUMMON_FRIEND cast on one target, with a
// short reach.
const folkSummonFriendSkill = modelskill.ID(9302)

// TestFolkDropsCastStepOnSummonFriendTargetOutOfTheWorld pins
// AbstractAI.isTargetLost(WorldObject, L2Skill) for a civilian NPC: the
// world-registration check comes before the SUMMON_FRIEND known-bypass, so
// a movable NPC whose SUMMON_FRIEND target logged out neither switches to
// its run stance nor walks toward it nor casts, while another player keeps
// its region awake.
func TestFolkDropsCastStepOnSummonFriendTargetOutOfTheWorld(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil, gameservertest.WithAITask())
	w.srv.SeedCharacterFor(t, "player2", "Watcher", playerLevel, 0)
	watcher := w.srv.DialClient(t, "player2", 1)
	startInWorld(t, w.srv, watcher)

	a := location.Location{X: w.at.X + 400, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 400, Y: w.at.Y + 300, Z: w.at.Z}
	f := w.spawnCastingWalker(t, a, modelskill.NewTable([]modelskill.Definition{{
		ID: folkSummonFriendSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, CastRange: 100, SkillType: "SUMMON_FRIEND",
		HitTime: 1500, StaticHitTime: true, StaticReuse: true,
	}}), route.WalkerLocation{Location: a, DelayMillis: int(nodeDelay / time.Millisecond)}, route.WalkerLocation{Location: b})
	w.srv.AdvanceUntil(t, "walker standing on its delayed node", standingOn(f, a))

	queueCastDesire(t, f, w.onlineCharacter(t), modelskill.Ref{ID: folkSummonFriendSkill, Level: 1}, outweighsRoute)
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	w.srv.AdvanceUntil(t, "caster's target out of the world", func() bool {
		_, ok := w.srv.State.Player(w.player)
		return !ok
	})
	drainUntilQuiet(t, watcher)

	for range 3 {
		if err := w.srv.AI.Tick(); err != nil {
			t.Fatalf("AI.Tick() = %v", err)
		}
		if frames := aboutFolk(drainFrames(t, watcher), f); len(frames) != 0 {
			t.Fatalf("frames about the walker after its target left = %#x, want none", npcOpcodes(frames))
		}
	}
	if f.Running() || f.IsMoving() || f.CastingNow() || folkAt(f) != a {
		t.Fatalf("walker running=%v moving=%v casting=%v at %v, want it idle on %v", f.Running(), f.IsMoving(), f.CastingNow(), folkAt(f), a)
	}
}
