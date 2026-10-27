package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/skill/skilltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// neutralCreature supplies neutral values for every Creature method except
// the world placement ones: embed it next to world.Presence in a test double
// and override only what the test exercises. The neutral creature cannot be
// rolled against, never reflects or blocks, and is not an NPC.
type neutralCreature struct {
	skilltest.Creature
}

// neutralPlayer adds neutral values for the player-only cast surface on top
// of neutralCreature, so a player-kind double only overrides what its test
// exercises.
type neutralPlayer struct {
	neutralCreature
}

func (neutralPlayer) Kind() actor.Kind                               { return actor.KindPlayer }
func (neutralPlayer) CP() float64                                    { return 0 }
func (neutralPlayer) MaxCPValue() float64                            { return 0 }
func (neutralPlayer) SetCP(float64)                                  {}
func (neutralPlayer) BreakCastOnDamage(float64)                      {}
func (neutralPlayer) Charges() int                                   { return 0 }
func (neutralPlayer) ReviveRestoringExp(float64) bool                { return false }
func (neutralPlayer) CursedWeaponEquipped() bool                     { return false }
func (neutralPlayer) Operating() bool                                { return false }
func (neutralPlayer) Rooted() bool                                   { return false }
func (neutralPlayer) InCombat() bool                                 { return false }
func (neutralPlayer) FestivalParticipant() bool                      { return false }
func (neutralPlayer) NotifyHPRestored(string, int, bool)             {}
func (neutralPlayer) NotifyMPRestored(string, int, bool)             {}
func (neutralPlayer) NotifyCPRestored(string, int, bool)             {}
func (neutralPlayer) NotifyAttackFailed()                            {}
func (neutralPlayer) NotifyResistedSkill(string, modelskill.ID, int) {}
func (neutralPlayer) NotifyResistedMagic(string)                     {}
func (neutralPlayer) NotifySkillDamage(int32, int, bool, bool, bool) {}
func (neutralPlayer) NotifySpoilAlready()                            {}
func (neutralPlayer) NotifySpoilSuccess()                            {}
func (neutralPlayer) Mounted() bool                                  { return false }
func (neutralPlayer) OlympiadMode() bool                             { return false }
func (neutralPlayer) ObserverMode() bool                             { return false }
func (neutralPlayer) NoSummonFriendZone() bool                       { return false }
func (neutralPlayer) AddExpAndSp(int64, int)                         {}
func (neutralPlayer) ReviveRequest(player.Reviver, float64, bool)    {}
func (neutralPlayer) NotifyEffectRemovedDueLackMP(*effect.Effect)    {}

// neutralNPC adds the NPC-only cast surface on top of neutralCreature.
type neutralNPC struct {
	neutralCreature
}

func (neutralNPC) Kind() actor.Kind           { return actor.KindNPC }
func (neutralNPC) Lethalable() bool           { return true }
func (neutralNPC) SpoilPool() *item.SpoilPool { return nil }

// SeedState: the neutral NPC was never sown, so the manor handlers find no
// lifecycle to act on.
func (neutralNPC) SeedState() *npc.SeedState { return nil }

// MonsterKind: the neutral NPC is no Monster.
func (neutralNPC) MonsterKind() bool { return false }

// The neutral NPC keeps no hate and is no chest.
func (neutralNPC) AggroHate(attackable.Combatant) float64               { return 0 }
func (neutralNPC) Unlockable() bool                                     { return false }
func (neutralNPC) Box() bool                                            { return false }
func (neutralNPC) Interacted() bool                                     { return false }
func (neutralNPC) ClaimInteraction() bool                               { return false }
func (neutralNPC) AddAttackDesire(attackable.Combatant, float64)        {}
func (neutralNPC) AddDamageHate(attackable.Combatant, float64, float64) {}
func (neutralNPC) DeleteMe()                                            {}

// neutralSummon adds the summon-only cast surface on top of
// neutralCreature: an ownerless servitor that ignores revives and grants.
type neutralSummon struct {
	neutralCreature
}

func (neutralSummon) Kind() actor.Kind                { return actor.KindSummon }
func (neutralSummon) SiegeSummon() bool               { return false }
func (neutralSummon) SummonOwner() summon.Owner       { return nil }
func (neutralSummon) UnSummon(summon.Owner)           {}
func (neutralSummon) IsPet() bool                     { return false }
func (neutralSummon) AddExpAndSp(int64, int)          {}
func (neutralSummon) ReviveRestoringExp(float64) bool { return false }
func (neutralSummon) ResurrectOutright(float64)       {}
