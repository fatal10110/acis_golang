package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	actorcast "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cast"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// gameSummonSpawner spawns a live player's pet or servitor for its summon
// request events: it has the world, npc templates, summon-item table and
// pet persistence the domain layer intentionally doesn't depend on directly.
// One is created per connected live player.
type gameSummonSpawner struct {
	link *GameClientLink
	live *livePlayer
}

const petSpawnOffset = 40

// petRestoreTimeout bounds the pet-restore DB read: it runs on the cast's
// Hit-phase timer, not a request goroutine, so there's no connection-scoped
// context to cancel it if the owner disconnects mid-cast. Mirrors
// taskeffects.go's autosaveSaveTimeout for the same "bound an off-request
// DB read" shape.
const petRestoreTimeout = 5 * time.Second

// SpawnPet resolves controlItem's saved or default pet state, spawns it
// beside the owner, and registers it as the owner's active summon,
// mirroring SummonCreature.java:44-76. It sends SUMMON_ONLY_ONE and stops if
// the owner already has a pet or servitor tracked — the reference re-checks
// this at the handler layer even though SummonItems.java already gated it
// once before the cast started.
//
// Every other rejection below (missing template, unmapped or non-pet
// summon item, missing npc template, a restore-state error, ID exhaustion,
// an unresolvable level) sends no further packet and simply stops. This runs
// inside the cast's already-committed Hit phase — MagicSkillUse, the
// SUMMON_A_PET system message and MagicSkillLaunched are already sent by the
// caller before SpawnPet runs — and Java's own handler is silent for the
// identical set of conditions (SummonCreature.java:36,40,44,49,54, 59 are
// all bare `return;`, with no packet beyond what the cast itself already
// sent).
//
// When the pets row has to be read, the spawn finishes on the owner's queue
// after the read (see spawnRestoredPet), so it reports nothing to its caller.
func (s *gameSummonSpawner) SpawnPet(owner *player.Character, controlItem *item.Instance) {
	link, live := s.link, s.live
	if link == nil || live == nil || controlItem == nil {
		return
	}
	if _, ok := link.world.Summon(live.ObjectID()); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonOnlyOne))
		return
	}

	tmpl, ok := live.Inventory().Templates().Get(controlItem.TemplateID)
	if !ok {
		return
	}
	summonItem, ok := link.summonItems.Item(tmpl.ID)
	if !ok || summonItem.SummonType != summonItemTypePet {
		return
	}
	npcTmpl, ok := link.npcs.Get(int(summonItem.NPCID))
	if !ok || npcTmpl.Pet == nil {
		return
	}

	// An unsummon or logout whose pets-row write is still queued restores
	// from the state it queued; otherwise the row has to be read.
	if state, hasSaved := link.queuedPets.latest(controlItem.ObjectID); hasSaved {
		s.spawnRestoredPet(controlItem, summonItem, npcTmpl, state, true)
		return
	}
	// The read runs on the control item's persistence lane: behind every
	// pets-row write queued for that item, and off this actor queue, where a
	// slow database would hold a pool worker and stall unrelated actors. The
	// spawn continues on the owner's queue with the row, so the packets it
	// sends keep their own order; a closed queue (the owner logged out while
	// the read ran) drops it.
	link.persist.Enqueue(controlItem.ObjectID, func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), petRestoreTimeout)
		defer cancel()
		state, hasSaved, err := link.petStore.Get(restoreCtx, controlItem.ObjectID)
		if err != nil {
			link.log.Error().Err(err).Int32("item_obj_id", controlItem.ObjectID).Msg("summon: pet restore failed")
			return
		}
		postLive(live, func() { s.spawnRestoredPet(controlItem, summonItem, npcTmpl, state, hasSaved) })
	})
}

