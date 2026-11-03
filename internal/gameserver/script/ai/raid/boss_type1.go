package raid

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// BossType1 is RaidBossType1: a lone raid boss with a self buff and three
// close-range specials (see BossType1Hooks).
func BossType1() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossType1", Behavior: true, NPCs: bossType1IDs(), Hooks: BossType1Hooks()}
}

// bossType1IDs are RaidBossType1's own NPCs.
func bossType1IDs() []int32 {
	return []int32{
		25019, 25050, 25063, 25088, 25098, 25102, 25118, 25125, 25127, 25158, 25162, 25169, 25188, 25198,
		25229, 25233, 25234, 25244, 25248, 25259, 25272, 25276, 25280, 25281, 25282, 25305, 25315, 25365,
		25372, 25391, 25394, 25437, 29036, 25512, 25527, 25523, 25255, 25126, 25163,
	}
}

// castByType has n want the skill its template lists under t on target at
// 1000000; nothing when it lists none.
func castByType(n *script.NPC, target script.Creature, t npc.SkillType) {
	if k, ok := n.SkillByType(t); ok {
		n.AddCastDesire(target, k, 1000000)
	}
}

// specials has n answer c, a playable that hit it or a caster it saw: its
// physical special on c when c is its top desire target, one time in 15;
// else, within 150, its ranged physical special around itself one time in
// 375; and within 150, whoever c is, its ranged cancel one time in 750.
func specials(s *script.Script, n *script.NPC, c script.Creature) {
	top := n.TopDesireTarget()
	distance := n.Distance2D(c)
	if script.Same(c, top) && s.Rnd(15) < 1 {
		castByType(n, c, npc.SkillTypePhysicalSpecialA)
	}
	if !script.Same(c, top) && distance < 150 && s.Rnd(375) < 1 {
		castByType(n, n, npc.SkillTypeSelfRangePhysicalSpecialA)
	}
	if distance < 150 && s.Rnd(750) < 1 {
		castByType(n, n, npc.SkillTypeSelfRangeCancelA)
	}
}

// BossType1Hooks are RaidBossType1's hooks, which its children derive
// theirs from: RaidBossStandard's, with its self buff wanted one time in
// five at each minute's check, and its specials (see specials) on a
// playable attacker and on a caster.
func BossType1Hooks() script.Hooks {
	base := BossStandardHooks()
	return base.With(script.Hooks{
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1001") {
				if s.Rnd(5) < 1 {
					castByType(e.NPC, e.NPC, npc.SkillTypeSelfBuffA)
				}
			}
			return base.Timer(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			if e.Attacker.Playable() {
				specials(s, e.NPC, e.Attacker)
			}
			base.Attacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			specials(s, e.NPC, e.Caster)
			base.SeeSpell(s, e)
		},
	}).Named("RaidBossType1")
}
