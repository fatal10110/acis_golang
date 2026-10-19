package individual

import (
	"math"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// The items and quests a tamed beast reads.
const (
	crystalOfPurity      = 8084
	requiredCrystalCount = 10
	jewelOfInnocence     = 7185
	goldenSpice          = 6643
	crystalSpice         = 6644
	bringUpWithLove      = "Q020_BringUpWithLove"
	tamingWildBeasts     = "Q655_AGrandPlanForTamingWildBeasts"
)

// WarriorPetForPC is a beast a player tamed. It keeps its owner, the
// creature its first spawn parameter names, in its first creature slot and
// the spice it eats in its fourth int slot: Golden Spice when its second
// spawn parameter is skill 2188, Crystal Spice otherwise. Created for a
// player, it may greet him, may give him the Jewel of Innocence of Bring Up
// With Love, may give his clan leader a Crystal of Purity of A Grand Plan
// for Taming Wild Beasts, and tells the beasts within 1500 that it is the
// newest of its owner's (event 10022), so his older ones leave. Every 5 s
// it may buff its owner when he lacks three of its five buffs, tells the
// NPCs within 700 that it is there (event 10017) and leaves once he is
// more than 2000 away; every 15 s it repeats its claim within 500; every
// minute it eats from his spice, or leaves when he has too little or is
// dead. Idle, it walks to 150 short of him; a clan member attacked makes
// it debuff that member and heal its owner as his HP drops.
func WarriorPetForPC() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{
		Dir: dir, Behavior: true,
		NPCs: []int32{16013, 16014, 16015, 16016, 16017, 16018, 16020, 16022, 16024},
		Hooks: base.With(script.Hooks{
			OnCreated: func(s *script.Script, e script.Created) {
				n := e.NPC
				if n.Int(script.SlotParam2) == 2188 {
					n.SetInt(script.SlotIntAI3, goldenSpice)
				} else {
					n.SetInt(script.SlotIntAI3, crystalSpice)
				}

				n.SetSlot(script.SlotCreature0, s.Object(n.Int(script.SlotParam1)))
				owner, ok := n.Slot(script.SlotCreature0).(*script.Player)
				if !ok {
					return
				}

				if s.Rnd(100) < 5 {
					i0 := s.Rnd(((2028 - 2024) + 1))
					i0 = (i0 + 2024)
					n.Say(script.NpcString(int32(i0), owner.Name()))
				}

				s.StartTimer("2001", n, nil, 5000*time.Millisecond)

				st := s.QuestState(owner, bringUpWithLove)
				if st != nil && s.Rnd(100) < 5 && !owner.HasItem(jewelOfInnocence) {
					s.GiveItems(owner, jewelOfInnocence, 1)
					st.SetCond(2)
				}

				testCrystalOfPurity(s, owner, n)

				n.SetInt(script.SlotIntAI1, s.CurrentTick())
				s.BroadcastScriptEventEx(n, 10022, owner.ObjectID(), n.Int(script.SlotIntAI1), 1500)
				s.StartTimer("2002", n, nil, 15000*time.Millisecond)
				s.StartTimer("2003", n, nil, 60000*time.Millisecond)
			},
			OnTimer: func(s *script.Script, e script.Timer) string {
				n := e.NPC
				owner := n.Slot(script.SlotCreature0)
				switch {
				case strings.EqualFold(e.Name, "2001"):
					if !n.Dead() {
						i0 := 5
						if abnormalLevelOf(s, owner, n, npc.SkillTypeBuff1) <= 0 {
							i0 = (i0 - 1)
						}
						if abnormalLevelOf(s, owner, n, npc.SkillTypeBuff2) <= 0 {
							i0 = (i0 - 1)
						}
						if abnormalLevelOf(s, owner, n, npc.SkillTypeBuff3) <= 0 {
							i0 = (i0 - 1)
						}
						if abnormalLevelOf(s, owner, n, npc.SkillTypeBuff4) <= 0 {
							i0 = (i0 - 1)
						}
						if abnormalLevelOf(s, owner, n, npc.SkillTypeBuff5) <= 0 {
							i0 = (i0 - 1)
						}

						if owner == nil {
							return ""
						}

						if i0 < 3 {
							i1 := s.Rnd(100)
							switch {
							case i1 < 20:
								buffIfMissing(s, n, owner, npc.SkillTypeBuff1)
							case i1 < 40:
								buffIfMissing(s, n, owner, npc.SkillTypeBuff2)
							case i1 < 60:
								buffIfMissing(s, n, owner, npc.SkillTypeBuff3)
							case i1 < 80:
								buffIfMissing(s, n, owner, npc.SkillTypeBuff4)
							default:
								buffIfMissing(s, n, owner, npc.SkillTypeBuff5)
							}
						}
					}

					s.BroadcastScriptEvent(n, 10017, n.ObjectID(), 700)

					if n.Distance2D(owner) > 2000 {
						n.DeleteMe()
					}

					s.StartTimer("2001", n, e.Player, 5000*time.Millisecond)
				case strings.EqualFold(e.Name, "2002"):
					if owner != nil {
						s.BroadcastScriptEventEx(n, 10022, owner.ObjectID(), n.Int(script.SlotIntAI1), 500)
					}

					s.StartTimer("2002", n, e.Player, 15000*time.Millisecond)
				case strings.EqualFold(e.Name, "2003"):
					if owner != nil {
						p := owner.(*script.Player)
						if p.ItemCount(n.Int(script.SlotIntAI3)) >= n.IntParamOr("ConsumeFeedNum", 1) && !owner.Dead() {
							s.TakeItems(p, n.Int(script.SlotIntAI3), n.IntParamOr("ConsumeFeedNum", 1))
							if n.IntParam("TakeSocial") != 0 {
								n.AddSocialDesire(1, int((n.IntParam("TakeSocial")*1000)/30), 200)
							}

							i0 := s.Rnd(((2038 - 2029) + 1))
							i0 = (i0 + 2029)
							n.Say(script.NpcString(int32(i0)))
						} else {
							n.DeleteMe()
						}
					}

					s.StartTimer("2003", n, e.Player, 60000*time.Millisecond)
				}
				return ""
			},
			OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
				if e.EventID == 10022 {
					owner := e.NPC.Slot(script.SlotCreature0)
					if owner == nil {
						return
					}

					if e.Arg1 == owner.ObjectID() && e.Arg2 > e.NPC.Int(script.SlotIntAI1) {
						e.NPC.DeleteMe()
					}
				}
			},
			OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
				if script.Same(e.Caller, e.Called) {
					return
				}

				debuff1, hasDebuff1 := e.Called.SkillByType(npc.SkillTypeDebuff1)
				debuff2, hasDebuff2 := e.Called.SkillByType(npc.SkillTypeDebuff2)
				owner := e.Called.Slot(script.SlotCreature0)

				i0 := 2
				if abnormalLevelOf(s, owner, e.Called, npc.SkillTypeDebuff1) <= 0 {
					i0 = (i0 - 1)
				}
				if abnormalLevelOf(s, owner, e.Called, npc.SkillTypeDebuff2) <= 0 {
					i0 = (i0 - 1)
				}

				if i0 < 1 && e.Caller.HPRatio() >= 0.8 {
					if abnormalLevelOf(s, e.Caller, e.Called, npc.SkillTypeDebuff1) <= 0 && s.Rnd(100) < 33 && hasDebuff1 {
						e.Called.AddCastDesire(e.Caller, debuff1, 1000000)
					}
					if abnormalLevelOf(s, e.Caller, e.Called, npc.SkillTypeDebuff2) <= 0 && s.Rnd(100) < 33 && hasDebuff2 {
						e.Called.AddCastDesire(e.Caller, debuff2, 1000000)
					}
				}

				if owner == nil {
					return
				}

				switch {
				case owner.HPRatio() < 0.25:
					if s.Rnd(100) < 40 {
						castByType(e.Called, owner, npc.SkillTypeHeal)
					}
				case owner.HPRatio() < 0.5:
					if s.Rnd(100) < 20 {
						castByType(e.Called, owner, npc.SkillTypeHeal)
					}
				}
			},
			OnNoDesire: func(_ *script.Script, e script.NoDesire) {
				n := e.NPC
				owner := n.Slot(script.SlotCreature0)
				if owner == nil {
					return
				}
				to, at := script.LocOf(owner), script.LocOf(n)

				deltaX := float64(to.X - at.X)
				deltaY := float64(to.Y - at.Y)

				magnitude := math.Sqrt(deltaX*deltaX + deltaY*deltaY)

				unitX := deltaX / magnitude
				unitY := deltaY / magnitude

				desiredX := commons.TruncInt32Sat(float64(to.X) - 150*unitX)
				desiredY := commons.TruncInt32Sat(float64(to.Y) - 150*unitY)

				n.SetRunning(true)
				n.AddMoveToDesire(script.Loc{X: int(desiredX), Y: int(desiredY), Z: to.Z}, 10000)
			},
		}),
	}
}