// spawnRestoredPet builds and publishes the pet from its resolved pets-row
// state, on the owner's queue. It re-checks both gates that the caster's own
// state can have invalidated while the pets-row read was outstanding: the
// control item still being held, and no other summon having reached the
// world.
//
// The reference re-resolves the control item from the caster's inventory at
// use time and drops out silently when it is gone or no longer theirs
// (SummonCreature.java:34-41). Go needs that check on this side of the read:
// the read runs off the owner's queue, so the owner's own handlers — drop,
// destroy, a trade transfer — can run between the cast's Hit phase and this
// task, and a pet built from a collar someone else now holds would answer to
// two players through one pets row.
//
// A logout in that window is the same problem one step further: sim.Queue
// refuses later posts but still runs every task it has already accepted, so a
// continuation queued just before detachLivePlayer closed the queue would run
// after the session left the world — publishing a pet for an offline owner,
// past the only cleanup that would have removed it. The reference cannot
// reach that state: Player.cleanup aborts the cast and unsummons the pet
// (Player.java:6266-6283), and its spawn had no separate continuation to
// leave behind. The detaching flag is the same one taskeffects.go checks
// before applying a deferred effect to a departing session.
func (s *gameSummonSpawner) spawnRestoredPet(controlItem *item.Instance, summonItem item.SummonItem, npcTmpl *npc.Template, state petmodel.State, hasSaved bool) {
	link, live := s.link, s.live
	if live.detached() {
		return
	}
	inv := live.Inventory()
	if inv == nil || inv.ItemByObjectID(controlItem.ObjectID) == nil {
		return
	}
	if _, ok := link.world.Summon(live.ObjectID()); ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSummonOnlyOne))
		return
	}

	// Java's unsaved branch commits the seeded row immediately
	// (Pet.java:554's pet.store()); Go defers that first write to the
	// first savePet instead — a deliberate difference locked in by this
	// suite's "no pets row until a save point" assertions. Level/Name/
	// Fed/HP/MP/Exp/SP are restored here because summon.Actor already
	// exposes somewhere to put them. Java's saved-row dead check
	// (Pet.java:540-544: curHp < 0.5 restores dead and skips regen) has
	// no Go counterpart yet — summon.Actor has no dead state or regen
	// task at all — tracked as #2307.
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
		return
	}
	fed, curHP, curMP := levelStats.MaxMeal, levelStats.MaxHP, levelStats.MaxMP
	exp := levelStats.MaxExp
	if hasSaved {
		fed, curHP, curMP = state.Fed, state.CurHP, state.CurMP
		exp = state.Exp
	}

	objID, err := link.ids.NextID()
	if err != nil {
		return
	}

	name := npcTmpl.Name
	named := hasSaved && state.Name != ""
	if named {
		name = state.Name
	}

	// food1/food2 restore different amounts: each maps to its own feed
	// skill (PetFoods.java's hardcoded item->skill map), and those skills'
	// Feed values differ (e.g. Strider's food vs Clan Hall Strider's food).
	foodRestore1, _ := petFoodFeedAmount(link.skills, link.petConfig.FoodRate, int32(npcTmpl.Pet.Food1))
	var foodRestore2 int
	if npcTmpl.Pet.Food2 != 0 {
		foodRestore2, _ = petFoodFeedAmount(link.skills, link.petConfig.FoodRate, int32(npcTmpl.Pet.Food2))
	}

	pet, err := link.newPet(summon.PetConfig{
		ObjectID:        objID,
		Owner:           live,
		ControlItemID:   controlItem.ObjectID,
		OwnerInventory:  live.Inventory(),
		NPCID:           int(summonItem.NPCID),
		CollisionRadius: npcTmpl.CollisionRadius,
		CollisionHeight: npcTmpl.CollisionHeight,
		Name:            name,
		Named:           named,
		Level:           level,
		MaxBuffsAmount:  link.playerConfig.MaxBuffsAmount,
		Exp:             exp,
		SP:              state.SP,
		ExpType:         levelStats.ExpType,
		Growth:          npcTmpl.Pet,
		CON:             npcTmpl.CON,
		Config:          nil, // set by newPet from link.petConfig
		Inventory:       itemcontainer.NewPetInventory(objID, live.Inventory().Templates()),
		Fed:             fed,
		MaxMeal:         levelStats.MaxMeal,
		MealInNormal:    levelStats.MealInNormal,
		MealInBattle:    levelStats.MealInBattle,
		Food1:           int32(npcTmpl.Pet.Food1),
		Food2:           int32(npcTmpl.Pet.Food2),
		FoodRestore1:    foodRestore1,
		FoodRestore2:    foodRestore2,
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
			CritRate: npcTmpl.CritRate,
		},
		Skills:    npcTmpl.Skills,
		Passives:  npcTmpl.Passives,
		SkillDefs: link.skills,
		Zones:     link.zones,
		LOS:       link.summonLineOfSight(),
	})
	if err != nil {
		return
	}
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
	// Attach must run before SpawnBesideOwner publishes pet into world.State:
	// SpawnBesideOwner's registry writes take a mutex, giving a
	// happens-before edge to any other goroutine's registry read (e.g. the
	// connection goroutine looking the pet up to dispatch TryUseSkill).
	// Setting brain first means that edge also covers the brain field,
	// instead of leaving it as an unsynchronized write racing that read.
	link.wireSummonAI(pet, npcTmpl.RunSpeed)
	pet.SyncControlItemEnchant()

	offset := location.Location{X: petSpawnOffset, Y: 0, Z: 0}
	summon.SpawnBesideOwner(link.world, pet, live, offset)
	pet.TryToFollow(live)
	link.broadcastSummonSpawnRelation(live, pet)

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

	servitor, err := summon.NewServitor(summon.ServitorConfig{
		ObjectID:        objID,
		Owner:           live,
		NPCID:           def.NpcID,
		CollisionRadius: npcTmpl.CollisionRadius,
		CollisionHeight: npcTmpl.CollisionHeight,
		Name:            npcTmpl.Name,
		Level:           npcTmpl.Level,
		MaxBuffsAmount:  link.playerConfig.MaxBuffsAmount,
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
			CritRate:         npcTmpl.CritRate,
		},
		Skills:    npcTmpl.Skills,
		Passives:  npcTmpl.Passives,
		SkillDefs: link.skills,
		Zones:     link.zones,
		LOS:       link.summonLineOfSight(),
	})
	if err != nil {
		return false
	}
	link.wireSummonAI(servitor, npcTmpl.RunSpeed)
	summon.SpawnBesideOwner(link.world, servitor, live, location.Location{X: petSpawnOffset})
	servitor.TryToFollow(live)
	link.broadcastSummonSpawnRelation(live, servitor)
	return true
}

