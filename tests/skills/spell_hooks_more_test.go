package skills

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// spellEmptyAura is an offensive aura too narrow to reach anyone.
const spellEmptyAura = modelskill.ID(4104)

// TestPlayerSkillWithNoTargetStillSeen (CreatureCast.onMagicHitTimer,
// callSkill): a player's skill whose launch found no target still reaches
// every NPC within 1000 of the player as a seen spell, with no targets.
func TestPlayerSkillWithNoTargetStillSeen(t *testing.T) {
	t.Parallel()
	log := newSpellLog()
	defs := []modelskill.Definition{{
		ID: spellEmptyAura, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetAura,
		Radius: 10, Offensive: true, SkillType: "PDAM", Power: 1, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, defs)),
		log.option(),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, int(spellEmptyAura), 1)
	startInWorld(t, c)
	log.spawnWatchers(t, srv, objID)

	c.Send(encodeRequestMagicSkillUse(int32(spellEmptyAura), false, false))
	seen := fmt.Sprintf("see %d-1 caster=P(*script.Player) targets=[] pet=false", spellEmptyAura)
	log.expect(t, srv, "the aura on no one", map[string][]string{"A": {seen}, "B": {seen}})
}

// TestWeaponOnMagicSkillSpelled (Weapon.castSkillOnMagic): the skill a
// player's weapon sets off on an NPC is seen by the NPCs around the player,
// then, when it is not offensive, lands on that NPC as a good spell, all
// before the skill that set it off. A good proc on one NPC is not seen by
// that NPC; an offensive one is, and is no good spell.
func TestWeaponOnMagicSkillSpelled(t *testing.T) {
	t.Parallel()
	player := "P(*script.Player)"
	see := func(id modelskill.ID) string {
		return fmt.Sprintf("see %d-1 caster=%s targets=[A(*script.NPC)] pet=false", id, player)
	}
	abnormal := func(id modelskill.ID) string { return fmt.Sprintf("abnormal %d-1 caster=%s", id, player) }
	spelledLine := func(id modelskill.ID) string { return fmt.Sprintf("spelled %d-1 caster=%s", id, player) }
	for _, tc := range []struct {
		name  string
		proc  modelskill.ID
		cast  modelskill.ID
		ctrl  bool
		wantA []string
		wantB []string
	}{
		{
			name: "good", proc: onGoodTriggered, cast: spellGoodOne, ctrl: true,
			wantA: []string{abnormal(onGoodTriggered), spelledLine(onGoodTriggered), abnormal(spellGoodOne), spelledLine(spellGoodOne)},
			wantB: []string{see(onGoodTriggered), see(spellGoodOne)},
		},
		{
			name: "offensive", proc: mirageTriggered, cast: spellDebuff,
			wantA: []string{abnormal(mirageTriggered), see(mirageTriggered), abnormal(spellDebuff), see(spellDebuff)},
			wantB: []string{see(mirageTriggered), see(spellDebuff)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			log := newSpellLog()
			catalog := procWeaponCatalog(t, func(w *item.WeaponDetail) {
				w.OnCastSkill = &item.SkillTrigger{Skill: item.SkillRef{ID: int32(tc.proc), Level: 1}, Chance: -1}
			})
			defs := append(chanceSkills(), spellProbeSkills()...)
			srv, c, objID := bootArmedOpts(t, []gameservertest.Option{log.option()}, defs, catalog, func(int) int { return 0 }, []int32{fixtureSwordID}, int(tc.cast))
			a := log.spawnWatchers(t, srv, objID)
			targetHostile(t, c, a.ObjectID())
			drainUntilQuiet(t, c)

			c.Send(encodeRequestMagicSkillUse(int32(tc.cast), tc.ctrl, false))
			log.expect(t, srv, "the skill and its weapon proc", map[string][]string{"A": tc.wantA, "B": tc.wantB})
		})
	}
}