// abnormalLevelOf is the level at which c holds the effect of the skill
// n's template lists under t, -1 when the template lists none; with a
// skill listed and no creature it panics, so the invocation aborts there.
func abnormalLevelOf(s *script.Script, c script.Creature, n *script.NPC, t npc.SkillType) int32 {
	ref, ok := n.SkillByType(t)
	if !ok {
		return -1
	}
	if c == nil {
		panic("individual: the abnormal level of no creature")
	}
	return s.AbnormalLevel(c, ref)
}

// buffIfMissing makes n want to cast at owner, weight 1000000, the buff its
// template lists under t, when owner does not hold it.
func buffIfMissing(s *script.Script, n *script.NPC, owner script.Creature, t npc.SkillType) {
	if abnormalLevelOf(s, owner, n, t) <= 0 {
		castByType(n, owner, t)
	}
}

// castByType makes n want to cast at target, weight 1000000, the skill its
// template lists under t; a template that lists none casts nothing.
func castByType(n *script.NPC, target script.Creature, t npc.SkillType) {
	if ref, ok := n.SkillByType(t); ok {
		n.AddCastDesire(target, ref, 1000000)
	}
}

// testCrystalOfPurity gives player, within 2000 of n, a Crystal of Purity,
// up to 10, while the leader of his clan, he himself or one online within
// 2000 of n, is on the first step of A Grand Plan for Taming Wild Beasts;
// the tenth moves the leader's quest to its second step.
func testCrystalOfPurity(s *script.Script, player *script.Player, n *script.NPC) {
	if player == nil || n == nil {
		return
	}

	if !script.InRadius3D(player, n, 2000) {
		return
	}

	var st *script.QuestState

	if player.IsClanLeader() {
		st = s.QuestState(player, tamingWildBeasts)
	} else {
		if !player.InClan() {
			return
		}

		leader := player.ClanLeader()
		if leader == nil {
			return
		}

		if script.InRadius3D(leader, n, 2000) {
			st = s.QuestState(leader, tamingWildBeasts)
		}
	}

	if st == nil {
		return
	}

	if st.Cond() != 1 {
		return
	}

	if s.DropItemsAlways(player, crystalOfPurity, 1, requiredCrystalCount) {
		st.SetCond(2)
	}
}
