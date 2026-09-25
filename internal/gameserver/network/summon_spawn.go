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
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// gameSummonSpawner spawns a live player's pet or servitor for its summon
// request events: it has the world, npc templates, summon-item table and
// pet persistence the domain layer intentionally doesn't depend on directly.
// It holds no state of its own: one is built per summon-request event from
// the connection's link and live player, so per-player state belongs on
// livePlayer, not here.
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

// petRestoreHoldCeiling bounds how long a summon cast is held open for its
// pets-row read, measured from the enqueue rather than from the job's start
// so it covers the wait for a shared persistence lane as well as the query.
//
// It is deliberately well under petRestoreTimeout. Hitting it then degrades
// to the ordering this hold exists to fix — cast completes, pet arrives
// afterwards — instead of coinciding with the read giving up, which would
// turn a slow lane into no pet at all. The caster is blocked from changing
// target, picking up, unequipping and casting item skills while the hold
// stands, so the ceiling is sized as the longest stall worth trading for
// packet order, not as a second read budget.
//
// The two are not on the same clock, so the gap between them is not the
// difference of the constants: this one starts at the enqueue, and
// petRestoreTimeout at the job's start, separated by however long the lane
// wait runs. A long wait then a fast query overruns the ceiling and still
// succeeds; no wait and a query past its own budget trips the ceiling and
// then fails. The degraded mode is the same either way, but neither
// constant bounds the other.
const petRestoreHoldCeiling = 2 * time.Second

// SpawnPet resolves controlItem's saved or default pet state, spawns it
// beside the owner, and registers it as the owner's active summon,
// mirroring SummonCreature.java:44-76. It stops if the owner already has a
// pet or servitor tracked, or another pets-row read in flight — a re-check of
// the gate useSummonItem already answered before the cast started.
//
// Every rejection below (that re-check, missing template, unmapped or
// non-pet summon item, missing npc template, a restore-state error, ID
// exhaustion, an unresolvable level) sends no further packet and simply
// stops. This runs
// inside the cast's already-committed Hit phase — MagicSkillUse, the
// SUMMON_A_PET system message and MagicSkillLaunched are already sent by the
// caller before SpawnPet runs — and Java's own handler is silent for the
// identical set of conditions (SummonCreature.java:36,40,44,49,54, 59 are
// all bare `return;`, with no packet beyond what the cast itself already
// sent).
//
// When the pets row has to be read, the spawn finishes on the owner's queue
// after the read (see spawnRestoredPet), so it reports nothing to its
// caller. The owner's summon slot is reserved across that read so no other
// summon or mount can take it meanwhile; see the read below.
func (s *gameSummonSpawner) SpawnPet(owner *player.Character, controlItem *item.Instance) {
	link, live := s.link, s.live
	if link == nil || live == nil || controlItem == nil {
		return
	}
	if link.hasActiveSummon(live) || link.restoringSummon(live) {
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
	//
	// Moving the read off the queue splits what the reference does in one
	// synchronous block: it resolves the row, registers the summon and
	// spawns inside useSkill, and only then schedules the cast's finalizer
	// (SummonCreature.java:28-77, PlayerCast.java:150-173). Both halves of
	// that atomicity have to be rebuilt here, because SUMMON_CREATURE
	// carries no cool time — Plan.FinalDelay stays 0, so the Finish timer
	// would otherwise be armed the instant the Hit phase returns and no
	// database round trip could beat it.
	//
	// The Finish hold stands in for the finalizer's scheduling: the cast
	// stays in flight until the spawn has run, so the client sees the pet
	// before the cast's completion, in the reference's order. This is not
	// extra waiting invented here — Pet.restore is a synchronous query
	// inside useSkill, so the reference's caster is in its cast across the
	// identical read. Unlike the reference, the queue itself keeps running
	// the owner's other work throughout.
	//
	// The summon slot is deliberately *not* claimed here. The reference's
	// setSummon runs after Pet.restore returns (SummonCreature.java:58,64),
	// as does its World.addPet, so getSummon() and getPet() are both null
	// across the read and every gate reading them answers "no summon" —
	// which is what hasActiveSummon does too. Claiming it early would make
	// Go reject or accept where the reference does the opposite, and would
	// leave hasActiveSummon reporting a summon that world.Summon cannot
	// produce.
	//
	// The hold is released once the continuation has run, and on every
	// other exit from the read: the read's error branch, a continuation the
	// queue refused, and a lane that refused the job outright, which
	// Enqueue reports without running it once the worker is closed.
	//
	// It also has a ceiling. petRestoreTimeout bounds the query, but the
	// job first waits its turn on one of persist.Lanes lanes shared by
	// every owner (persist.LaneIndex), and nothing bounds that wait —
	// a burst of logout saves landing on the same lane would otherwise keep
	// the caster in a cast long after the client's own cast bar ended,
	// which the reference never does: its read has nothing queued ahead of
	// it. Past the ceiling the ordering guarantee yields to keeping the
	// player responsive, and the pet spawns after the cast completed, as it
	// did before the hold existed. Release is idempotent, so the timer and
	// the continuation race harmlessly.
	// petRestoreInFlight outlives the ceiling on purpose. The ceiling ends
	// the cast to keep the player responsive, but the pet is still on its
	// way, so the gates the reference closes with isCastingNow() have to
	// stay closed until it lands — otherwise the ceiling hands back exactly
	// the window the hold was added to remove, and a wyvern collar used in
	// it leaves the owner mounted with a pet arriving beside them.
	live.petRestoreInFlight.Store(true)
	releaseFinish := link.castController(live).HoldFinish()
	live.Queue().After(petRestoreHoldCeiling, releaseFinish)
	if !link.persist.Enqueue(controlItem.ObjectID, func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), petRestoreTimeout)
		defer cancel()
		state, hasSaved, err := link.petStore.Get(restoreCtx, controlItem.ObjectID)
		if err != nil {
			link.log.Error().Err(err).Int32("item_obj_id", controlItem.ObjectID).Msg("summon: pet restore failed")
			if !postLive(live, func() { s.endRestore(releaseFinish) }) {
				s.endRestore(releaseFinish)
			}
			return
		}
		posted := postLive(live, func() {
			defer s.endRestore(releaseFinish)
			s.spawnRestoredPet(controlItem, summonItem, npcTmpl, state, hasSaved)
		})
		if !posted {
			s.endRestore(releaseFinish)
		}
	}) {
		s.endRestore(releaseFinish)
	}
}

