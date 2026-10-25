package raid

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// BossType5 is RaidBossType5: a RaidBossParty that, one time in 150, wants
// its ranged hold at 1000000 on a playable hitting its party and on a
// caster it sees, while that creature holds none of its effect.
func BossType5() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/RaidBossParty/RaidBossType5", Behavior: true, NPCs: bossType5IDs(), Hooks: bossType5Hooks()}
}

// bossType5IDs are RaidBossType5's own NPCs.
func bossType5IDs() []int32 {
	return []int32{
		25109, 25322, 25346, 25220, 25296, 25302, 25283, 25328, 25514, 25001, 25032, 25060, 25070, 25082,
		25089, 25095, 25112, 25119, 25128, 25137, 25155, 25179, 25189, 25199, 25208, 25214, 25223, 25249,
		25263, 25273, 25352, 25360, 25373, 25378, 25383, 25392, 25401, 25404, 25407, 25410, 25418, 25426,
		25429, 25434, 25438, 25460, 25470, 25473, 25484, 25487, 25496, 29033, 25506, 25504,
	}
}

// bossType5Hooks are RaidBossType5's hooks. A template that lists no
// ranged hold still draws, then wants nothing.
func bossType5Hooks() script.Hooks {
	base := bossPartyHooks()
	return base.With(script.Hooks{
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			hold, _ := e.Called.SkillByType(npc.SkillTypeRangeHoldA)
			if (e.Target.Playable() && s.AbnormalLevel(e.Target, hold) == -1) && s.Rnd(10*15) < 1 {
				castByType(e.Called, e.Target, npc.SkillTypeRangeHoldA)
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			hold, _ := e.NPC.SkillByType(npc.SkillTypeRangeHoldA)
			if s.AbnormalLevel(e.Caster, hold) == -1 && s.Rnd(10*15) < 1 {
				castByType(e.NPC, e.Caster, npc.SkillTypeRangeHoldA)
			}
			base.SeeSpell(s, e)
		},
	}).Named("RaidBossType5")
}
