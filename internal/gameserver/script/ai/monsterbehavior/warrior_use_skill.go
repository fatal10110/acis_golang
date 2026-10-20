package monsterbehavior

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// warriorSlotParams reads the parameters of n's skill i, 1 to 6, as a
// skill-using warrior reads them: every skill wanted one time in three, at
// any distance below 2000, none a main attack.
func warriorSlotParams(n *script.NPC, i int) skillParams {
	name := "Skill0" + strconv.Itoa(i)
	return skillParams{
		probability: n.IntParamOr(name+"_Probablity", 3333),
		target:      n.IntParam(name + "_Target"),
		skillType:   n.IntParam(name + "_Type"),
		desire:      n.IntParamOr(name+"_Desire", 1000000000),
		checkDist:   n.IntParam(name + "_Check_Dist"),
		distMin:     n.IntParam(name + "_Dist_Min"),
		distMax:     n.IntParamOr(name+"_Dist_Max", 2000),
		hpTarget:    n.IntParam(name + "_HPTarget"),
		highHP:      n.IntParamOr(name+"_HighHP", 100),
		lowHP:       n.IntParam(name + "_LowHP"),
		mainAttack:  n.IntParam(name + "_MainAttack"),
	}
}

// warriorSkills returns, from 1 to 6, the skills n lists that it may ever
// want (a probability above 0), each with its parameters; a healer
// (IsHealer 1) leaves out its first.
func warriorSkills(n *script.NPC) []listedSkill {
	types := [...]npc.SkillType{
		npc.SkillTypeSkill01ID, npc.SkillTypeSkill02ID, npc.SkillTypeSkill03ID,
		npc.SkillTypeSkill04ID, npc.SkillTypeSkill05ID, npc.SkillTypeSkill06ID,
	}
	var out []listedSkill
	for i := 1; i < 7; i++ {
		if i == 1 && n.IntParam("IsHealer") == 1 {
			continue
		}
		k, ok := n.SkillByType(types[i-1])
		if p := warriorSlotParams(n, i); ok && p.probability > 0 {
			out = append(out, listedSkill{i, k, p})
		}
	}
	return out
}

// warriorUseSkill has n want l's skill on c, unless a mute keeps it from
// the skill or the skill's desire is not positive, on a draw of its
// probability in 10000: not outside its HP band of n's own HP (HPTarget 0
// or 1), nor, when it checks the distance, outside [Dist_Min, Dist_Max) of
// c. A debuff or a buff (type 1 or 2) is wanted only on a c that holds no
// effect of its kind. A moving warrior (MovingAttack 1) walks to c, the
// other casts from where it stands.
func warriorUseSkill(s *script.Script, n *script.NPC, c script.Creature, l listedSkill) {
	k, p := l.k, l.p
	magic := s.Skill(k).Magic()
	if magic && n.Muted() || !magic && n.PhysicalMuted() {
		return
	}
	if p.desire <= 0 {
		return
	}
	if s.Rnd(10000) >= int(p.probability) {
		return
	}
	hp := n.HPRatio() * 100
	if (p.hpTarget == 0 && (hp > float64(p.highHP) || hp < float64(p.lowHP))) || (p.hpTarget == 1 && (hp > float64(p.highHP) || hp < float64(p.lowHP))) {
		return
	}
	if p.checkDist == 1 && (n.Distance2D(c) < float64(p.distMin) || n.Distance2D(c) >= float64(p.distMax)) {
		return
	}
	// A second distance check on a target that is both n and not n never
	// holds, so it is left out.
	if p.skillType == 0 || ((p.skillType == 1 || p.skillType == 2) && s.AbnormalLevel(c, k) <= 0) || p.skillType == 3 {
		switch n.IntParamOr("MovingAttack", 1) {
		case 1:
			n.AddCastDesire(c, k, float64(p.desire))
		case 0:
			n.AddCastDesireHold(c, k, float64(p.desire))
		}
	}
}

