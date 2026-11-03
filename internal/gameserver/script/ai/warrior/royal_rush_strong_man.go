package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// keyBoxID is the Key Box a Four Sepulchers strong man leaves where it
// dies.
const keyBoxID = 31455

// RoyalRushStrongMan1 is RoyalRushStrongMan1: a Four Sepulchers strong man
// that buffs itself as it appears, strikes what it fights with its first
// physical special, blasts around itself with its first self range DD magic
// when another playable hits it, leaves the world once the local clock
// reaches minute 55 of an hour, and leaves a Key Box where it dies.
func RoyalRushStrongMan1() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{18158, 18160, 18162, 18164}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.LookNeighbor()
			addCastByType(n, n, npc.SkillTypeSelfBuff, 1000000)

			s.StartTimerAtFixedRate("6001", n, nil, 60000*time.Millisecond, 60000*time.Millisecond)

			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)

				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.Intention() != script.IntentionAttack {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, c) {
						addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial1, 1000000)
					}
				}
			}

			defaultnpc.TryToAttack(s, n, c)

			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
						addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial1, 1000000)
					}

					if s.Rnd(100) < 33 && !script.Same(topDesireTarget, attacker) {
						addCastByType(n, n, npc.SkillTypeSelfRangeDDMagic1, 1000000)
					}
				}
			}

			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if (called.LifeTime() > 7 && attacker.Playable()) && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					addCastByType(called, attacker, npc.SkillTypePhysicalSpecial1, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "6001") {
				if s.Now().Minute() >= 55 {
					e.NPC.DeleteMe()
				}
			}

			return base.Timer(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			at := script.LocOf(n)
			s.Spawner().CreateOnePrivateEx(n, keyBoxID, script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, script.Params{})

			base.MyDying(s, e)
		},
	})}
}

// RoyalRushStrongMan2 is RoyalRushStrongMan2: RoyalRushStrongMan1 save
// that, when another playable hits it, it casts its first DD magic on what
// it fights instead.
func RoyalRushStrongMan2() script.Script {
	base := WarriorHooks()
	return script.Script{Dir: "ai/individual/Monster/WarriorBase/Warrior", Behavior: true, NPCs: []int32{18159, 18161, 18163, 18165}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			n.LookNeighbor()
			addCastByType(n, n, npc.SkillTypeSelfBuff, 1000000)

			s.StartTimerAtFixedRate("6001", n, nil, 60000*time.Millisecond, 60000*time.Millisecond)

			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)

				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() && n.Intention() != script.IntentionAttack {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, c) {
						addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial1, 1000000)
					}
				}
			}

			defaultnpc.TryToAttack(s, n, c)

			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if topDesireTarget := n.TopDesireTarget(); topDesireTarget != nil {
					if s.Rnd(100) < 33 && script.Same(topDesireTarget, attacker) {
						addCastByType(n, topDesireTarget, npc.SkillTypePhysicalSpecial1, 1000000)
					}

					if s.Rnd(100) < 33 && !script.Same(topDesireTarget, attacker) {
						addCastByType(n, topDesireTarget, npc.SkillTypeDDMagic1, 1000000)
					}
				}
			}

			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if (called.LifeTime() > 7 && attacker.Playable()) && called.Intention() != script.IntentionAttack {
				if s.Rnd(100) < 33 {
					addCastByType(called, attacker, npc.SkillTypePhysicalSpecial1, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "6001") {
				if s.Now().Minute() >= 55 {
					e.NPC.DeleteMe()
				}
			}

			return base.Timer(s, e)
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			at := script.LocOf(n)
			s.Spawner().CreateOnePrivateEx(n, keyBoxID, script.Loc{X: at.X, Y: at.Y, Z: at.Z}, 0, script.Params{})

			base.MyDying(s, e)
		},
	})}
}