// wireSummonAI attaches actor's AI brain, cast controller, status/damage
// notifiers, and returns the installed cast controller so a caller (or a
// test) can drive it directly rather than reaching into the brain's
// unexported state.
func (l *GameClientLink) wireSummonAI(actor *summon.Actor, speed ...float64) *actorcast.AIController {
	runSpeed := 0.0
	if len(speed) != 0 {
		runSpeed = speed[0]
	}
	sink := &summonSink{link: l, actor: actor}
	// A summon's work runs on its owner's queue.
	var queue *sim.Queue
	if owner, ok := liveSummonOwner(actor); ok {
		queue = owner.Queue()
	}
	if queue != nil {
		actor.SetQueue(queue)
	}
	moveController := ai.SummonMoveController(inertSummonMoveController{})
	if actor != nil && l.geo != nil {
		x, y, z := actor.Position()
		if err := actor.InitMovement(location.Location{X: x, Y: y, Z: z}, runSpeed, l.geo); err != nil {
			l.log.Warn().Err(err).Msg("summon: create movement controller")
		} else {
			setWaterSurface(actor.Move(), l.zones)
			if controller, err := move.NewController(actor.Move(), actor, sink); err != nil {
				l.log.Warn().Err(err).Msg("summon: attach movement controller")
			} else {
				controller.SetPositionUpdates(l.positions)
				moveController = controller
				sink.move = controller
			}
		}
	}
	attackController := attack.NewPlayable(actor, sink)
	attackController.SetLogger(l.log)
	if queue != nil {
		attackController.SetQueue(queue)
	}
	brain := ai.NewSummon(actor, moveController, attackController)
	sink.brain = brain
	actor.SetRaidCursesDisabled(l.disableRaidCurse)
	// SetLogger records broadcast errors from TryToAttack/TryToFollow/TryToIdle/Think
	// that have no caller left to return them to; left unset, they're silently
	// discarded through the zero-value zerolog.Logger.
	brain.SetLogger(l.log)
	// SetLogger records where a panic recovered from a scheduled
	// Launch/Hit/Finish callback (Controller.scheduleLocked's recover
	// wrapper, unconditional for every Controller) is logged; left unset,
	// it's silently discarded through the zero-value zerolog.Logger.
	// Player-owned controllers get the same wiring (live.cast.SetLogger /
	// c.SetLogger).
	castController := actorcast.NewController(actorcast.SummonActor{Summon: actor}, nil)
	castController.SetLogger(l.log)
	if queue != nil {
		castController.SetQueue(queue)
	}
	aiController := &actorcast.AIController{
		Controller:  castController,
		Definitions: l.skills,
		Effects:     actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
		Caster:      actor,
	}
	// Summon.sendPacket forwards every packet to the owner (base
	// Creature.sendPacket is a no-op), but Java only calls sendPacket
	// unconditionally for ATTACK_FAILED (Pdam.java:130, Manadam.java:44),
	// MISSED_TARGET (Manadam.java:44), and the Lethal Strike messages
	// (Formulas.java:242-244); the target-side LETHAL_STRIKE message is
	// itself Player-gated, so lethal.TargetID's lookup naturally covers only
	// real targets. S1_DODGES_ATTACK and S1_PERFORMING_COUNTERATTACK
	// (Blow.java:46-47,88-89) and the generic per-effect resisted message
	// (L2Skill.java:1196-1197) are all gated `instanceof Player` on the
	// caster/effector and never fire for a Summon at all in the reference —
	// but Mdam.java:69, Blow.java:74, Manadam.java:55, and
	// L2SkillChargeDmg.java:77 send S1_RESISTED_YOUR_S2 unconditionally for
	// the skill's own effect-landing resist, so that subset
	// (Resisted.Unconditional) is forwarded below
	// alongside AttackFailed/Lethals/MagicResists/ManaDamageMissed/ManaDrains
	// (ManaDrains is itself gated `target instanceof Player`,
	// Manadam.java:68, independent of caster type), but
	// YOUR_OPPONENTS_MP_WAS_REDUCED_BY_S1 (Manadam.java:72) is gated
	// `creature instanceof Player` on the caster and stays unforwarded; a
	// hostile NPC caster routes through DeliverHitResult (nil live) instead:
	// caster-addressed messages like this one are dropped there too, but
	// target-addressed ones still reach an online target (issue #2350).
	// AVOIDED_S1_ATTACK and COUNTERED_S1_ATTACK (Blow.java:49-50,85-86) are
	// gated on the *target* being a Player, independent of caster type, so
	// Dodges/Counterattacks are forwarded here too; sendSkillHandlerResult
	// resolves attacker/defender by ID regardless of the live argument, so
	// the caster-addressed halves (S1_DODGES_ATTACK,
	// S1_PERFORMING_COUNTERATTACK) still correctly stay dropped since the
	// summon itself is never resolvable as a livePlayer (issue #2353).
	aiController.OnHitResult = func(result actorcast.EffectResult) {
		owner, ok := l.livePlayerByID(actor.OwnerID())
		if !ok {
			return
		}
		// Only the unconditional skill-level Resisted entries (Mdam.java:69,
		// Blow.java:74, Manadam.java:55, L2SkillChargeDmg.java:77 — no
		// `instanceof Player` gate) reach the owner via Summon.sendPacket's
		// unconditional forwarding; the
		// generic per-effect L2Skill.getEffects resist is gated
		// `effector instanceof Player` and never fires for a Summon caster.
		var resisted []handlerskill.Resisted
		for _, r := range result.Resisted {
			if r.Unconditional {
				resisted = append(resisted, r)
			}
		}
		l.sendSkillHandlerResult(owner, actorcast.EffectResult{
			AttackFailed:     result.AttackFailed,
			Lethals:          result.Lethals,
			MagicResists:     result.MagicResists,
			ManaDamageMissed: result.ManaDamageMissed,
			ManaDrains:       result.ManaDrains,
			Dodges:           result.Dodges,
			Counterattacks:   result.Counterattacks,
			Resisted:         resisted,
		})
	}
	brain.SetCastController(aiController)
	actor.Attach(summon.Runtime{AI: brain, Sink: sink})
	stopFollow := brain.StartOffensiveFollowTicker(queue, l.log)
	sink.despawn = stopFollow
	if l.ai != nil {
		runner := summonAIActor{Actor: actor, brain: brain}
		l.ai.Add(runner)
		sink.despawn = func() {
			l.ai.Remove(runner)
			stopFollow()
		}
	}
	return aiController
}

