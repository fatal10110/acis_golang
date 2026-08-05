package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// gameSummonSpawner is the network layer's player.SummonSpawner: it has the
// world, npc templates, summon-item table and pet persistence the domain
// layer intentionally doesn't depend on directly (mirrors castController's
// split for the same reason). One is wired per connected live player.
type gameSummonSpawner struct {
	link *GameClientLink
	live *livePlayer
}

var _ player.SummonSpawner = (*gameSummonSpawner)(nil)

const petSpawnOffset = 40

// SpawnPet resolves controlItem's saved or default pet state, spawns it
// beside the owner, and registers it as the owner's active summon,
// mirroring SummonCreature.java:44-76. It sends SUMMON_ONLY_ONE and reports
// false if the owner already has a pet or servitor tracked — the reference
// re-checks this at the handler layer even though SummonItems.java already
// gated it once before the cast started.
func (s *gameSummonSpawner) SpawnPet(owner *player.Character, controlItem *item.Instance) bool {
	link, live := s.link, s.live
	if link == nil || live == nil || controlItem == nil {
		return false
	}
	if _, ok := link.world.Summon(live.ObjectID()); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonOnlyOne))
		return false
	}

	tmpl, ok := live.Inventory().Templates().Get(controlItem.TemplateID)
	if !ok {
		return false
	}
	summonItem, ok := link.summonItems.Item(tmpl.ID)
	if !ok || summonItem.SummonType != summonItemTypePet {
		return false
	}
	npcTmpl, ok := link.npcs.Get(int(summonItem.NPCID))
	if !ok || npcTmpl.Pet == nil {
		return false
	}

	state, hasSaved, err := link.petStore.Get(context.Background(), controlItem.ObjectID)
	if err != nil {
		link.log.Error().Err(err).Int32("item_obj_id", controlItem.ObjectID).Msg("summon: pet restore failed")
		return false
	}

	// Name/Exp/SP restore and persistence writeback (Pet.java's other
	// saved fields) are deferred with the rest of the pet-relog-persistence
	// follow-up — see this PR's linked issue. Level/Fed/HP/MP are restored
	// here because summon.Actor already exposes somewhere to put them.
	level := petmodel.InitialLevel(int(summonItem.NPCID), npcTmpl.Level, live.LevelValue())
	fed := npcTmpl.Pet.Levels[level].MaxMeal
	var curHP, curMP float64
	if hasSaved {
		level = state.Level
		fed = state.Fed
		curHP, curMP = state.CurHP, state.CurMP
	}
	levelStats := npcTmpl.Pet.Levels[level]
	if !hasSaved {
		curHP, curMP = levelStats.MaxHP, levelStats.MaxMP
	}

	objID, err := link.ids.NextID()
	if err != nil {
		return false
	}

	pet := link.newPet(summon.PetConfig{
		ObjectID: objID,
		Owner:    live,
		NPCID:    int(summonItem.NPCID),
		Level:    level,
		CON:      npcTmpl.CON,
		Config:   nil, // set by newPet from link.petConfig
		// Inventory is left nil: the pet's own carried-item inventory
		// (paperdoll/warehouse-adjacent, separate from the collar) is
		// deferred — see this PR's linked follow-up.
		Fed:           fed,
		MaxMeal:       levelStats.MaxMeal,
		MealInNormal:  levelStats.MealInNormal,
		MealInBattle:  levelStats.MealInBattle,
		Food1:         int32(npcTmpl.Pet.Food1),
		Food2:         int32(npcTmpl.Pet.Food2),
		AutoFeedLimit: npcTmpl.Pet.AutoFeedLimit,
		HungryLimit:   npcTmpl.Pet.HungryLimit,
		UnsummonLimit: npcTmpl.Pet.UnsummonLimit,
		Stats: summon.CombatStats{
			STR: npcTmpl.STR, CON: npcTmpl.CON, DEX: npcTmpl.DEX,
			INT: npcTmpl.INT, WIT: npcTmpl.WIT, MEN: npcTmpl.MEN,
			PAtk: levelStats.PAtk, PDef: levelStats.PDef,
			MAtk: levelStats.MAtk, MDef: levelStats.MDef,
			MaxHP: levelStats.MaxHP, MaxMP: levelStats.MaxMP,
			SSCount: levelStats.SSCount, SPSCount: levelStats.SPSCount,
			AttackRange: npcTmpl.BaseAttackRange,
		},
		Skills: npcTmpl.Skills,
	})
	pet.SetHP(curHP)
	// Java's Servitor/Pet construction sets max HP/MP before restoring
	// saved current values (Pet.java:552-556); NewPet already seeds
	// current HP/MP at max, so a restored value only needs applying when
	// it differs from that default.
	if hasSaved {
		pet.AddMP(curMP - levelStats.MaxMP)
	}

	offset := location.Location{X: petSpawnOffset, Y: 0, Z: 0}
	summon.SpawnBesideOwner(link.world, pet, live, offset)

	// Combat AI wiring (owner-commanded attack/follow execution against a
	// real move/attack controller, and the cast controller that would let
	// TryUseSkill's dispatched cast actually resolve) is deferred — see
	// this PR's linked follow-up. Attaching AI here with inert controllers
	// still gives TryUseSkill a non-nil brain, matching its own documented
	// contract: "a dispatched cast reports true even if the AI goes on to
	// reject it" (live_accessors.go), which is exactly what a cast
	// controller-less ai.Summon already does by design.
	pet.SetAI(ai.NewSummon(pet, inertSummonMoveController{}, inertSummonAttackController{}))

	return true
}

// inertSummonMoveController and inertSummonAttackController satisfy
// ai.Summon's move/attack surfaces with safe no-op behavior: every method
// reports "can't/didn't act". Real combat AI (tryToAttack/tryToFollow
// actually moving and swinging) is deferred to the follow-up this PR links
// — see gameSummonSpawner.SpawnPet's own comment.
type inertSummonMoveController struct{}

func (inertSummonMoveController) MaybeStartOffensiveFollow(attackable.Combatant, int) bool {
	return false
}
func (inertSummonMoveController) MoveHome(location.Location) {}
func (inertSummonMoveController) Stop()                      {}
func (inertSummonMoveController) MaybeStartFriendlyFollow(attackable.Combatant, int) bool {
	return false
}

type inertSummonAttackController struct{}

func (inertSummonAttackController) BowCoolingDown() bool                { return false }
func (inertSummonAttackController) AttackingNow() bool                  { return false }
func (inertSummonAttackController) CanAttack(attackable.Combatant) bool { return false }
func (inertSummonAttackController) DoAttack(attackable.Combatant)       {}
