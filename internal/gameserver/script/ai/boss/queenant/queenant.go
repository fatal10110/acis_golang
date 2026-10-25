package queenant

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// QueenAnt is the grand boss Queen Ant. Appearing, she sends the players
// on her ground to one of three places, plays her music and calls her
// privates and her larva; every 10 seconds, while she wanders, she may
// play a social action. A dying nurse comes back after 10 seconds, any
// other private after 280 to 319. A hit from a player more than 8 levels
// above her wants her curse on that player and drops her hate of it; a hit
// while she or her attacker stands off her ground sends her back to her
// nest forgetting her targets. A playable's hit, or one on a member of her
// party or clan, may want one of her skills on it, by its distance, and
// makes her hate it by the damage. She curses the caster of a spell she
// sees when it is more than 8 levels above her, and may answer a hostile
// spell; leaving her ground sends her home; dying, she plays her death
// music.
func QueenAnt() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: "ai/boss/queenant", Behavior: true, NPCs: []int32{29001}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			if s.Rnd(100) < 33 {
				s.TeleportPlayersInTerritory(n, script.Loc{X: -19480, Y: 187344, Z: -5600}, 200)
			} else if s.Rnd(100) < 50 {
				s.TeleportPlayersInTerritory(n, script.Loc{X: -17928, Y: 180912, Z: -5520}, 200)
			} else {
				s.TeleportPlayersInTerritory(n, script.Loc{X: -23808, Y: 182368, Z: -5600}, 200)
			}

			n.PlayMusic("BS01_A")

			n.SetInt(script.SlotWeightPoint, 10)

			defaultnpc.CreatePrivates(s, n)

			s.Spawner().CreateOnePrivateEx(n, 29002, script.Loc{X: -21600, Y: 179482, Z: -5846, Heading: s.Rnd(360)}, 0, script.Params{})

			s.StartTimerAtFixedRate("1001", n, nil, 10000*time.Millisecond, 10000*time.Millisecond)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			if strings.EqualFold(e.Name, "1001") {
				if n.Intention() == script.IntentionWander && s.Rnd(100) < 30 {
					social := 4
					if s.RndBool() {
						social = 3
					}
					n.AddSocialDesire(social, (50*1000)/30, 30)
				}
			}
			return base.Timer(s, e)
		},
		OnPartyDied: func(s *script.Script, e script.PartyDied) {
			caller := e.Caller
			if !script.Same(caller, e.Called) {
				if caller.NpcID() == 29003 {
					s.Spawner().ScheduleRespawn(caller, 10000*time.Millisecond)
				} else {
					s.Spawner().ScheduleRespawn(caller, time.Duration(280+s.Rnd(40))*1000*time.Millisecond)
				}
			}
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n, attacker := e.NPC, e.Attacker
			if attacker.Level() > n.Level()+8 {
				n.AddCastDesire(attacker, skill.Ref{ID: 4515, Level: 1}, 1000000)

				n.StopHate(attacker)
			}

			if !n.InMyTerritory() || !n.InSpawnTerritory(attacker) {
				n.TeleportTo(script.Loc{X: -21610, Y: 181594, Z: -5734})
				n.RemoveAllAttackDesire()
			}

			if attacker.Playable() {
				if script.Riding(attacker) && s.AbnormalLevel(attacker, skill.Ref{ID: 4258, Level: 1}) <= 0 {
					n.AddCastDesire(attacker, skill.Ref{ID: 4258, Level: 1}, 1000000)
				}

				if e.Skill != (skill.Ref{}) && s.Skill(e.Skill).Element() == skill.ElementFire && s.Rnd(100) < 70 {
					n.AddCastDesire(attacker, skill.Ref{ID: 4018, Level: 1}, 1000000)
				} else {
					dist := n.Distance2D(attacker)
					if dist > 500 && s.Rnd(100) < 10 {
						n.AddCastDesireHold(attacker, skill.Ref{ID: 4019, Level: 1}, 1000000)
					} else if dist > 150 && s.Rnd(100) < 10 {
						if s.Rnd(100) < 80 {
							n.AddCastDesireHold(attacker, skill.Ref{ID: 4018, Level: 1}, 1000000)
						} else {
							n.AddCastDesireHold(attacker, skill.Ref{ID: 4019, Level: 1}, 1000000)
						}
					} else if dist < 250 && s.Rnd(100) < 5 {
						n.AddCastDesireHold(n, skill.Ref{ID: 4017, Level: 1}, 1000000)
					} else if s.Rnd(100) < 1 {
						n.AddSocialDesire(1, (60*1000)/30, 3000000)
					}
				}

				damage := float64(e.Damage)
				n.AddAttackDesireHold(attacker, float64(commons.TruncInt32Sat(damage/float64(n.MaxHP())/0.05*damage*float64(n.Int(script.SlotWeightPoint))*1000)))

				if top := n.TopDesireTarget(); top != nil && !n.CanAutoAttack(top) {
					n.StopHate(top)
				}
			}
		},
		OnPartyAttacked: func(s *script.Script, e script.PartyAttacked) {
			caller, called, target := e.Caller, e.Called, e.Target
			if target.Playable() && !script.Same(caller, called) {
				dist := called.Distance2D(target)
				if dist > 500 && s.Rnd(100) < 5 {
					called.AddCastDesireHold(target, skill.Ref{ID: 4019, Level: 1}, 1000000)
				} else if dist > 150 && s.Rnd(100) < 5 {
					if s.Rnd(100) < 80 {
						called.AddCastDesireHold(target, skill.Ref{ID: 4018, Level: 1}, 1000000)
					} else {
						called.AddCastDesireHold(target, skill.Ref{ID: 4019, Level: 1}, 1000000)
					}
				} else if dist < 250 && s.Rnd(100) < 2 {
					called.AddCastDesireHold(called, skill.Ref{ID: 4017, Level: 1}, 1000000)
				}

				damage := float64(e.Damage)
				called.AddAttackDesireHold(target, float64(commons.TruncInt32Sat(damage/float64(called.MaxHP())/0.05*damage*float64(caller.Int(script.SlotWeightPoint))*1000)))
			}
		},
		OnClanAttacked: func(s *script.Script, e script.ClanAttacked) {
			called, attacker := e.Called, e.Attacker
			if attacker.Playable() {
				dist := called.Distance2D(attacker)
				if dist > 500 && s.Rnd(100) < 3 {
					called.AddCastDesireHold(attacker, skill.Ref{ID: 4019, Level: 1}, 1000000)
				} else if dist > 150 && s.Rnd(100) < 3 {
					if s.Rnd(100) < 80 {
						called.AddCastDesireHold(attacker, skill.Ref{ID: 4018, Level: 1}, 1000000)
					} else {
						called.AddCastDesireHold(attacker, skill.Ref{ID: 4019, Level: 1}, 1000000)
					}
				} else if dist < 250 && s.Rnd(100) < 2 {
					called.AddCastDesireHold(called, skill.Ref{ID: 4017, Level: 1}, 1000000)
				}

				called.AddAttackDesireHold(attacker, float64(commons.TruncInt32Sat(float64(e.Damage)/float64(called.MaxHP())/0.05*500)))
			}
		},
		OnSeeSpell: func(s *script.Script, e script.SeeSpell) {
			n, caster := e.NPC, e.Caster
			if caster.Level() > n.Level()+8 {
				n.AddCastDesire(caster, skill.Ref{ID: 4215, Level: 1}, 1000000)
				return
			}

			if s.Skill(e.Skill).AggroPoints() > 0 && s.Rnd(100) < 15 {
				n.AddCastDesire(caster, skill.Ref{ID: 4018, Level: 1}, 1000000)
			}
		},
		OnOutOfTerritory: func(_ *script.Script, e script.OutOfTerritory) {
			n := e.NPC
			n.TeleportTo(script.Loc{X: -21610, Y: 181594, Z: -5734})
			n.RemoveAllAttackDesire()
		},
		OnMyDying: func(_ *script.Script, e script.MyDying) {
			e.NPC.PlayMusic("BS02_D")
		},
	})}
}
