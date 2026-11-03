package skills

import (
	"fmt"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// doorSeerID is the template of the NPC that watches the door's attacker.
const doorSeerID = int32(20064)

// doorSeen is what one see-spell hook read of its payload's targets.
type doorSeen struct {
	n          int
	isDoor     bool
	objectID   int32
	name       string
	sameAsSelf bool
	power      float64
	dead       bool
}

// TestSeeSpellListsDoorTargetAsDoorHandle: a damage skill a player casts
// on a door under siege reaches the see-spell hook of an NPC watching
// nearby with the door among its targets (onSeeSpell's Creature[] holds
// doors), as a script door handle: the door's object id and template
// name, the same as itself and no other creature, standing, and a skill's
// power read against it.
func TestSeeSpellListsDoorTargetAsDoorHandle(t *testing.T) {
	t.Parallel()
	var (
		mu   sync.Mutex
		seen []doorSeen
	)
	catalog := script.Catalog{"ai.DoorSeer": func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{doorSeerID}, Hooks: script.Hooks{
			OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
				got := doorSeen{n: len(e.Targets)}
				if len(e.Targets) > 0 {
					c := e.Targets[0]
					_, got.isDoor = c.(*script.Door)
					got.objectID, got.name, got.dead = c.ObjectID(), c.Name(), c.Dead()
					got.sameAsSelf = script.Same(c, c) && !script.Same(c, e.NPC) && !script.Same(c, e.Caster) && !script.Same(c, nil)
					got.power = s.Skill(e.Skill).Power(c)
				}
				mu.Lock()
				seen = append(seen, got)
				mu.Unlock()
			},
		}}
	}}
	scripts := gameservertest.WithNPCScripts(map[int32]script.NPCKind{doorSeerID: script.KindHostile}, []script.Listing{{Path: "ai.DoorSeer"}}, catalog)

	def := doorSkill(doorPdamSkillID, "PDAM", true, 200)
	tmpl := combatDoorTemplate(door.KindDoor)
	srv, objID, gate := bootDoorCombat(t, tmpl, []modelskill.Definition{def}, scripts, gameservertest.WithScriptSkills(def))
	srv.BeginDoorSiege(t, gate.DoorID())
	x, y, z := srv.PlayerPosition(t, objID)
	seer := conditionNPCTemplate("Monster", 0)
	seer.ID, seer.TemplateID = int(doorSeerID), int(doorSeerID)
	srv.SpawnHostileNPCTemplateAt(t, seer, location.Location{X: x, Y: y + 200, Z: z})
	c := srv.Client
	drainUntilQuiet(t, c)

	selectTarget(t, c, gate.ObjectID())
	c.Send(encodeRequestMagicSkillUse(doorPdamSkillID, false, false))
	srv.AdvanceUntil(t, "the seer saw the skill", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(seen) > 0
	})
	srv.Settle(t)

	mu.Lock()
	defer mu.Unlock()
	want := doorSeen{n: 1, isDoor: true, objectID: gate.ObjectID(), name: tmpl.Name, sameAsSelf: true, power: 200}
	if len(seen) != 1 || fmt.Sprint(seen[0]) != fmt.Sprint(want) {
		t.Fatalf("see-spell hooks read %+v, want one %+v", seen, want)
	}
}