// WarriorUseSkill is a WarriorBehavior that casts the six skills its AI
// parameters describe (see warriorUseSkill), from 1 to 6, a healer
// (IsHealer 1) skipping its first, each on the target its Skill0<n>_Target
// names when there is one:
//   - hit: 0 its top desire target as the hit began, when playable; 1 a
//     playable attacker; 2 or 3 itself;
//   - told by a clan member of a hit, once its Aggressive_Time has passed:
//     0 and 1 as above, 3 itself, 2 and 4 the clan member, only for
//     Party_Type 0, 2, or 1 with Party_Loyalty 0;
//   - told by a party member of a hit (not for Party_Type 0): 0 and 1 as
//     above, 3 the attacker; for 2 and 4, as its Party_Type and
//     Party_Loyalty say, the party member itself casts the skill on the
//     attacker;
//   - seeing a creature, when aggressive (IsAggressive) and no younger than
//     its Aggressive_Time: 0 as above, 1 a playable it sees, 2 or 3 itself;
//   - seeing a spell: 0 as above, 1 the caster, 2 or 3 itself.
func WarriorUseSkill() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/WarriorBehavior/WarriorUseSkill", Behavior: true, NPCs: []int32{20136, 20214, 20215, 20217, 20218, 20219, 20222, 20751, 20752, 20753, 20754, 21035, 21037}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			top := n.TopDesireTarget()
			for _, l := range warriorSkills(n) {
				switch {
				case l.p.target == 0 && playable(top):
					warriorUseSkill(s, n, top, l)
				case l.p.target == 1 && attacker.Playable():
					warriorUseSkill(s, n, attacker, l)
				case l.p.target == 2 || l.p.target == 3:
					warriorUseSkill(s, n, n, l)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			if called.LifeTime() < called.IntParam("Aggressive_Time") {
				base.ClanAttacked(s, e)
				return
			}
			top := called.TopDesireTarget()
			partyType, partyLoyalty := called.IntParam("Party_Type"), called.IntParam("Party_Loyalty")
			helps := partyType == 0 || (partyType == 1 && partyLoyalty == 0) || partyType == 2
			for _, l := range warriorSkills(called) {
				switch {
				case l.p.target == 0 && playable(top):
					warriorUseSkill(s, called, top, l)
				case l.p.target == 1 && attacker.Playable():
					warriorUseSkill(s, called, attacker, l)
				case l.p.target == 2 && helps:
					warriorUseSkill(s, called, caller, l)
				case l.p.target == 3:
					warriorUseSkill(s, called, called, l)
				case l.p.target == 4 && helps:
					warriorUseSkill(s, called, caller, l)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			partyType := called.IntParam("Party_Type")
			if partyType == 0 {
				base.PartyAttacked(s, e)
				return
			}
			top := called.TopDesireTarget()
			partyLoyalty := called.IntParam("Party_Loyalty")
			master := called.Master()
			callerCasts := (partyType == 1 && (partyLoyalty == 0 || partyLoyalty == 1)) || (partyType == 1 && partyLoyalty == 2 && script.Same(caller, master)) || (partyType == 2 && !script.Same(caller, master))
			for _, l := range warriorSkills(called) {
				switch {
				case l.p.target == 0 && playable(top):
					warriorUseSkill(s, called, top, l)
				case l.p.target == 1 && target.Playable():
					warriorUseSkill(s, called, target, l)
				case l.p.target == 2:
					if callerCasts {
						warriorUseSkill(s, caller, target, l)
					}
				case l.p.target == 3:
					warriorUseSkill(s, called, target, l)
				case l.p.target == 4:
					if callerCasts {
						warriorUseSkill(s, caller, target, l)
					}
				}
			}
			base.PartyAttacked(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if n.IntParam("IsAggressive") == 0 || n.LifeTime() < n.IntParam("Aggressive_Time") {
				base.SeeCreature(s, e)
				return
			}
			top := n.TopDesireTarget()
			for _, l := range warriorSkills(n) {
				switch {
				case l.p.target == 0 && playable(top):
					warriorUseSkill(s, n, top, l)
				case l.p.target == 1 && c.Playable():
					warriorUseSkill(s, n, c, l)
				case l.p.target == 2 || l.p.target == 3:
					warriorUseSkill(s, n, n, l)
				}
			}
			base.SeeCreature(s, e)
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n := e.NPC
			top := n.TopDesireTarget()
			for _, l := range warriorSkills(n) {
				switch {
				case l.p.target == 0 && playable(top):
					warriorUseSkill(s, n, top, l)
				case l.p.target == 1:
					warriorUseSkill(s, n, e.Caster, l)
				case l.p.target == 2 || l.p.target == 3:
					warriorUseSkill(s, n, n, l)
				}
			}
			base.SeeSpell(s, e)
		},
	})}
}