// endRestore ends the pets-row read: it reopens the owner's summon slot and
// releases the cast's deferred Finish if the ceiling has not already. It runs
// on every exit from the read, including the ones that build no pet, so
// neither a failed restore nor a session that went away mid-read can leave
// the owner unable to summon or mount.
func (s *gameSummonSpawner) endRestore(releaseFinish func()) {
	if s.live != nil {
		s.live.petRestoreInFlight.Store(false)
	}
	releaseFinish()
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
	// This re-check is against the world, not against restoringSummon: the
	// restore still in flight here is this spawn's own, so consulting it
	// would reject every restored pet. It catches a summon that reached the
	// world while the read was outstanding. Every summon entry already
	// refuses to start while restoringSummon holds, so this is an unreachable
	// backstop. Silent, like SpawnPet's own re-check.
	if _, ok := link.world.Summon(live.ObjectID()); ok {
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
		Inventory: itemcontainer.NewPetInventoryWithDelivery(objID, live.Inventory().Templates(), &petInventoryDelivery{
			updates: link.inventoryUpdates,
			live:    live,
			state:   link.world,
			log:     link.log,
		}, link.itemPersister(objID)),
		Fed:           fed,
		MaxMeal:       levelStats.MaxMeal,
		MealInNormal:  levelStats.MealInNormal,
		MealInBattle:  levelStats.MealInBattle,
		Food1:         int32(npcTmpl.Pet.Food1),
		Food2:         int32(npcTmpl.Pet.Food2),
		FoodRestore1:  foodRestore1,
		FoodRestore2:  foodRestore2,
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
	// A pet restore still in flight owns the slot even though world.Summon
	// cannot show it yet: past the hold ceiling this cast could otherwise
	// take the slot and the inbound pet would be dropped at
	// spawnRestoredPet's re-check. The restoringSummon half is an unreachable
	// backstop: every summon entry, handleMagicSkillUse included, refuses to
	// start while it holds, and a cast in flight blocks the collar that would
	// begin a read.
	if link.hasActiveSummon(live) || link.restoringSummon(live) {
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
		Effects:         link.effects,
		ObjectID:        objID,
		Owner:           live,
		NPCID:           def.NpcID,
		CollisionRadius: npcTmpl.CollisionRadius,
		CollisionHeight: npcTmpl.CollisionHeight,
		Name:            npcTmpl.Name,
		Level:           npcTmpl.Level,
		MaxBuffsAmount:  link.playerConfig.MaxBuffsAmount,
		OwnerInventory:  live.Inventory(),
		ExpPenalty:      def.ExpPenalty,
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
	owner, _ := liveSummonOwner(actor)
	queue := owner.Queue()
	actor.SetQueue(queue)
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
	attackController.SetQueue(queue)
	brain := ai.NewSummon(actor, moveController, attackController)
	sink.brain = brain
	actor.SetRaidCursesDisabled(l.disableRaidCurse)
	// SetLogger records broadcast errors from TryToAttack/TryToFollow/TryToIdle/Think
	// that have no caller left to return them to; left unset, they're silently
	// discarded through the zero-value zerolog.Logger.
	brain.SetLogger(l.log)
	castController := actorcast.NewController(actorcast.SummonActor{Summon: actor}, nil)
	castController.SetQueue(queue)
	aiController := &actorcast.AIController{
		Controller:  castController,
		Definitions: l.skills,
		Effects:     actorcast.EffectHandlers{Targets: l.targets, Skills: l.skillHandlers},
		Caster:      actor,
		// Every summon removal aborts first, so a hit reaching a summon
		// that has left the world lost a race with that abort.
		HitNeedsPresence: true,
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
		var messages []any
		for _, message := range result.Messages {
			switch m := message.(type) {
			case handlerskill.Resisted:
				if m.Unconditional {
					messages = append(messages, m)
				}
			case handlerskill.OpponentMPReducedMessage:
				// This caster-only message does not reach a summon's owner.
			default:
				messages = append(messages, message)
			}
		}
		l.sendSkillHandlerResult(owner, actorcast.EffectResult{Messages: messages})
	}
	brain.SetCastController(aiController)
	actor.Attach(summon.Runtime{AI: brain, Sink: sink})
	// Each cleanup is registered directly after the thing it releases
	// starts, never before and never batched into a single field written
	// after both. Once the summon is attached, another actor's ERASE or
	// signet can despawn it from that actor's own queue before this
	// function returns. Registering afterwards means the losing side still
	// releases its resource through onDespawn's already-despawned path, so
	// a summon that leaves the world inside this window cannot leave the AI
	// task ticking a dead actor.
	sink.onDespawn(brain.StartOffensiveFollowTicker(queue))
	if l.ai != nil {
		runner := summonAIActor{Actor: actor, brain: brain}
		l.ai.Add(runner)
		sink.onDespawn(func() { l.ai.Remove(runner) })
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
func (inertSummonMoveController) Stop()                                          {}
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
