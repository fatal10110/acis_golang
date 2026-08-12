package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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

// petRestoreTimeout bounds the pet-restore DB read: it runs on the cast's
// Hit-phase timer, not a request goroutine, so there's no connection-scoped
// context to cancel it if the owner disconnects mid-cast. Mirrors
// taskeffects.go's autosaveSaveTimeout for the same "bound an off-request
// DB read" shape.
const petRestoreTimeout = 5 * time.Second

// SpawnPet resolves controlItem's saved or default pet state, spawns it
// beside the owner, and registers it as the owner's active summon,
// mirroring SummonCreature.java:44-76. It sends SUMMON_ONLY_ONE and reports
// false if the owner already has a pet or servitor tracked — the reference
// re-checks this at the handler layer even though SummonItems.java already
// gated it once before the cast started.
//
// Every other rejection below (missing template, unmapped or non-pet
// summon item, missing npc template, a restore-state error, ID exhaustion,
// an unresolvable level) sends no further packet and only reports false.
// This runs inside the cast's already-committed Hit phase — MagicSkillUse,
// the SUMMON_A_PET system message and MagicSkillLaunched are already sent
// by the caller before SpawnPet runs — and Java's own handler is silent
// for the identical set of conditions (SummonCreature.java:36,40,44,49,54,
// 59 are all bare `return;`, with no packet beyond what the cast itself
// already sent).
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

	restoreCtx, cancel := context.WithTimeout(context.Background(), petRestoreTimeout)
	defer cancel()
	state, hasSaved, err := link.petStore.Get(restoreCtx, controlItem.ObjectID)
	if err != nil {
		link.log.Error().Err(err).Int32("item_obj_id", controlItem.ObjectID).Msg("summon: pet restore failed")
		return false
	}

	// Exp/SP restore and persistence writeback (Pet.java's other saved
	// fields) are deferred with the rest of the pet-relog-persistence
	// follow-up — see this PR's linked issue. Level/Name/Fed/HP/MP are
	// restored here because summon.Actor already exposes somewhere to put
	// them.
	level := petmodel.InitialLevel(int(summonItem.NPCID), npcTmpl.Level, live.LevelValue())
	if hasSaved {
		level = state.Level
	}
	levelStats, ok := npcTmpl.Pet.Levels[level]
	if !ok {
		// A corrupted/out-of-range saved level, or a template with no
		// stat row for its own declared level: reject rather than spawn
		// with zero-value combat/feeding stats, matching Pet.restore
		// returning null on bad data (SummonCreature.java:59's pet==null
		// check, itself a silent no-op — see this file's own SpawnPet doc).
		return false
	}
	fed, curHP, curMP := levelStats.MaxMeal, levelStats.MaxHP, levelStats.MaxMP
	if hasSaved {
		fed, curHP, curMP = state.Fed, state.CurHP, state.CurMP
	}

	objID, err := link.ids.NextID()
	if err != nil {
		return false
	}

	name := npcTmpl.Name
	if hasSaved && state.Name != "" {
		name = state.Name
	}

	pet := link.newPet(summon.PetConfig{
		ObjectID:        objID,
		Owner:           live,
		ControlItemID:   controlItem.ObjectID,
		NPCID:           int(summonItem.NPCID),
		CollisionRadius: npcTmpl.CollisionRadius,
		Name:            name,
		Level:           level,
		Exp:             state.Exp,
		SP:              state.SP,
		CON:             npcTmpl.CON,
		Config:          nil, // set by newPet from link.petConfig
		Inventory:       itemcontainer.NewPetInventory(objID, live.Inventory().Templates()),
		Fed:             fed,
		MaxMeal:         levelStats.MaxMeal,
		MealInNormal:    levelStats.MealInNormal,
		MealInBattle:    levelStats.MealInBattle,
		Food1:           int32(npcTmpl.Pet.Food1),
		Food2:           int32(npcTmpl.Pet.Food2),
		AutoFeedLimit:   npcTmpl.Pet.AutoFeedLimit,
		HungryLimit:     npcTmpl.Pet.HungryLimit,
		UnsummonLimit:   npcTmpl.Pet.UnsummonLimit,
		Stats: summon.CombatStats{
			STR: npcTmpl.STR, CON: npcTmpl.CON, DEX: npcTmpl.DEX,
			INT: npcTmpl.INT, WIT: npcTmpl.WIT, MEN: npcTmpl.MEN,
			PAtk: levelStats.PAtk, PDef: levelStats.PDef,
			MAtk: levelStats.MAtk, MDef: levelStats.MDef,
			MaxHP: levelStats.MaxHP, MaxMP: levelStats.MaxMP,
			SSCount: levelStats.SSCount, SPSCount: levelStats.SPSCount,
			AttackRange: npcTmpl.BaseAttackRange, AttackSpeed: npcTmpl.AtkSpd,
		},
		Skills: npcTmpl.Skills,
	})
	pet.SetZones(link.zones)
	pet.SetHP(curHP)
	// Java's Servitor/Pet construction sets max HP/MP before restoring
	// saved current values (Pet.java:552-556); NewPet already seeds
	// current HP/MP at max, so a restored value only needs applying when
	// it differs from that default.
	if hasSaved {
		if curMP < pet.MPValue() {
			pet.ReduceMP(pet.MPValue() - curMP)
		} else {
			pet.AddMP(curMP - pet.MPValue())
		}
	}

	// Combat AI wiring (owner-commanded attack/follow execution against a
	// real move/attack controller, and the cast controller that would let
	// TryUseSkill's dispatched cast actually resolve) is deferred — see
	// this PR's linked follow-up. Attaching AI here with inert controllers
	// still gives TryUseSkill a non-nil brain, matching its own documented
	// contract: "a dispatched cast reports true even if the AI goes on to
	// reject it" (live_accessors.go), which is exactly what a cast
	// controller-less ai.Summon already does by design.
	//
	// SetAI must run before SpawnBesideOwner publishes pet into world.State:
	// SpawnBesideOwner's registry writes take a mutex, giving a
	// happens-before edge to any other goroutine's registry read (e.g. the
	// connection goroutine looking the pet up to dispatch TryUseSkill).
	// Setting brain first means that edge also covers the brain field,
	// instead of leaving it as an unsynchronized write racing that read.
	link.wireSummonAI(pet)

	offset := location.Location{X: petSpawnOffset, Y: 0, Z: 0}
	summon.SpawnBesideOwner(link.world, pet, live, offset)
	link.broadcastSummonSpawnRelation(live, pet)

	return true
}

