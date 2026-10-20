package warrior

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// royalRushRoomBossDir is the page directory of the Four Sepulchers room
// bosses.
const royalRushRoomBossDir = "ai/individual/Monster/WarriorBase/Warrior/RoyalRushRoomBossBasic"

// RoyalRushRoomBossBasic is RoyalRushRoomBossBasic: the Warrior a Four
// Sepulchers room boss is. Hit by a playable it wants, on a 33 in 100
// draw, to strike its top desire target with its first physical special
// skill when that is the attacker; dying, it leaves its KeyBox NPC at the
// KeyBox_X, KeyBox_Y, KeyBox_Z point, or where it stands when one of them
// is 0.
func RoyalRushRoomBossBasic() script.Script {
	return script.Script{Dir: royalRushRoomBossDir, Behavior: true, Hooks: royalRushRoomBossBasicHooks()}
}

// royalRushRoomBossBasicHooks are RoyalRushRoomBossBasic's hooks, which
// its children derive theirs from.
func royalRushRoomBossBasicHooks() script.Hooks {
	base := WarriorHooks()
	return base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if top := n.TopDesireTarget(); top != nil && script.Same(top, attacker) && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypePhysicalSpecial1)
				}
			}
			base.Attacked(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			keyBox := n.IntParam("KeyBox")
			x, y, z := n.IntParam("KeyBox_X"), n.IntParam("KeyBox_Y"), n.IntParam("KeyBox_Z")
			if x != 0 && y != 0 && z != 0 {
				s.Spawner().CreateOnePrivateEx(n, keyBox, script.Loc{X: int(x), Y: int(y), Z: int(z)}, 0, script.Params{})
			} else {
				at := script.LocOf(n)
				s.Spawner().CreateOnePrivateEx(n, keyBox, script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, script.Params{})
			}
			base.MyDying(s, e)
		},
	}).Named("RoyalRushRoomBossBasic")
}

// RoyalRushRoomBoss1 is RoyalRushRoomBoss1: a RoyalRushRoomBossBasic that
// weakens itself with its first self debuff as it appears and again as
// the second, fourth and sixth member of its clan dies (its second and
// third self debuffs for the last two), counting the deaths in i_ai0. Hit
// by a playable it wants, on a 33 in 100 draw, its first range magic on
// its top desire target when that is the attacker.
func RoyalRushRoomBoss1() script.Script {
	return script.Script{Dir: royalRushRoomBossDir, Behavior: true, NPCs: []int32{18121, 18124, 18127, 18130, 18174, 18177, 18180, 18183}, Hooks: royalRushRoomBoss1Hooks()}
}

// royalRushRoomBoss1Hooks are RoyalRushRoomBoss1's hooks, which its child
// derives its own from.
func royalRushRoomBoss1Hooks() script.Hooks {
	base := royalRushRoomBossBasicHooks()
	return base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			n := e.NPC
			n.SetInt(script.SlotIntAI0, 0)
			castByType(n, n, npc.SkillTypeSelfDebuff1)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if top := n.TopDesireTarget(); top != nil && script.Same(top, attacker) && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypeRangeDDMagic1)
				}
			}
			base.Attacked(s, e)
		},
		OnClanDied: func(s *script.Script, e script.ClanDied) {
			if called := e.Called; !script.Same(e.Caller, called) {
				switch called.Int(script.SlotIntAI0) {
				case 2:
					castByType(called, called, npc.SkillTypeSelfDebuff1)
				case 4:
					castByType(called, called, npc.SkillTypeSelfDebuff2)
				case 6:
					castByType(called, called, npc.SkillTypeSelfDebuff3)
				}
				called.SetInt(script.SlotIntAI0, called.Int(script.SlotIntAI0)+1)
			}
			base.ClanDied(s, e)
		},
	}).Named("RoyalRushRoomBoss1")
}

// RoyalRushRoomBoss2 is RoyalRushRoomBoss2: a RoyalRushRoomBoss1 that
// appears without weakening itself.
func RoyalRushRoomBoss2() script.Script {
	return script.Script{Dir: royalRushRoomBossDir, Behavior: true, NPCs: []int32{18120, 18123, 18126, 18129}, Hooks: royalRushRoomBoss1Hooks().With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
		},
	})}
}

// RoyalRushRoomBoss4 is RoyalRushRoomBoss4: a RoyalRushRoomBossBasic that
// buffs its clan with its first clan buff as it appears. Hit by a playable
// it wants, on a 33 in 100 draw, its first range magic on its top desire
// target when that is the attacker. Told of a hit on its clan it wants, on
// a 33 in 100 draw, the same magic on an attacker that is not its top
// desire target, and, on a 10 in 100 draw, its clan buff on the member
// that was hit.
func RoyalRushRoomBoss4() script.Script {
	base := royalRushRoomBossBasicHooks()
	ids := []int32{18141, 18142, 18143, 18144, 18173, 18176, 18179, 18182, 18212, 18213, 18214, 18215, 18216, 18217, 18218, 18219}
	return script.Script{Dir: royalRushRoomBossDir, Behavior: true, NPCs: ids, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			castByType(n, n, npc.SkillTypeClanBuff1)
			base.Created(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if top := n.TopDesireTarget(); top != nil && script.Same(top, attacker) && s.Rnd(100) < 33 {
					castByType(n, attacker, npc.SkillTypeRangeDDMagic1)
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if s.Rnd(100) < 33 && !script.Same(called.TopDesireTarget(), attacker) {
				castByType(called, attacker, npc.SkillTypeRangeDDMagic1)
			}
			if s.Rnd(100) < 10 {
				castByType(called, e.Caller, npc.SkillTypeClanBuff1)
			}
			base.ClanAttacked(s, e)
		},
	})}
}

// RoyalRushRoomBoss5 is RoyalRushRoomBoss5: a RoyalRushRoomBossBasic that,
// hit by a playable while it has a top desire target, draws three times
// out of 100: under 33 on the first, when the attacker is not its top
// desire target, it wants its first self range magic on itself; under 33
// on the second and on the third, when the attacker is that target, it
// wants its debuff and its magic on the attacker.
func RoyalRushRoomBoss5() script.Script {
	base := royalRushRoomBossBasicHooks()
	return script.Script{Dir: royalRushRoomBossDir, Behavior: true, NPCs: []int32{18122, 18125, 18128, 18131, 18175, 18178, 18181, 18184}, Hooks: base.With(script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if top := n.TopDesireTarget(); top != nil {
					if s.Rnd(100) < 33 && !script.Same(top, attacker) {
						castByType(n, n, npc.SkillTypeSelfRangeDDMagic1)
					}
					if s.Rnd(100) < 33 && script.Same(top, attacker) {
						castByType(n, attacker, npc.SkillTypeDebuff)
					}
					if s.Rnd(100) < 33 && script.Same(top, attacker) {
						castByType(n, attacker, npc.SkillTypeDDMagic)
					}
				}
			}
			base.Attacked(s, e)
		},
	})}
}
