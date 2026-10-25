package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestScriptNPCHitPointsOfFolkAndHostile: NPC.HP and NPC.MaxHP read a
// civilian NPC as they read a hostile one (CreatureStatus.getHp and
// getMaxHp on any creature): the maximum is the NPC's calculated one, a
// fresh spawn is at it, and the current hit points follow SetHP, fraction
// kept.
func TestScriptNPCHitPointsOfFolkAndHostile(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, nil, nil)
	hostileTmpl := &npc.Template{
		ID: 20040, TemplateID: 20041, Type: "Monster", Level: 20, HPMax: 1000, AtkSpd: 300,
		RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
	h := w.srv.SpawnHostileNPCTemplateAt(t, hostileTmpl, location.Location{X: w.at.X + 30, Y: w.at.Y, Z: w.at.Z})
	f := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 35588), location.Location{X: w.at.X, Y: w.at.Y + 30, Z: w.at.Z})

	for _, tc := range []struct {
		name  string
		npc   attackable.Combatant
		maxHP int32
	}{
		{"hostile", h, int32(h.MaxHP())},
		{"folk", f, int32(f.MaxHP())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runOps(t, w.srv, w.player, q001, func(_ *script.Script, _ *script.Player) {
				n := script.NPCOf(tc.npc)
				if got := n.MaxHP(); got != tc.maxHP {
					t.Errorf("MaxHP() = %d, want %d", got, tc.maxHP)
				}
				if got := n.HP(); got != float64(tc.maxHP) {
					t.Errorf("HP() of a fresh spawn = %v, want %d", got, tc.maxHP)
				}
				n.SetHP(200.5)
				if got := n.HP(); got != 200.5 {
					t.Errorf("HP() after SetHP(200.5) = %v", got)
				}
			})
		})
	}
}

// TestScriptAsksAnNPCForItemsAborts: an NPC has no inventory, so asking a
// hostile or a civilian NPC for items aborts the invocation; a player
// answers from its own inventory.
func TestScriptAsksAnNPCForItemsAborts(t *testing.T) {
	t.Parallel()
	w := bootDialog(t, nil, nil, opsItemTemplates())
	h := w.srv.SpawnHostileNPCAt(t, location.Location{X: w.at.X + 30, Y: w.at.Y, Z: w.at.Z})
	f := w.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 35588), location.Location{X: w.at.X, Y: w.at.Y + 30, Z: w.at.Z})
	for _, tc := range []struct {
		name string
		npc  attackable.Combatant
	}{{"hostile", h}, {"folk", f}} {
		answered := false
		if w.srv.RunScript(t, w.player, q001, func(_ *script.Script, _ *script.Player) {
			var c script.Creature = script.NPCOf(tc.npc)
			c.HasAtLeastOneItem(57)
			answered = true
		}) || answered {
			t.Errorf("asking a %s NPC for items did not abort the invocation", tc.name)
		}
	}
	runOps(t, w.srv, w.player, q001, func(sc *script.Script, p *script.Player) {
		var c script.Creature = p
		if c.HasAtLeastOneItem(dropItem) {
			t.Error("a player without the item answered true")
		}
		sc.GiveItems(p, dropItem, 1)
		if !c.HasAtLeastOneItem(soulshotNoGrade, dropItem) {
			t.Error("a player holding the second item answered false")
		}
	})
}