// SpawnServitor creates the non-cubic SUMMON skill's live servitor beside its
// owner. The cast handler supplies the skill definition, whose NpcID selects
// the servitor template.
func (s *gameSummonSpawner) SpawnServitor(owner *player.Character, def modelskill.Definition) bool {
	link, live := s.link, s.live
	if link == nil || live == nil || owner == nil || def.NpcID == 0 {
		return false
	}
	if _, ok := link.world.Summon(live.ObjectID()); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonOnlyOne))
		return false
	}
	npcTmpl, ok := link.npcs.Get(def.NpcID)
	if !ok {
		return false
	}
	objID, err := link.ids.NextID()
	if err != nil {
		return false
	}

	servitor := summon.NewServitor(summon.ServitorConfig{
		ObjectID:        objID,
		Owner:           live,
		NPCID:           def.NpcID,
		CollisionRadius: npcTmpl.CollisionRadius,
		Name:            npcTmpl.Name,
		Level:           npcTmpl.Level,
		OwnerInventory:  live.Inventory(),
		Lifetime: summon.LifetimeState{
			TimeRemaining:    def.SummonTotalLifeTime,
			TotalLifeTime:    def.SummonTotalLifeTime,
			ItemConsumeSteps: 0,
		},
		Stats: summon.CombatStats{
			STR: npcTmpl.STR, CON: npcTmpl.CON, DEX: npcTmpl.DEX,
			INT: npcTmpl.INT, WIT: npcTmpl.WIT, MEN: npcTmpl.MEN,
			PAtk: npcTmpl.PAtk, PDef: npcTmpl.PDef, MAtk: npcTmpl.MAtk, MDef: npcTmpl.MDef,
			MaxHP: npcTmpl.HPMax, MaxMP: npcTmpl.MPMax,
			BaseRandomDamage: npcTmpl.BaseRandomDamage,
			SSCount:          npcTmpl.SSCount,
			SPSCount:         npcTmpl.SPSCount,
			AttackRange:      npcTmpl.BaseAttackRange,
			AttackSpeed:      npcTmpl.AtkSpd,
		},
		Skills: npcTmpl.Skills,
	})
	servitor.SetZones(link.zones)
	link.wireSummonAI(servitor)
	summon.SpawnBesideOwner(link.world, servitor, live, location.Location{X: petSpawnOffset})
	link.broadcastSummonSpawnRelation(live, servitor)
	return true
}

