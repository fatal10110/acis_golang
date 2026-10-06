package ai

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/testsupport/scenario"
)

// probeID is the NPC id the runner's probe behavior is bound to.
const probeID = 20120

// probe is a behavior that answers its NPC's attacked and dying hooks with
// a sound to the attacker or killer, naming the hook and the damage, so a
// scenario sees through packets which hooks its hit and kill steps raised.
// It stands in for the behavior scripts until the first one is ported.
func probe() script.Script {
	sound := func(s *script.Script, c script.Creature, name string) {
		if p, ok := c.(*script.Player); ok {
			s.PlaySound(p, name)
		}
	}
	return script.Script{Behavior: true, NPCs: []int32{probeID}, Hooks: script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			sound(s, e.Attacker, fmt.Sprintf("Probe.attacked_%d", e.Damage))
		},
		OnMyDying: func(s *script.Script, e script.MyDying) { sound(s, e.Killer, "Probe.dying") },
	}}
}

// scenarioScripts are the scripts an NPC behavior scenario may list, by
// scripts.xml path.
var scenarioScripts = script.Catalog{
	"ai.Probe": probe,
}

// TestScenarios plays every NPC behavior scenario of testdata/scenarios
// through the booted server and MariaDB (format: package scenario).
func TestScenarios(t *testing.T) {
	t.Parallel()
	scenario.RunDir(t, "testdata/scenarios", scenario.Config{Catalog: scenarioScripts})
}
