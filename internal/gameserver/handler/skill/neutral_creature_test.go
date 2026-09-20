package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/skill/skilltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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
func (neutralPlayer) Revive(float64) bool                            { return false }
func (neutralPlayer) RestoreExp(float64)                             {}
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
func (neutralPlayer) NotifySpoilAlready()                            {}
func (neutralPlayer) NotifySpoilSuccess()                            {}
func (neutralPlayer) Mounted() bool                                  { return false }
func (neutralPlayer) OlympiadMode() bool                             { return false }
func (neutralPlayer) ObserverMode() bool                             { return false }
func (neutralPlayer) NoSummonFriendZone() bool                       { return false }

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
