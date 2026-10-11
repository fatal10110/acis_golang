package skills

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The skill-finished probes: two quick casts and one slow enough to abort.
const (
	finishedFirst = modelskill.ID(9301)
	finishedNext  = modelskill.ID(9302)
	finishedSlow  = modelskill.ID(9303)
)

// TestNPCSkillFinishedHook (NpcCast.notifyCastFinishToAI): every cast end
// of a monster bound to a behavior raises the behavior's skill-finished
// hook once, with the skill and the cast's target: after the desire that
// drove the cast is dropped, and before the AI picks the next one. An
// aborted cast reports no success.
func TestNPCSkillFinishedHook(t *testing.T) {
	t.Parallel()
	const npcID = int32(20070)
	var (
		mu     sync.Mutex
		lines  []string
		caster *npc.Hostile
	)
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{npcID}, Hooks: script.Hooks{
			OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
				mu.Lock()
				h := caster
				mu.Unlock()
				var desires []modelskill.ID
				for _, d := range h.AI().Desires().Snapshot() {
					desires = append(desires, d.Skill.ID)
				}
				line := fmt.Sprintf("npc=%d skill=%d-%d target=%T:%d success=%v desires=%v casting=%v",
					e.NPC.ObjectID(), e.Skill.ID, e.Skill.Level, e.Creature, e.Creature.ObjectID(), e.Success, desires, h.CastingNow())
				mu.Lock()
				lines = append(lines, line)
				mu.Unlock()
			},
		}}
	}
	probe := func(id modelskill.ID, hitTime int) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive,
			Target: modelskill.TargetOne, Offensive: true, SkillType: "REAL_DAMAGE",
			CastRange: 900, HitTime: hitTime, StaticHitTime: true, StaticReuse: true, Power: 1,
		}
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{npcID: script.KindHostile},
			[]script.Listing{{Path: "ai.Recorder"}}, script.Catalog{"ai.Recorder": behavior}),
	)
	startInWorld(t, srv.Client)
	h, aiCtl := srv.SpawnCastingHostileNPC(t, realDamageCasterTemplate(int(npcID)), modelskill.NewTable([]modelskill.Definition{
		probe(finishedFirst, 500), probe(finishedNext, 500), probe(finishedSlow, 5000),
	}))
	mu.Lock()
	caster = h
	mu.Unlock()
	drainUntilQuiet(t, srv.Client)
	objID := srv.SoleObjectID(t)
	player := worldCombatant(t, srv, objID)
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(lines)
	}
	line := func(id modelskill.ID, success bool, desires []modelskill.ID) string {
		return fmt.Sprintf("npc=%d skill=%d-1 target=*script.Player:%d success=%v desires=%v casting=false", h.ObjectID(), id, objID, success, desires)
	}

	// Two cast desires: the heavier one is cast first; when it ends, its
	// desire is gone, the lighter one still waits and no cast has started.
	h.AI().AddCastDesire(player, modelskill.Ref{ID: finishedFirst, Level: 1}, 1000, false, true)
	h.AI().AddCastDesire(player, modelskill.Ref{ID: finishedNext, Level: 1}, 10, false, true)
	thinkOnNPCQueue(t, h)
	srv.AdvanceUntil(t, "both casts ending", func() bool { return count() >= 2 })

	// A slow cast stopped in flight.
	onNPCQueue(t, h, func() { aiCtl.Cast(player, modelskill.Ref{ID: finishedSlow, Level: 1}) })
	srv.Advance(t, time.Second)
	onNPCQueue(t, h, h.StopCast)
	srv.AdvanceUntil(t, "the aborted cast ending", func() bool { return count() >= 3 })
	srv.Advance(t, 5*time.Second)

	want := []string{
		line(finishedFirst, true, []modelskill.ID{finishedNext}),
		line(finishedNext, true, nil),
		line(finishedSlow, false, nil),
	}
	mu.Lock()
	got := slices.Clone(lines)
	mu.Unlock()
	if !slices.Equal(got, want) {
		t.Fatalf("skill-finished hooks =\n%q\nwant\n%q", got, want)
	}
}