func (l *GameClientLink) wireSummonAI(actor *summon.Actor) {
	brain := ai.NewSummon(actor, inertSummonMoveController{}, inertSummonAttackController{})
	castController := actorcast.NewController(actorcast.SummonActor{Summon: actor})
	castController.SetLogger(l.log)
	brain.SetCastController(&actorcast.AIController{
		Controller:  castController,
		Definitions: l.skills,
		Effects:     actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
		Caster:      actor,
	})
	actor.SetAI(brain)
	actor.SetStatusUpdater(func() { l.broadcastSummonStatus(actor) })
	actor.SetDamageNotifier(func(attackerName string, damage int32) {
		owner, ok := l.livePlayerByID(actor.OwnerID())
		if !ok {
			return
		}
		messageID := serverpackets.SystemMessageSummonReceivedS2ByS1
		if actor.IsPet() {
			messageID = serverpackets.SystemMessagePetReceivedS2DamageByS1
		}
		owner.SendFrame(serverpackets.FrameSystemMessageStringNumber(messageID, attackerName, damage))
	})
}

func (l *GameClientLink) broadcastSummonStatus(actor *summon.Actor) {
	if actor == nil {
		return
	}
	owner, ok := actor.ActingPlayer().(*livePlayer)
	if !ok {
		return
	}
	status, ok := petInfoSnapshot(actor, owner, owner.npcs)
	if !ok {
		return
	}
	owner.SendFrame(serverpackets.FramePetStatusUpdate(status))
	if l.world == nil {
		return
	}
	info, ok := summonInfoSnapshot(actor, owner.npcs)
	if !ok {
		return
	}
	broadcastFrame(func() wire.Frame {
		return serverpackets.FrameNPCInfo(info)
	}, func(send func(frameReceiver)) {
		l.world.ForEachKnown(actor, func(object world.Tracked) {
			if object.ObjectID() == owner.ObjectID() {
				return
			}
			if receiver, ok := object.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
}

// inertSummonMoveController and inertSummonAttackController satisfy
// ai.Summon's move/attack surfaces with safe no-op behavior: every method
// reports "can't/didn't act". Real combat AI (tryToAttack/tryToFollow
// actually moving and swinging) is deferred to the follow-up this PR links
// — see gameSummonSpawner.SpawnPet's own comment.
type inertSummonMoveController struct{}

func (inertSummonMoveController) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (inertSummonMoveController) MoveHome(location.Location) error { return nil }
func (inertSummonMoveController) Stop() error                      { return nil }
func (inertSummonMoveController) MaybeStartFriendlyFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}

type inertSummonAttackController struct{}

func (inertSummonAttackController) BowCoolingDown() bool                { return false }
func (inertSummonAttackController) AttackingNow() bool                  { return false }
func (inertSummonAttackController) CanAttack(attackable.Combatant) bool { return false }
func (inertSummonAttackController) DoAttack(attackable.Combatant) error { return nil }
