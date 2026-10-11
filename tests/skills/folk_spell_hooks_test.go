package skills

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	// spellFolkID is the civilian NPC bound to the recording behavior.
	spellFolkID = int32(30120)
	// spellHealOverTime is a good skill whose effect is neither a buff nor
	// a debuff.
	spellHealOverTime = modelskill.ID(4105)
	// spellFolkAura is the civilian NPC's own cast.
	spellFolkAura = modelskill.ID(4106)
)

// folkCatalog binds a behavior to spellFolkID recording its skill-finished,
// see-spell, spelled and abnormal-status hooks in l.
func (l *spellLog) folkCatalog() script.Catalog {
	return script.Catalog{"ai.FolkSeer": func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{spellFolkID}, Hooks: script.Hooks{
			OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
				l.add(e.NPC, "finished %d-%d target=%v success=%v", e.Skill.ID, e.Skill.Level, e.Creature, e.Success)
			},
			OnSeeSpell: func(_ *script.Script, e script.SeeSpell) {
				l.add(e.NPC, "see %d-%d caster=%v", e.Skill.ID, e.Skill.Level, e.Caster)
			},
			OnSpelled: func(_ *script.Script, e script.Spelled) {
				l.add(e.NPC, "spelled %d-%d caster=%v", e.Skill.ID, e.Skill.Level, e.Caster)
			},
			OnAbnormalStatusChanged: func(_ *script.Script, e script.AbnormalStatusChanged) {
				l.add(e.NPC, "abnormal %d-%d caster=%v", e.Skill.ID, e.Skill.Level, e.Caster)
			},
		}}
	}}
}

// TestFolkSpellAndCastHooks (Folk.addEffect, CreatureCast.callSkill,
// NpcCast.notifyCastFinishToAI): a civilian NPC's behavior is told of a
// player's good skill landing on it, its effect first, then the spell; an
// effect that is neither a buff nor a debuff never lands on a civilian NPC,
// so only its spell is reported. The NPC's own cast end reports the skill,
// its first target and its success.
func TestFolkSpellAndCastHooks(t *testing.T) {
	t.Parallel()
	log := newSpellLog()
	defs := append(spellProbeSkills(), modelskill.Definition{
		ID: spellHealOverTime, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "HOT", CastRange: 900, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Time: 15, Icon: true, StackType: "probe_hot", StackOrder: 1}},
	})
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
		gameservertest.WithAITask(),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{spellFolkID: script.KindFolk},
			[]script.Listing{{Path: "ai.FolkSeer"}}, log.folkCatalog()),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	for _, id := range []modelskill.ID{spellGoodOne, spellHealOverTime} {
		seedKnownSkill(t, srv, objID, int(id), 1)
	}
	startInWorld(t, c)
	setPlayerRollSource(t, srv, objID, func(int) int { return 0 })
	log.name(objID, "P")
	folk := srv.SpawnCastingFolkNPCAt(t, gameservertest.FolkTemplate("Folk", int(spellFolkID)),
		location.Location{X: hostileX, Y: hostileY, Z: hostileZ}, modelskill.NewTable([]modelskill.Definition{{
			ID: spellFolkAura, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetAura,
			Radius: 1000, SkillType: "HEAL", Power: 1, HitTime: 500, StaticHitTime: true, StaticReuse: true,
		}}))
	log.name(folk.ObjectID(), "F")
	settleAI(t, srv)
	drainUntilQuiet(t, c)
	player := "P(*script.Player)"

	targetHostile(t, c, folk.ObjectID())
	drainUntilQuiet(t, c)
	c.Send(encodeRequestMagicSkillUse(int32(spellGoodOne), false, false))
	log.expect(t, srv, "the good skill on the folk", map[string][]string{"F": {
		fmt.Sprintf("abnormal %d-1 caster=%s", spellGoodOne, player),
		fmt.Sprintf("spelled %d-1 caster=%s", spellGoodOne, player),
	}})
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(int32(spellHealOverTime), false, false))
	log.expect(t, srv, "the heal over time on the folk", map[string][]string{"F": {
		fmt.Sprintf("spelled %d-1 caster=%s", spellHealOverTime, player),
	}})
	drainUntilQuiet(t, c)

	addCastDesire(t, folk, modelskill.Ref{ID: spellFolkAura, Level: 1})
	tickAI(t, srv)
	log.expect(t, srv, "the folk's own cast", map[string][]string{"F": {
		fmt.Sprintf("finished %d-1 target=%s success=true", spellFolkAura, player),
	}})
}
