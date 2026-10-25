package raid

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The three debuffs the Ice Fairy Sirra casts on the Ice Queen's orders.
func sirraDebuff1() skill.Ref { return skill.Ref{ID: 4480, Level: 1} }
func sirraDebuff2() skill.Ref { return skill.Ref{ID: 4481, Level: 1} }
func sirraDebuff3() skill.Ref { return skill.Ref{ID: 4482, Level: 1} }

// IceFairySirra is the Ice Fairy Sirra, a RaidBossType2 on a clock: 34.5
// minutes after it appears it runs maker script event 10005 on the npcmaker
// schuttgart13_npc2314_1m1 and leaves the world. Script event 11036 sets its
// phase (i_ai0) to its first argument: phases 1 and 2 have it want its
// first or second debuff on itself, and phase 3 has it look around, from
// then on wanting its third debuff, unchecked, on an Ice Queen's guard
// (29057) it sees that does not hold it. Script event 11040 names the
// player (c_ai0) whose party it counts down to: 5, 15 and 30 minutes later
// that party sees the time left, 30 then 20 minutes, then the end. It wants
// to attack, at 200, a playable it sees while it stands in its territory,
// and one time in 50 wants its ranged DD magic at 1000000 on what hits its
// party and on a caster it sees. Dying, it runs maker script event 10025
// on schuttgart13_npc2314_1m1 and schuttgart13_npc2314_3m1.
func IceFairySirra() script.Script {
	return script.Script{Dir: "ai/individual/Monster/RaidBoss/RaidBossAlone/IceFairySirra", Behavior: true, NPCs: []int32{29056}, Hooks: iceFairySirraHooks()}
}

// sirraCountdown shows the countdown text to the party of the player n
// counts down to, if that player is in one.
func sirraCountdown(n *script.NPC, text string) {
	if p, ok := n.Slot(script.SlotCreature0).(*script.Player); ok && p != nil {
		p.ShowPartyScreenMessage(10000, text)
	}
}

// iceFairySirraHooks are IceFairySirra's hooks. Its see-creature, timer,
// script event and dying hooks call no parent.
func iceFairySirraHooks() script.Hooks {
	base := bossType2Hooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n := e.NPC
			if guard, ok := e.Creature.(*script.NPC); ok && n.Int(script.SlotIntAI0) == 3 && guard.NpcID() == 29057 && s.AbnormalLevel(guard, sirraDebuff3()) <= 0 {
				n.AddCastDesireUnchecked(guard, sirraDebuff3(), 1000000)
			}
			if !e.Creature.Playable() {
				return
			}
			if n.InMyTerritory() {
				n.AddAttackDesire(e.Creature, 200)
			}
		},
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			s.StartTimer("1005", n, nil, (34*60*1000+30000)*time.Millisecond)
			n.SetSlot(script.SlotCreature0, nil)
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			switch {
			case strings.EqualFold(e.Name, "1005"):
				s.Spawner().SendMakerEvent("schuttgart13_npc2314_1m1", "10005", 0, 0)
				n.DeleteMe()
			case strings.EqualFold(e.Name, "2003"):
				sirraCountdown(n, script.NpcString(1010643, "30"))
				s.StartTimer("2004", n, nil, 600000*time.Millisecond)
			case strings.EqualFold(e.Name, "2004"):
				sirraCountdown(n, script.NpcString(1010643, "20"))
				s.StartTimer("1012", n, nil, 15*60000*time.Millisecond)
			case strings.EqualFold(e.Name, "1012"):
				sirraCountdown(n, script.NpcString(1121002))
			}
			return ""
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			if s.Rnd(50) < 1 {
				castByType(e.Called, e.Target, npc.SkillTypeRangeDDMagicA)
			}
			base.PartyAttacked(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			if s.Rnd(50) < 1 {
				castByType(e.NPC, e.Caster, npc.SkillTypeRangeDDMagicA)
			}
			base.SeeSpell(s, e)
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			switch e.EventID {
			case 11036:
				switch e.Arg1 {
				case 1:
					n.SetInt(script.SlotIntAI0, 1)
					n.AddCastDesire(n, sirraDebuff1(), 1000000)
				case 2:
					n.SetInt(script.SlotIntAI0, 2)
					n.AddCastDesire(n, sirraDebuff2(), 1000000)
				case 3:
					n.SetInt(script.SlotIntAI0, 3)
					n.LookNeighbor()
				case 0:
					n.SetInt(script.SlotIntAI0, 0)
				}
			case 11040:
				n.SetSlot(script.SlotCreature0, s.Object(e.Arg1))
				s.StartTimer("2003", n, nil, 300000*time.Millisecond)
			}
		},
		OnMyDying: func(s *script.Script, _ script.MyDying) {
			s.Spawner().SendMakerEvent("schuttgart13_npc2314_1m1", "10025", 0, 0)
			s.Spawner().SendMakerEvent("schuttgart13_npc2314_3m1", "10025", 0, 0)
		},
	}).Named("IceFairySirra")
}
