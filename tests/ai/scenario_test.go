package ai

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/follower"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/randomteleport"
	"github.com/fatal10110/acis_golang/internal/testsupport/scenario"
)

// probeID and probeFolkID are the NPC ids the runner's probe behavior is
// bound to: a monster and a civilian, each walking when its npc line says
// walks.
const (
	probeID     = 20120
	probeFolkID = 35600
)

// probe is a behavior that answers its NPC's attacked and dying hooks with
// a sound to the attacker or killer, naming the hook and the damage, its
// see-creature hook with a sound to the player seen, its created hook with
// a chat line, and its see-item and picked-item hooks with a chat line
// naming the items, a see-item hook asking it to pick each one up, so a
// scenario sees through packets which hooks its hit, kill, approach and
// look steps and its NPCs' spawns raised.
// It stands in for the behavior scripts until the first one is ported.
func probe() script.Script {
	sound := func(s *script.Script, c script.Creature, name string) {
		if p, ok := c.(*script.Player); ok {
			s.PlaySound(p, name)
		}
	}
	return script.Script{Behavior: true, NPCs: []int32{probeID, probeFolkID}, Hooks: script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			sound(s, e.Attacker, fmt.Sprintf("Probe.attacked_%d", e.Damage))
		},
		OnMyDying:     func(s *script.Script, e script.MyDying) { sound(s, e.Killer, "Probe.dying") },
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) { sound(s, e.Creature, "Probe.see") },
		OnCreated:     func(_ *script.Script, e script.Created) { e.NPC.Say("Probe.created") },
		OnSeeItem: func(_ *script.Script, e script.SeeItem) {
			ids := make([]string, 0, len(e.Items))
			for _, it := range e.Items {
				ids = append(ids, strconv.Itoa(int(it.ItemID)))
			}
			slices.Sort(ids)
			e.NPC.Say(fmt.Sprintf("Probe.see_item q=%d %s", e.Quantity, strings.Join(ids, ",")))
			for _, it := range e.Items {
				e.NPC.AddPickUpDesire(it.ObjectID, 1_000_000)
			}
		},
		OnPickedItem: func(_ *script.Script, e script.PickedItem) {
			e.NPC.Say(fmt.Sprintf("Probe.picked %d", e.Item.ItemID))
		},
	}}
}

// scenarioScripts are the scripts an NPC behavior scenario may list, by
// scripts.xml path.
var scenarioScripts = script.Catalog{
	"ai.Probe": probe,
	"script.ai.group.FollowerMovingAroundMaster": follower.New,
	"script.ai.group.RandomTeleport":             randomteleport.New,
}

// TestScenarios plays every NPC behavior scenario of testdata/scenarios
// through the booted server and MariaDB (format: package scenario).
func TestScenarios(t *testing.T) {
	t.Parallel()
	scenario.RunDir(t, "testdata/scenarios", scenario.Config{Catalog: scenarioScripts})
}
