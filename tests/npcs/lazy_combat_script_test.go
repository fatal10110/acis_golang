package npcs

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestIdleWizardTimerBuildsNoCombatParts spawns a monster bound to a
// Wizard-derived behavior nobody fights. Wizard's fixed-rate timer reads,
// refreshes and trims the hate list every 10 seconds from spawn; those
// periodic passes over an NPC that hates no one build none of its
// combat-only parts.
func TestIdleWizardTimerBuildsNoCombatParts(t *testing.T) {
	t.Parallel()
	var refreshes atomic.Int32
	leaf := func() script.Script {
		base := wizard.WizardHooks()
		return script.Script{Behavior: true, NPCs: []int32{lazyCombatMonsterID}, Hooks: base.With(script.Hooks{
			OnTimer: func(s *script.Script, e script.Timer) string {
				if strings.EqualFold(e.Name, "1002") {
					refreshes.Add(1)
				}
				return base.Timer(s, e)
			},
		})}
	}
	catalog := script.Catalog{
		"script.ai.individual.DefaultNpc":                       defaultnpc.New,
		"script.ai.individual.Monster.MonsterAI":                monster.New,
		"script.ai.individual.Monster.WizardBase.WizardBase":    wizard.Base,
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard": wizard.Wizard,
		"ai.LazyWizard": leaf,
	}
	var list []script.Listing
	for _, path := range []string{
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.WizardBase.WizardBase",
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard",
		"ai.LazyWizard",
	} {
		list = append(list, script.Listing{Path: path})
	}
	w, _ := lazyCombatWorld(t,
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{lazyCombatMonsterID: script.KindHostile}, list, catalog))

	n, err := w.srv.NpcSpawns.AddSpawn(lazyCombatMonsterID, w.at.X+100, w.at.Y, w.at.Z, 0, false, 0)
	if err != nil {
		t.Fatalf("spawn the monster: %v", err)
	}
	w.srv.Settle(t)
	h, ok := n.(*npc.Hostile)
	if !ok {
		t.Fatalf("spawned %T, want a hostile NPC", n)
	}
	assertNoCombatParts(t, h, "spawned")

	w.srv.Advance(t, 21*time.Second)
	w.srv.Settle(t)
	if got := refreshes.Load(); got < 2 {
		t.Fatalf("test setup: Wizard's refresh timer ran %d times in 21 s, want at least 2", got)
	}
	assertNoCombatParts(t, h, "after Wizard's refresh timer ran")
}
