package monsterbehavior

import (
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// skillParams are the AI parameters that steer one of a skill-using
// wizard's six listed skills (SKILL01_ID to SKILL06_ID), each read as
// Skill0<n>_<name>.
type skillParams struct {
	probability, target, skillType, desire, checkDist, distMin, distMax, hpTarget, highHP, lowHP, mainAttack int32
}

// slotParams reads the parameters of n's skill i, 1 to 6. The first two
// skills are its main attacks by default: the first wanted from 300 to
// 2000 away, the second within 300, each always; the other four wanted at
// any distance, one time in three.
func slotParams(n *script.NPC, i int) skillParams {
	probabilities := [...]int32{10000, 10000, 3333, 3333, 3333, 3333}
	checkDists := [...]int32{1, 1, 0, 0, 0, 0}
	minDists := [...]int32{300, 0, 0, 0, 0, 0}
	maxDists := [...]int32{2000, 300, 2000, 2000, 2000, 2000}
	mainAttacks := [...]int32{1, 1, 0, 0, 0, 0}
	name := "Skill0" + strconv.Itoa(i)
	return skillParams{
		probability: n.IntParamOr(name+"_Probablity", probabilities[i-1]),
		target:      n.IntParam(name + "_Target"),
		skillType:   n.IntParam(name + "_Type"),
		desire:      n.IntParamOr(name+"_Desire", 1000000000),
		checkDist:   n.IntParamOr(name+"_Check_Dist", checkDists[i-1]),
		distMin:     n.IntParamOr(name+"_Dist_Min", minDists[i-1]),
		distMax:     n.IntParamOr(name+"_Dist_Max", maxDists[i-1]),
		hpTarget:    n.IntParam(name + "_HPTarget"),
		highHP:      n.IntParamOr(name+"_HighHP", 100),
		lowHP:       n.IntParam(name + "_LowHP"),
		mainAttack:  n.IntParamOr(name+"_MainAttack", mainAttacks[i-1]),
	}
}

// listedSkill is one of a skill-using wizard's six skills: its number, 1
// to 6, the skill and its parameters.
type listedSkill struct {
	i int
	k skill.Ref
	p skillParams
}

// listedSkills returns, from 1 to 6, the skills n lists that it may ever
// want (a probability above 0), each with its parameters.
func listedSkills(n *script.NPC) []listedSkill {
	types := [...]npc.SkillType{
		npc.SkillTypeSkill01ID, npc.SkillTypeSkill02ID, npc.SkillTypeSkill03ID,
		npc.SkillTypeSkill04ID, npc.SkillTypeSkill05ID, npc.SkillTypeSkill06ID,
	}
	var out []listedSkill
	for i := 1; i < 7; i++ {
		k, ok := n.SkillByType(types[i-1])
		if p := slotParams(n, i); ok && p.probability > 0 {
			out = append(out, listedSkill{i, k, p})
		}
	}
	return out
}

// playable reports whether c is a creature a player controls; false for
// none.
func playable(c script.Creature) bool { return c != nil && c.Playable() }

// useSkill has n want k on c as p says, on a draw of p.probability in
// 10000: not while a mute keeps it from k, nor outside p's HP band of its
// own HP (p.hpTarget 0 or 1), nor, when p.checkDist is 1, outside
// [p.distMin, p.distMax) of c. A main attack (type 0, p.mainAttack 1) whose
// HP or MP cost n cannot pay makes it attack c at 1000 instead and leave
// its skills for 10 s; a debuff or a buff (type 1 or 2) is wanted only on
// a c that holds no effect of its kind. A moving wizard (MovingAttack 1)
// walks to c, the other casts from where it stands.
func useSkill(s *script.Script, n *script.NPC, c script.Creature, k skill.Ref, p skillParams) {
	magic := s.Skill(k).Magic()
	if magic && n.Muted() || !magic && n.PhysicalMuted() {
		return
	}
	if s.Rnd(10000) >= int(p.probability) {
		return
	}
	if p.desire <= 0 {
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
	desire := float64(p.desire)
	movingAttack := n.IntParamOr("MovingAttack", 1)
	switch {
	case p.skillType == 0:
		if movingAttack == 1 {
			if p.mainAttack == 1 {
				if n.MeetsHPMPConditions(c, k) {
					n.AddCastDesire(c, k, desire)
				} else {
					n.SetInt(script.SlotIntAI4, 1)
					n.AddAttackDesire(c, 1000)
					s.StartTimer("5001", n, nil, 10000*time.Millisecond)
				}
			} else {
				n.AddCastDesire(c, k, desire)
			}
		} else if movingAttack == 0 && p.mainAttack == 1 {
			if n.MeetsHPMPConditions(c, k) {
				n.AddCastDesireHold(c, k, desire)
			} else {
				n.SetInt(script.SlotIntAI4, 1)
				n.AddAttackDesireHold(c, 1000)
				s.StartTimer("5001", n, nil, 10000*time.Millisecond)
			}
		}
	case (p.skillType == 1 || p.skillType == 2) && s.AbnormalLevel(c, k) <= 0, p.skillType == 3:
		if movingAttack == 1 {
			n.AddCastDesire(c, k, desire)
		} else if movingAttack == 0 {
			n.AddCastDesireHold(c, k, desire)
		}
	}
}

// answerAttack has n, off its skills, attack attacker at 10: a moving
// wizard (MovingAttack 1) also teleports onto attacker when attacker is its
// top desire target, it is hurt and it failed to find a path more than 10
// times in a row, and, rooted more than 40 from that target, drops the
// target when it cannot hit it from where it stands and wants attacker at
// 10 more; the other drops such a target and attacks attacker from where
// it stands, only while it has a top desire target.
func answerAttack(n *script.NPC, attacker script.Creature) {
	top := n.TopDesireTarget()
	if n.IntParamOr("MovingAttack", 1) == 1 {
		n.AddAttackDesire(attacker, attackedWeightPoint)
		if top != nil {
			if n.GeoPathFailCount() > 10 && script.Same(attacker, top) && n.HPRatio() < 1 {
				n.TeleportTo(attacker)
			}
			if n.Rooted() && n.Distance2D(top) > 40 {
				if !n.CanAutoAttack(top) {
					n.RemoveAttackDesire(top)
				}
				n.AddAttackDesire(attacker, attackedWeightPoint)
			}
		}
		return
	}
	if top != nil {
		if !n.CanAutoAttack(top) {
			n.RemoveAttackDesire(top)
		}
		n.AddAttackDesireHold(attacker, attackedWeightPoint)
	}
}

// WizardUseSkill is a WizardBehavior that casts the six skills its AI
// parameters describe (see useSkill). While on its skills (int slot 4 is
// 0) it wants each skill whose Skill0<n>_Target names an available target,
// from 1 to 6:
//   - hit by a playable: target 0 its most hated when playable, 1 the
//     attacker, 2 or 3 itself; a healer (IsHealer 1) skips its first skill;
//   - told by a clan member of a playable's hit: 0 and 1 as above, 2 and 4
//     the clan member, 3 itself, the clan member only for Party_Type 0, 2,
//     or 1 with Party_Loyalty 0; a healer, or a wizard past its
//     Aggressive_Time, skips its first two skills;
//   - told by a party member (not for Party_Type 0): as for a clan member,
//     the party member as its Party_Type and Party_Loyalty say;
//   - seeing a playable, when aggressive (IsAggressive) and no younger than
//     its Aggressive_Time: 0 its most hated, 1 the playable, 2 or 3 itself.
//
// Off its skills (int slot 4 is 1: a silence, or a main attack it could
// not pay for), it attacks instead (see answerAttack); seeing a playable
// inside its territory, it wants to attack it at 300. Each cast it
// finishes while on its skills, it wants its second skill on its most
// hated when that one stands where the second skill's distance check
// allows, else its first.
func WizardUseSkill() script.Script {
	base := WizardHooks()
	return script.Script{Dir: "ai/WizardBehavior/WizardUseSkill", Behavior: true, NPCs: []int32{20216, 20220, 21036, 21038, 21039, 21040, 21394, 21395}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			base.Attacked(s, e)
			n, attacker := e.NPC, e.Attacker
			mostHated := n.HateList().MostHated()
			if n.Int(script.SlotIntAI4) != 0 {
				if n.IntParam("IsHealer") != 1 {
					answerAttack(n, attacker)
				}
				return
			}
			for _, l := range listedSkills(n) {
				k, p := l.k, l.p
				if l.i == 1 && n.IntParam("IsHealer") == 1 || !attacker.Playable() {
					continue
				}
				switch {
				case p.target == 0 && playable(mostHated):
					useSkill(s, n, mostHated, k, p)
				case p.target == 1:
					useSkill(s, n, attacker, k, p)
				case p.target == 2 || p.target == 3:
					useSkill(s, n, n, k, p)
				}
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			base.ClanAttacked(s, e)
			caller, called, attacker := e.Caller, e.Called, e.Attacker
			mostHated := called.HateList().MostHated()
			partyType := called.IntParam("Party_Type")
			if called.Int(script.SlotIntAI4) != 0 {
				if partyType != 0 && called.LifeTime() > called.IntParam("Aggressive_Time") && called.IntParam("IsHealer") != 1 {
					answerAttack(called, attacker)
				}
				return
			}
			partyLoyalty := called.IntParam("Party_Loyalty")
			for _, l := range listedSkills(called) {
				k, p := l.k, l.p
				if l.i <= 2 && (called.IntParam("IsHealer") == 1 || called.LifeTime() > called.IntParam("Aggressive_Time")) || !attacker.Playable() {
					continue
				}
				helps := partyType == 0 || (partyType == 1 && partyLoyalty == 0) || partyType == 2
				switch {
				case p.target == 0 && playable(mostHated):
					useSkill(s, called, mostHated, k, p)
				case p.target == 1:
					useSkill(s, called, attacker, k, p)
				case p.target == 2 && helps:
					useSkill(s, called, caller, k, p)
				case p.target == 3:
					useSkill(s, called, called, k, p)
				case p.target == 4 && helps:
					useSkill(s, called, caller, k, p)
				}
			}
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			base.PartyAttacked(s, e)
			caller, called, target := e.Caller, e.Called, e.Target
			partyType, partyLoyalty := called.IntParam("Party_Type"), called.IntParam("Party_Loyalty")
			if partyType == 0 {
				return
			}
			mostHated := called.HateList().MostHated()
			master := called.Master()
			if called.Int(script.SlotIntAI4) != 0 {
				if partyLoyalty == 1 || ((partyLoyalty == 2 && script.Same(caller, master)) && called.LifeTime() > called.IntParam("Aggressive_Time") && called.IntParam("IsHealer") != 1) {
					answerAttack(called, target)
				}
				return
			}
			for _, l := range listedSkills(called) {
				k, p := l.k, l.p
				if l.i <= 2 && (called.IntParam("IsHealer") == 1 || called.LifeTime() > called.IntParam("Aggressive_Time")) || !target.Playable() {
					continue
				}
				// A master's call reaches a loyal follower, and any call but
				// the master's a leader's private, whatever the skill's
				// target says, unless it names the most hated or the hit one.
				masterCall := partyType == 1 && partyLoyalty == 2 && script.Same(caller, master)
				otherCall := partyType == 2 && !script.Same(caller, master)
				follows := partyType == 1 && (partyLoyalty == 0 || partyLoyalty == 1)
				switch {
				case p.target == 0 && playable(mostHated):
					useSkill(s, called, mostHated, k, p)
				case p.target == 1:
					useSkill(s, called, target, k, p)
				case (p.target == 2 && follows) || masterCall || otherCall:
					useSkill(s, called, caller, k, p)
				case p.target == 3:
					useSkill(s, called, called, k, p)
				case (p.target == 4 && follows) || masterCall || otherCall:
					useSkill(s, called, caller, k, p)
				}
			}
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			base.SeeCreature(s, e)
			n, c := e.NPC, e.Creature
			if !c.Playable() || n.IntParam("IsAggressive") == 0 || n.LifeTime() < n.IntParam("Aggressive_Time") {
				return
			}
			mostHated := n.HateList().MostHated()
			if n.Int(script.SlotIntAI4) == 0 {
				for _, l := range listedSkills(n) {
					k, p := l.k, l.p
					switch {
					case p.target == 0 && playable(mostHated):
						useSkill(s, n, mostHated, k, p)
					case p.target == 1:
						useSkill(s, n, c, k, p)
					case p.target == 2 || p.target == 3:
						useSkill(s, n, n, k, p)
					}
				}
			} else if n.InMyTerritory() {
				if n.IntParamOr("MovingAttack", 1) == 1 {
					n.AddAttackDesire(c, 300)
				} else {
					n.AddAttackDesireHold(c, 300)
				}
				base.SeeCreature(s, e)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			if n.Int(script.SlotIntAI4) != 0 {
				return
			}
			mostHated := n.HateList().MostHated()
			if mostHated == nil {
				return
			}
			skill01, ok1 := n.SkillByType(npc.SkillTypeSkill01ID)
			skill02, ok2 := n.SkillByType(npc.SkillTypeSkill02ID)
			p1 := skillParams{
				probability: n.IntParamOr("Skill01_Probablity", 10000),
				target:      n.IntParam("Skill01_Target"),
				skillType:   n.IntParam("Skill01_Type"),
				desire:      n.IntParamOr("Skill01_Desire", 1000000000),
				checkDist:   n.IntParamOr("Skill01_Check_Dist", 1),
				distMin:     n.IntParamOr("Skill01_Dist_Min", 300),
				distMax:     n.IntParamOr("Skill01_Dist_Max", 2000),
				hpTarget:    n.IntParam("Skill01_HPTarget"),
				highHP:      n.IntParamOr("Skill01_HighHP", 100),
				lowHP:       n.IntParam("Skill01_LowHP"),
				mainAttack:  n.IntParamOr("Skill01_MainAttack", 1),
			}
			p2 := skillParams{
				probability: n.IntParamOr("Skill02_Probablity", 10000),
				target:      n.IntParam("Skill02_Target"),
				skillType:   n.IntParam("Skill02_Type"),
				desire:      n.IntParamOr("Skill02_Desire", 1000000000),
				checkDist:   n.IntParamOr("Skill02_Check_Dist", 1),
				distMin:     n.IntParamOr("Skill02_Dist_Min", 0),
				distMax:     n.IntParamOr("Skill02_Dist_Max", 300),
				hpTarget:    n.IntParam("Skill02_HPTarget"),
				highHP:      n.IntParamOr("Skill02_HighHP", 100),
				lowHP:       n.IntParam("Skill02_LowHP"),
				mainAttack:  n.IntParamOr("Skill02_MainAttack", 1),
			}
			switch {
			case ok2 && p2.probability > 0 && p2.checkDist == 1 && n.Distance2D(mostHated) > float64(p2.distMin) && n.Distance2D(mostHated) < float64(p2.distMax) && p2.mainAttack == 1:
				if (p2.target == 0 || p2.target == 1) && mostHated.Playable() {
					useSkill(s, n, mostHated, skill02, p2)
				} else if p2.target == 2 || p2.target == 3 {
					useSkill(s, n, n, skill02, p2)
				}
			case ok1 && p1.probability > 0:
				if (p1.target == 0 || p1.target == 1) && mostHated.Playable() {
					useSkill(s, n, mostHated, skill01, p1)
				} else if p1.target == 2 || p1.target == 3 {
					useSkill(s, n, n, skill01, p1)
				}
			}
		},
	})}
}
