package raid

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// BossType4 is RaidBossType4: a RaidBossParty that answers a playable
// hitting its party, and a caster in its sight, while it has a top desire
// target (see bossType4Answer).
func BossType4() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossParty/RaidBossType4", Behavior: true, NPCs: bossType4IDs(), Hooks: bossType4Hooks()}
}

// bossType4IDs are RaidBossType4's own NPCs.
func bossType4IDs() []int32 {
	return []int32{
		25319, 25325, 25290, 25306, 25293, 25349, 25140, 25054, 25245, 25453, 25252, 25309, 25316, 25013,
		25023, 25038, 25047, 25064, 25076, 25092, 25103, 25122, 25152, 25176, 25192, 25205, 25230, 25266,
		25354, 25369, 25380, 25385, 25398, 25415, 25420, 25423, 25441, 25450, 25467, 25478, 25493, 25498,
		29037, 25509, 29062,
	}
}

// bossType4Answer has n answer c while n has a top desire target. When c
// is not that target and both stand within 150 of n, n wants, at 1000000
// around itself, its ranged debuff one time in 2 and its other ranged
// debuff one time in 5, each only while neither c nor the target holds
// that debuff's effect. Then, one time in 75, it wants its DD magic on c
// at 1000000.
func bossType4Answer(s *script.Script, n *script.NPC, c script.Creature) {
	top := n.TopDesireTarget()
	if top == nil {
		return
	}
	if !script.Same(c, top) && n.Distance2D(c) < 150 && n.Distance2D(top) < 150 {
		debuff, _ := n.SkillByType(npc.SkillTypeSelfRangeDebuffA)
		if s.AbnormalLevel(c, debuff) == -1 && s.AbnormalLevel(top, debuff) == -1 && s.Rnd(2) < 1 {
			castByType(n, n, npc.SkillTypeSelfRangeDebuffA)
		}
		another, _ := n.SkillByType(npc.SkillTypeSelfRangeDebuffAnotherA)
		if s.AbnormalLevel(c, another) == -1 && s.AbnormalLevel(top, another) == -1 && s.Rnd(5) < 1 {
			castByType(n, n, npc.SkillTypeSelfRangeDebuffAnotherA)
		}
	}
	if s.Rnd(75) < 1 {
		castByType(n, c, npc.SkillTypeDDMagicA)
	}
}

// bossType4Hooks are RaidBossType4's hooks.
func bossType4Hooks() script.Hooks {
	base := bossPartyHooks()
	return base.With(script.Hooks{
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if e.Target.Playable() {
				bossType4Answer(s, e.Called, e.Target)
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			bossType4Answer(s, e.NPC, e.Caster)
			base.SeeSpell(s, e)
		},
	}).Named("RaidBossType4")
}
