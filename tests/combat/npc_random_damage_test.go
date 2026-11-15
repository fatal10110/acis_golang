package combat

import (
	"testing"

	"github.com/rs/zerolog"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestShippedNPCRandomDamageFollowsWeapon pins the physical damage spread
// of shipped NPC templates: an NPC holding a right-hand weapon rolls that
// weapon's random_damage (0 included), an unarmed one 5 + sqrt(level); the
// template's baseRandomDamage never counts. Of the 6,499 shipped templates,
// 2,290 hold a weapon. A roll of the spread's low end scales the hit to
// 1 - spread/100.
func TestShippedNPCRandomDamageFollowsWeapon(t *testing.T) {
	t.Parallel()
	xmlDir := datapack.Path(t, "data", "xml")
	items, err := xmldata.LoadItemTemplates(xmlDir+"/items", zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	npcs, err := xmldata.LoadNPCTemplates(xmlDir+"/npcs", items, modelskill.NewTable(nil), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadNPCTemplates: %v", err)
	}
	armed := 0
	for _, tmpl := range npcs.All() {
		if it, ok := items.Get(int32(tmpl.RightHand)); ok && it.Weapon != nil {
			armed++
		}
	}
	if total := npcs.Len(); total != 6499 || armed != 2290 {
		t.Fatalf("shipped templates = %d with %d armed, want 6499 with 2290", total, armed)
	}

	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithItemTemplates(items),
	)
	type spreadRoller interface {
		creature.FormulaActor
		SetRollSource(func(n int) int)
	}
	for i, tc := range []struct {
		name string
		id   int
		folk bool
		// weapon is RandomDamageSpread's answer, -1 for unarmed; want is
		// the spread rolled.
		weapon, want int
	}{
		{"Gremlin, level 1, unarmed", 20001, false, -1, 6},
		{"level 70 monster, unarmed", 18008, false, -1, 13},
		{"monster with a random_damage 10 weapon", 18006, false, 10, 10},
		{"monster with a weapon of no random_damage", 21354, false, 0, 0},
		{"Claudia Athebalt, level 70 folk, unarmed", 31001, true, -1, 13},
	} {
		tmpl, ok := npcs.Get(tc.id)
		if !ok {
			t.Fatalf("%s: template %d missing", tc.name, tc.id)
		}
		at := location.Location{X: hostileX + 200*(i+1), Y: hostileY, Z: hostileZ}
		var npc spreadRoller
		if tc.folk {
			npc = srv.SpawnFolkNPCAt(t, tmpl, at)
		} else {
			npc = srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
		}
		if got := npc.RandomDamageSpread(); got != tc.weapon {
			t.Fatalf("%s: RandomDamageSpread() = %d, want %d", tc.name, got, tc.weapon)
		}
		npc.SetRollSource(func(int) int { return 0 })
		if got, want := creature.RandomDamageMultiplier(npc, modelskill.Definition{}), 1-float64(tc.want)/100; got != want {
			t.Fatalf("%s: lowest random damage multiplier = %v, want %v", tc.name, got, want)
		}
	}
}
