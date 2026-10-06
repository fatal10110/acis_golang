package skills

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// attackedLog records a behavior's attacked hooks as lines naming the
// attacker by object id. Hooks run on the caster's queue.
type attackedLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *attackedLog) behavior(ids ...int32) func() script.Script {
	return func() script.Script {
		return script.Script{Behavior: true, NPCs: ids, Hooks: script.Hooks{
			OnAttacked: func(_ *script.Script, e script.Attacked) {
				l.mu.Lock()
				defer l.mu.Unlock()
				l.lines = append(l.lines, fmt.Sprintf("attacker=%d damage=%d skill=%d-%d", e.Attacker.ObjectID(), e.Damage, e.Skill.ID, e.Skill.Level))
			},
		}}
	}
}

func (l *attackedLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.lines)
}

func (l *attackedLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.lines)
}

// TestSkillCastRaisesAttacked drives a player's cast at an NPC with a bound
// behavior. A skill that is an offensive debuff or carries aggro points
// raises the attacked hook once its effects applied, with max(120, aggro
// points) as the damage; a damaging skill also raises it from its hit, with
// the damage, and that call comes first. An offensive skill that is neither
// raises it only from its hit.
func TestSkillCastRaisesAttacked(t *testing.T) {
	t.Parallel()
	const npcID = int32(20030)
	cast := func(id modelskill.ID, skillType string, debuff bool, aggro int, power float32) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
			CastRange: 900, HitTime: 500, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
			SkillType: skillType, Offensive: true, Debuff: debuff, AggroPoints: aggro, Power: power,
		}
	}
	for _, tc := range []struct {
		name string
		def  modelskill.Definition
		// want are the hook lines after the attacker id; "hit" stands for
		// the damage the hit dealt.
		want []string
	}{
		{"debuff", cast(4001, "DEBUFF", true, 0, 0), []string{"damage=120 skill=4001-1"}},
		{"aggro points", cast(4002, "PDAM", false, 300, 1), []string{"hit skill=4002-1", "damage=300 skill=4002-1"}},
		{"neither", cast(4003, "PDAM", false, 0, 1), []string{"hit skill=4003-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := &attackedLog{}
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
				gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{tc.def})),
				gameservertest.WithNPCScripts(map[int32]script.NPCKind{npcID: script.KindHostile},
					[]script.Listing{{Path: "ai.Recorder"}}, script.Catalog{"ai.Recorder": log.behavior(npcID)}),
			)
			c, objID := srv.Client, srv.SoleObjectID(t)
			seedKnownSkill(t, srv, objID, int(tc.def.ID), 1)
			startInWorld(t, c)
			tmpl := conditionNPCTemplate("Monster", 0)
			tmpl.ID, tmpl.TemplateID = int(npcID), int(npcID)
			hostile := srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
			drainUntilQuiet(t, c)
			targetHostile(t, c, hostile.ObjectID())
			drainUntilQuiet(t, c)
			before := hostile.HP()

			c.Send(encodeRequestMagicSkillUse(int32(tc.def.ID), false, false))
			srv.AdvanceUntil(t, "the attacked hooks", func() bool { return log.count() >= len(tc.want) })

			var want []string
			for _, w := range tc.want {
				if rest, ok := strings.CutPrefix(w, "hit "); ok {
					// The hook sees the damage truncated to an int.
					w = fmt.Sprintf("damage=%d %s", int(math.Trunc(before-hostile.HP()+1e-9)), rest)
				}
				want = append(want, fmt.Sprintf("attacker=%d %s", objID, w))
			}
			if got := log.snapshot(); !slices.Equal(got, want) {
				t.Fatalf("attacked hooks = %q, want %q", got, want)
			}
		})
	}
}
