package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// RoyalRushBomb is RoyalRushBomb: a Warrior of the Four Sepulchers that,
// hit while its hit points are under DDMagicUseHpRate percent (30 unless
// set), wants on a SelfRangeDDMagicRate in 100 draw (33 unless set) to
// blow up around itself with its self range magic, and leaves the world
// when any of its casts ends.
func RoyalRushBomb() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior/RoyalRushBomb", Behavior: true, NPCs: []int32{18149, 18195, 18230}, Hooks: royalRushBombHooks()}
}

// royalRushBombHooks are RoyalRushBomb's hooks, which its children derive
// theirs from.
func royalRushBombHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if int32(s.Rnd(100)) < n.IntParamOr("SelfRangeDDMagicRate", 33) && n.HPRatio()*100 < float64(n.IntParamOr("DDMagicUseHpRate", 30)) {
				castByType(n, n, npc.SkillTypeSelfRangeDDMagic)
			}
			base.Attacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			e.NPC.DeleteMe()
		},
	}).Named("RoyalRushBomb")
}
