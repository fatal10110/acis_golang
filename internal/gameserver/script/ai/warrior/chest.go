package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TreasureChest is the treasure chest: a key's skill opens it or breaks it
// at the chance the key and the chest's level give, and any other attack
// makes it cast its treasure bomb on the attacker, after which it is gone.
func TreasureChest() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Chests", Behavior: true, NPCs: treasureChestIDs(), Hooks: BaseHooks().With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			treasureChestAttacked(s, e)
		},
		OnUseSkillFinished: func(_ *script.Script, e script.UseSkillFinished) {
			if e.Skill != (skill.Ref{}) {
				e.NPC.DeleteMe()
			}
		},
	})}
}

// treasureChestIDs are TreasureChest's own NPCs.
func treasureChestIDs() []int32 {
	return []int32{
		18265, 18266, 18267, 18268, 18269, 18270, 18271, 18272, 18273, 18274, 18275, 18276, 18277, 18278,
		18279, 18280, 18281, 18282, 18283, 18284, 18285, 18286, 18287, 18288, 18289, 18290, 18291, 18292,
		18293, 18294, 18295, 18296, 18297, 18298,
	}
}

// treasureChestAttacked is TreasureChest's attacked hook. A key skill
// rolls the chance its level gives against the chest's level: the chest
// dies to the attacker on a win and is deleted otherwise, its attacker's
// player hearing the key break. Anything else is answered with the
// treasure bomb, of the level the chest's level gives.
func treasureChestAttacked(s *script.Script, e script.Attacked) {
	// No skill is the zero Ref: id and level 0.
	n, attacker, k := e.NPC, e.Attacker, e.Skill
	i1 := k.Level
	npcLvl := int(n.Level())
	i0, i2 := 0, 0

	switch {
	case k.ID == 27:
		i0 = unlockChance(i1)
		i2 = i0 - (((npcLvl - (i1 * 4)) - 16) * 6)
		if i2 > i0 {
			i2 = i0
		}
	case k.ID == 2065:
		i2 = int(60 - (float64(npcLvl-((i1-1)*10)) * 1.5))
		if i2 > 60 {
			i2 = 60
		}
	case k.ID == 2229:
		i2 = deluxeKeyChance(i1, npcLvl)
	default:
		i0 = min(10, npcLvl/10+1)
		n.AddCastDesire(attacker, skill.Ref{ID: 4143, Level: i0}, 1000000)
		return
	}

	if s.Rnd(100) < i2 {
		n.DoDie(attacker)
	} else {
		n.DeleteMe()
		if p := script.ActingPlayer(attacker); p != nil {
			s.PlaySound(p, "ItemSound2.broken_key")
		}
	}
}

// unlockChance is the base chance, in percent, of the Unlock skill (27) of
// level i1; 0 for a level it does not have.
func unlockChance(i1 int) int {
	switch i1 {
	case 1:
		return 98
	case 2:
		return 84
	case 3:
		return 99
	case 4:
		return 84
	case 5:
		return 88
	case 6:
		return 90
	case 7:
		return 89
	case 8:
		return 88
	case 9:
		return 86
	case 10:
		return 90
	case 11:
		return 87
	case 12, 13, 14, 15:
		return 89
	}
	return 0
}

// deluxeKeyChance is the chance, in percent, a deluxe key (2229) of level
// i1 opens a chest of level npcLvl: certain below the key's grade, then a
// quadratic in the levels above it; a level past 8 never opens one.
func deluxeKeyChance(i1, npcLvl int) int {
	i3 := 0
	switch i1 {
	case 1:
		if i3 = npcLvl - 19; i3 < 0 {
			return 100
		}
		return int((((0.000200 * float64(i3*i3)) - (0.026400 * float64(i3))) + 0.769500) * 100)
	case 2:
		if i3 = npcLvl - 29; i3 < 0 {
			return 100
		}
		return int(((((0.000300 * float64(i3)) * float64(i3)) - (0.027900 * float64(i3))) + 0.756800) * 100)
	case 3:
		if i3 = npcLvl - 39; i3 < 0 {
			return 100
		}
		return int(((((0.000300 * float64(i3)) * float64(i3)) - (0.026900 * float64(i3))) + 0.733400) * 100)
	case 4:
		if i3 = npcLvl - 49; i3 < 0 {
			return 100
		}
		return int(((((0.000300 * float64(i3)) * float64(i3)) - (0.028400 * float64(i3))) + 0.803400) * 100)
	case 5:
		if i3 = npcLvl - 59; i3 < 0 {
			return 100
		}
		return int(((((0.000500 * float64(i3)) * float64(i3)) - (0.035600 * float64(i3))) + 0.906500) * 100)
	case 6:
		if i3 = npcLvl - 69; i3 < 0 {
			return 100
		}
		return int(((((0.000900 * float64(i3)) * float64(i3)) - (0.037300 * float64(i3))) + 0.857200) * 100)
	case 7:
		if i3 = npcLvl - 79; i3 < 0 {
			return 100
		}
		return int(((((0.004300 * float64(i3)) * float64(i3)) - (0.067100 * float64(i3))) + 0.959300) * 100)
	case 8:
		return 100
	}
	return 0
}

// TreasureChestMimic is the mimic: a monster posing as a chest. A key's
// skill makes it cast its wrath on itself and its strong attack on the
// attacker; every playable's attack makes it hate the attacker by the
// damage dealt, at least 1.
func TreasureChestMimic() script.Script {
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Chests", Behavior: true, NPCs: treasureChestMimicIDs(), Hooks: BaseHooks().With(script.Hooks{
		OnAttacked: func(_ *script.Script, e script.Attacked) {
			n, attacker, damage, k := e.NPC, e.Attacker, e.Damage, e.Skill
			if k.ID == 27 || k.ID == 2065 {
				skillLvl := int(n.Level()) - 5
				n.AddCastDesire(n, skill.Ref{ID: 4245, Level: 1}, 1000000)
				n.AddCastDesire(attacker, skill.Ref{ID: 4144, Level: skillLvl}, 1000000)
				n.AddAttackDesire(attacker, float64((int32(k.Level)/(n.Level()+7))*150))
			}
			if attacker.Playable() {
				if damage == 0 {
					damage = 1
				}
				n.AddAttackDesire(attacker, ((1.0*float64(damage))/float64(n.Level()+7))*100)
			}
		},
	})}
}

// treasureChestMimicIDs are TreasureChestMimic's own NPCs.
func treasureChestMimicIDs() []int32 {
	return []int32{
		21671, 21694, 21717, 21740, 21763, 21786, 21801, 21802, 21803, 21804, 21805, 21806, 21807, 21808,
		21809, 21810, 21811, 21812, 21813, 21814, 21815, 21816, 21817, 21818, 21819, 21820, 21821, 21822,
		18005,
	}
}
