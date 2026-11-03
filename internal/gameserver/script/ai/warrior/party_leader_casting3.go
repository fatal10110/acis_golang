package warrior

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

const partyLeaderCasting3Dir = "ai/individual/Monster/WarriorBase/Warrior/PartyLeaderWarrior/PartyLeaderCasting3SkillsMagicalAggressive"

// casting3DDMagic is the DD magic a
// PartyLeaderCasting3SkillsMagicalAggressive casts when its template lists
// none of the type.
func casting3DDMagic() skill.Ref { return skill.Ref{ID: 4001, Level: 1} }

// casting3Debuff is the debuff a PartyLeaderCasting3SkillsMagicalAggressive
// lays when its template lists none.
func casting3Debuff() skill.Ref { return skill.Ref{ID: 4037, Level: 1} }

// PartyLeaderCasting3SkillsMagicalAggressive is
// PartyLeaderCasting3SkillsMagicalAggressive: a PartyLeaderWarrior that
// attacks the players and summons it sees, and may cast either of its DD
// magics on one standing more than 100 away and lay its debuff on one it
// sees while wandering. The creature it most wants to attack, and a clan
// member's attacker while it is not attacking, get the same. It binds no
// NPC of its own.
func PartyLeaderCasting3SkillsMagicalAggressive() script.Script {
	return script.Script{Dir: partyLeaderCasting3Dir, Behavior: true, Hooks: PartyLeaderCasting3SkillsMagicalAggressiveHooks()}
}

// PartyLeaderCasting3SkillsMagicalAggressiveHooks are
// PartyLeaderCasting3SkillsMagicalAggressive's hooks, which its child
// derives its own from.
func PartyLeaderCasting3SkillsMagicalAggressiveHooks() script.Hooks {
	base := PartyLeaderHooks()
	return base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			n, c := e.NPC, e.Creature
			if !c.Playable() {
				base.SeeCreature(s, e)
				return
			}
			if n.LifeTime() > 7 && n.InMyTerritory() {
				if n.Distance3D(c) > 100 {
					if s.Rnd(100) < 33 {
						n.AddCastDesire(c, defaultnpc.SkillByTypeOrDefault(n, npc.SkillTypeDDMagic1, casting3DDMagic()), 1000000)
					}
					if s.Rnd(100) < 33 {
						n.AddCastDesire(c, defaultnpc.SkillByTypeOrDefault(n, npc.SkillTypeDDMagic2, casting3DDMagic()), 1000000)
					}
				}
				if n.Intention() == script.IntentionWander {
					debuff := defaultnpc.SkillByTypeOrDefault(n, npc.SkillTypeDebuff, casting3Debuff())
					if s.Rnd(100) < 33 && s.AbnormalLevel(c, debuff) <= 0 {
						n.AddCastDesire(c, debuff, 1000000)
					}
				}
			}
			defaultnpc.TryToAttack(s, n, c)
			base.SeeCreature(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Playable() {
				if script.Same(n.TopDesireTarget(), attacker) && n.Distance3D(attacker) > 100 {
					if s.Rnd(100) < 33 {
						n.AddCastDesire(attacker, defaultnpc.SkillByTypeOrDefault(n, npc.SkillTypeDDMagic1, casting3DDMagic()), 1000000)
					}
					if s.Rnd(100) < 33 {
						n.AddCastDesire(attacker, defaultnpc.SkillByTypeOrDefault(n, npc.SkillTypeDDMagic2, casting3DDMagic()), 1000000)
					}
					debuff := defaultnpc.SkillByTypeOrDefault(n, npc.SkillTypeDebuff, casting3Debuff())
					if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
						n.AddCastDesire(attacker, debuff, 1000000)
					}
				}
			}
			base.Attacked(s, e)
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() && called.LifeTime() > 7 && called.Intention() != script.IntentionAttack {
				if called.Distance3D(attacker) > 100 {
					if s.Rnd(100) < 33 {
						called.AddCastDesire(attacker, defaultnpc.SkillByTypeOrDefault(called, npc.SkillTypeDDMagic1, casting3DDMagic()), 1000000)
					}
					if s.Rnd(100) < 33 {
						called.AddCastDesire(attacker, defaultnpc.SkillByTypeOrDefault(called, npc.SkillTypeDDMagic2, casting3DDMagic()), 1000000)
					}
				}
				debuff := defaultnpc.SkillByTypeOrDefault(called, npc.SkillTypeDebuff, casting3Debuff())
				if s.Rnd(100) < 33 && s.AbnormalLevel(attacker, debuff) <= 0 {
					called.AddCastDesire(attacker, debuff, 1000000)
				}
			}
			base.ClanAttacked(s, e)
		},
	}).Named("PartyLeaderCasting3SkillsMagicalAggressive")
}

// behemothBomberID is the Dragon Bomber a Behemoth Dragon leads.
const behemothBomberID = 29070

// AntharasPrivateBehemothDragon is AntharasPrivateBehemothDragon: a
// PartyLeaderCasting3SkillsMagicalAggressive that leads 3, 5 or 7 Dragon
// Bombers, as its spawn's first parameter is 4, 5 or 6, none of which ever
// respawns, and leaves the world 240 minutes after it appears. Its created
// hook calls no parent: it creates no other privates and checks its
// territory once, after 120 s.
func AntharasPrivateBehemothDragon() script.Script {
	base := PartyLeaderCasting3SkillsMagicalAggressiveHooks()
	return script.Script{Dir: partyLeaderCasting3Dir, Behavior: true, NPCs: []int32{29069}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			switch n.Int(script.SlotParam1) {
			case 4:
				createBombers(s, n, behemothBomberID, 3)
			case 5:
				createBombers(s, n, behemothBomberID, 5)
			case 6:
				createBombers(s, n, behemothBomberID, 7)
			}
			if n.IntParam("MoveAroundSocial") > 0 || n.IntParam("ShoutMsg2") > 0 || n.IntParam("ShoutMsg3") > 0 {
				s.StartTimer("1001", n, nil, 10000*time.Millisecond)
			}
			s.StartTimer("1007", n, nil, 120000*time.Millisecond)
			s.StartTimer("1155", n, nil, 240*60*1000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			if strings.EqualFold(e.Name, "1155") {
				e.NPC.DeleteMe()
			}
			return base.Timer(s, e)
		},
	})}
}

// createBombers creates count privates of npcID around master, each set
// never to respawn.
func createBombers(s *script.Script, master *script.NPC, npcID int32, count int) {
	for range count {
		if bomber := s.Spawner().CreateOnePrivate(master, npcID, 0); bomber != nil {
			s.Spawner().SetRespawnDelay(bomber, 0)
		}
	}
}