// summonLineOfSight returns the geodata query summons use for attack
// visibility, or nil when the geodata collaborator provides none.
func (l *GameClientLink) summonLineOfSight() summon.LineOfSight {
	if los, ok := l.geo.(summon.LineOfSight); ok {
		return los
	}
	return nil
}

func (l *GameClientLink) broadcastSummonStatus(actor *summon.Actor) {
	if actor == nil {
		return
	}
	owner, ok := liveSummonOwner(actor)
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

// inertSummonMoveController is the fallback when a test or incomplete
// composition root has no geodata collaborator. Production wires a real
// move.Controller above.
type inertSummonMoveController struct{}

func (inertSummonMoveController) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (inertSummonMoveController) MoveToLocation(location.Location) (bool, error) { return false, nil }
func (inertSummonMoveController) CanMoveTo(location.Location) bool               { return true }
func (inertSummonMoveController) MoveHome(location.Location) error               { return nil }
func (inertSummonMoveController) Stop() error                                    { return nil }
func (inertSummonMoveController) MaybeStartFriendlyFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}

// summonAIActor adapts a live summon to the shared periodic AI task. The
// task owns tick scheduling; the summon AI owns its intention state.
type summonAIActor struct {
	*summon.Actor
	brain *ai.Summon
}

func (summonAIActor) Tick() {}

func (a summonAIActor) TickThink() error {
	a.brain.Think()
	return nil
}
