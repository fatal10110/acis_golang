package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishing"
	itemhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/item"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// Bait distance: a line lands baitDistance plus up to baitDistanceSpread-1
// units ahead of the fisher, and floats baitFloat above the water level.
const (
	baitDistance       = 250
	baitDistanceSpread = 50
	baitFloat          = 10
)

// Sounds a cast line and a bite play to the fisher.
const (
	soundCastLine = "SF_P_01"
	soundBite     = "SF_S_01"
	// soundTypeMusic is PlaySound's music sound type.
	soundTypeMusic = 1
)

// fishingDeps is what a cast line draws from: the fish table and the dice.
type fishingDeps struct {
	table *fish.Table
	roll  func(n int) int
}

// dice returns a uniform int in [0, n).
func (d fishingDeps) dice(n int) int {
	if d.roll != nil {
		return d.roll(n)
	}
	return rnd.Get(n)
}

// fishingRun is one player's fishing line and the timers driving it, all
// owned by the player's queue.
type fishingRun struct {
	stance    fishing.Stance
	firstLook *sim.Timer
	looks     *sim.Ticker
	fight     *sim.Ticker
}

func (r *fishingRun) stopTimers() {
	if r.firstLook != nil {
		r.firstLook.Stop()
		r.firstLook = nil
	}
	r.stopLooks()
	if r.fight != nil {
		r.fight.Stop()
		r.fight = nil
	}
}

func (r *fishingRun) stopLooks() {
	if r.looks != nil {
		r.looks.Stop()
		r.looks = nil
	}
}

// castFishing answers live's Fishing skill landing: a line already cast is
// taken out of the water; otherwise a line baited with the lure worn is cast
// into the fishing water ahead, using up one lure.
func (l *GameClientLink) castFishing(live *livePlayer) {
	if live.fishing.stance.Fishing() {
		l.endFishing(live, false)
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFishingAttemptCancelled))
		return
	}
	inv := live.Inventory()
	var lure *item.Instance
	if inv != nil {
		lure = inv.ItemAt(itemcontainer.LHand)
	}
	// ponytail: the on-a-boat refusal (CANNOT_FISH_ON_BOAT) waits for boat passengers, #229.
	switch fishing.CastRefusal(live.Character, lure != nil) {
	case fishing.RefuseNoRod:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFishingPoleNotEquipped))
		return
	case fishing.RefuseOperating:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotFishWhileUsingRecipeBook))
		return
	case fishing.RefuseInWater:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotFishUnderWater))
		return
	case fishing.RefuseNoLure:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageBaitOnHookBeforeFishing))
		return
	}
	bait, ok := l.fishingBaitSpot(live)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCannotFishHere))
		return
	}
	lureID := lure.TemplateID
	res, ok := l.inventory.DestroyItem(inv, lure.ObjectID, 1)
	if !ok {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughBait))
		return
	}
	// The last lure leaves the hand with the stack, its InventoryUpdate
	// showing it; nothing resends the appearance.
	l.applyEquipStatChanges(live, inv, res)
	l.startFishing(live, bait, lureID)
}

// fishingBaitSpot is where live's line lands: a little ahead of it, on the
// surface of fishing water it can see, with the ground below that surface.
// ok is false when the spot is no such water.
func (l *GameClientLink) fishingBaitSpot(live *livePlayer) (location.Location, bool) {
	at := location.OrientedLocation{Location: live.CurrentLocation(), Heading: live.CurrentHeading()}
	bait := at.Ahead(l.fishing.dice(baitDistanceSpread) + baitDistance)
	if l.zones == nil || l.geo == nil {
		return bait, false
	}
	water, ok := zone.FindAtXY[*zone.Fishing](l.zones, bait.X, bait.Y)
	if !ok {
		return bait, false
	}
	bait.Z = water.WaterLevel()
	if !live.CanSeePoint(bait.X, bait.Y, bait.Z) || int(l.geo.Height(bait.X, bait.Y, bait.Z)) >= bait.Z {
		return bait, false
	}
	bait.Z += baitFloat
	return bait, true
}

// startFishing casts live's line baited with lureID at bait: live stops
// and is held in place, the fish the line is cast for is drawn, and the wait
// for a bite begins. A lure whose grade sets no look period never gets a
// bite: its line stays cast with nothing looking for one.
func (l *GameClientLink) startFishing(live *livePlayer, bait location.Location, lureID int32) {
	if live.Dead() {
		return
	}
	if live.move != nil {
		live.move.Stop()
	}
	live.SetImmobilized(true)
	run := &live.fishing
	run.stance.Cast(lureID)
	live.SetFishingBait(bait)
	f, ok := fishing.Choose(l.fishing.table, lureID, l.fishingLevel(live), l.fishing.dice)
	if !ok {
		l.endFishing(live, false)
		return
	}
	run.stance.Hook(f)

	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCastLineAndStartFishing))
	fishType, night := int32(run.stance.FishType(l.night())), run.stance.NightLure()
	// ponytail: the ranking button shows under AllowFishChampionship once the championship lands, #254.
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameExFishingStart(live.ObjectID(), fishType, bait, night, false)
	})
	live.SendFrame(serverpackets.FramePlaySoundAt(serverpackets.Sound{Type: soundTypeMusic, File: soundCastLine}))

	period := fishing.CheckDelay(lureID, f.GutsCheckTime)
	if period <= 0 {
		return
	}
	q := live.Queue()
	run.stance.Wait(q.Now())
	live.SetFishing(true)
	run.firstLook = q.After(fishing.FirstLookDelay, func() {
		run.firstLook = nil
		if l.lookForFish(live) {
			run.looks = q.Every(period, func() { l.lookForFish(live) })
		}
	})
}

// fishingLevel is the level of the fish live draws: its Fisherman's Potion
// power while that potion holds, its Fishing Expertise level otherwise.
func (l *GameClientLink) fishingLevel(live *livePlayer) int {
	for _, e := range live.EffectList().All() {
		if e.Skill.ID != fishing.PotionSkillID {
			continue
		}
		if l.skills == nil {
			return 0
		}
		def, ok := l.skills.Definition(modelskill.Ref{ID: e.Skill.ID, Level: e.Skill.Level})
		if !ok {
			return 0
		}
		return int(def.Power)
	}
	return live.SkillLevel(fishing.ExpertiseSkillID)
}

func (l *GameClientLink) night() bool {
	return l.gameClock != nil && l.gameClock.IsNight()
}

// lookForFish looks once for a bite on live's line and reports whether the
// wait goes on. A bite opens the fight; a wait run out ends the fishing.
func (l *GameClientLink) lookForFish(live *livePlayer) bool {
	run := &live.fishing
	kind, combat := run.stance.Look(live.Queue().Now(), l.night(), l.fishing.dice)
	switch kind {
	case fishing.LookTimeout:
		l.endFishing(live, false)
		return false
	case fishing.LookBite:
		run.stopLooks()
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameExFishingStartCombat(live.ObjectID(), int32(combat.Time), int32(combat.HP), uint8(combat.Mode), uint8(combat.LureType), uint8(combat.Deceptive))
		})
		live.SendFrame(serverpackets.FramePlaySoundAt(serverpackets.Sound{Type: soundTypeMusic, File: soundBite}))
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageGotABite))
		run.fight = live.Queue().Every(fishing.CombatTick, func() { l.fightFish(live) })
		return false
	}
	return true
}

// fightFish runs one second of live's fight with its hooked fish.
func (l *GameClientLink) fightFish(live *livePlayer) {
	kind, gauge, broadcast := live.fishing.stance.Tick(l.fishing.dice)
	switch kind {
	case fishing.TickStolen:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageBaitStolenByFish))
		l.endFishing(live, false)
	case fishing.TickSpat:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageFishSpitTheHook))
		l.endFishing(live, false)
	default:
		if broadcast {
			l.broadcastFishingGauge(live, gauge)
		} else {
			live.SendFrame(fishingGaugeFrame(live.ObjectID(), gauge))
		}
	}
}

func fishingGaugeFrame(objectID int32, g fishing.HPRegen) wire.Frame {
	return serverpackets.FrameExFishingHpRegen(objectID, serverpackets.FishingHPRegen{
		Time: int32(g.Time), HP: int32(g.HP), Mode: uint8(g.Mode), GoodUse: uint8(g.GoodUse),
		Anim: uint8(g.Anim), Penalty: int32(g.Penalty), Deceptive: uint8(g.Deceptive),
	})
}

func (l *GameClientLink) broadcastFishingGauge(live *livePlayer, g fishing.HPRegen) {
	l.broadcastLiveFrame(live, func() wire.Frame { return fishingGaugeFrame(live.ObjectID(), g) })
}

// fishingActionMessages maps an action outcome to its message, for pumping
// and for reeling.
var fishingActionMessages = map[fishing.ActionMessage]int{
	fishing.ActionResisted:   serverpackets.SystemMessageFishResistedAttemptToBringItIn,
	fishing.ActionPumped:     serverpackets.SystemMessagePumpingSuccessfulS1Damage,
	fishing.ActionPumpFailed: serverpackets.SystemMessageFishResistedPumpingS1HP,
	fishing.ActionReeled:     serverpackets.SystemMessageReelingSuccessfulS1Damage,
	fishing.ActionReelFailed: serverpackets.SystemMessageFishResistedReelingS1HP,
}

// useFishingAction answers live's pumping or reeling skill landing: outside
// a fight it is refused; with a fishing rod in hand it acts on the hooked
// fish, doubled and used up by a charged fishing shot.
func (l *GameClientLink) useFishingAction(live *livePlayer, m skillhandler.FishingAction) {
	run := &live.fishing
	if !run.stance.Fighting() {
		msg := serverpackets.SystemMessageCanUsePumpingOnlyWhileFishing
		if m.Reeling {
			msg = serverpackets.SystemMessageCanUseReelingOnlyWhileFishing
		}
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
		live.SendFrame(serverpackets.FrameActionFailed())
		return
	}
	// The cast already ended; a rod taken off mid-fight leaves the fish
	// untouched, with nothing to answer.
	rod, ok := live.FishingRod()
	if !ok {
		return
	}
	shot := live.ChargedShot(item.ShotFishSoul)
	damage, penalty := fishing.Damage(m.Power, rod, shot, m.Level, live.SkillLevel(fishing.ExpertiseSkillID))
	if penalty > 0 {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageReelingPumping3LevelsHigher))
	}
	if shot {
		live.SetChargedShot(item.ShotFishSoul, false)
	}
	var a fishing.Action
	penaltyMsg := serverpackets.SystemMessagePumpingSuccessfulPenaltyS1
	if m.Reeling {
		a = run.stance.Reel(damage, penalty, l.fishing.dice)
		penaltyMsg = serverpackets.SystemMessageReelingSuccessfulPenaltyS1
	} else {
		a = run.stance.Pump(damage, penalty, l.fishing.dice)
	}
	if a.Message == fishing.ActionResisted {
		live.SendFrame(serverpackets.FrameSystemMessage(fishingActionMessages[a.Message]))
	} else {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(fishingActionMessages[a.Message], int32(a.Damage)))
	}
	if a.PenaltyNotice {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(penaltyMsg, int32(penalty)))
	}
	l.broadcastFishingGauge(live, a.Gauge)
	if a.End != fishing.EndNone {
		l.endFishing(live, a.End == fishing.EndCaught)
	}
}

// endFishing takes live's line out of the water. win pays the catch: the
// fish, or on a few catches a monster at live's feet instead. A line cast
// for no fish at all lost its bait.
func (l *GameClientLink) endFishing(live *livePlayer, win bool) {
	run := &live.fishing
	if win {
		if fishing.CaughtMonster(l.fishing.dice(100)) {
			if l.spawnFishingMonster(live) {
				live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCaughtSomethingSmelly))
			}
		} else {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouCaughtSomething))
			live.AddCreatedItem(run.stance.Fish().ID, 1, l.nextObjectID)
			// ponytail: the fishing championship's catch record joins here, #254.
		}
	}
	if !run.stance.HasFish() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageBaitLostFishGotAway))
	}
	run.stance.Reset()
	run.stopTimers()
	live.SetFishing(false)
	live.SetFishingBait(location.Location{})
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameExFishingEnd(live.ObjectID(), win)
	})
	live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageReelLineAndStopFishing))
	live.SetImmobilized(false)
}

// spawnFishingMonster places the monster live's catch turned into at its
// feet and reports whether there was one to place.
func (l *GameClientLink) spawnFishingMonster(live *livePlayer) bool {
	if l.npcs == nil {
		return false
	}
	tmpl, ok := l.npcs.Get(fishing.PenaltyMonsterID(live.Level()))
	if !ok {
		return false
	}
	// ponytail: the monster's own AI (attack its fisher, leave when idle) waits for AI scripts, #3306.
	if npcs := l.npcSpawns.Load(); npcs != nil {
		x, y, z := live.Position()
		if err := npcs.SpawnFixed(tmpl, x, y, z, live.CurrentHeading()); err != nil {
			l.log.Warn().Err(err).Int("npc_id", tmpl.ID).Msg("fishing: caught monster placed nothing")
		}
	}
	return true
}

// useFishShotItem charges live's fishing rod with a fishing shot used from
// the item window, and reports whether inst is one.
func (l *GameClientLink) useFishShotItem(live *livePlayer, inv *itemcontainer.Inventory, inst *item.Instance) bool {
	if live == nil || inv == nil || inst == nil {
		return false
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl.EtcItem == nil || tmpl.EtcItem.Handler != itemhandler.FishShotsHandler {
		return false
	}
	outcome, skillID := itemhandler.UseFishShot(itemhandler.FishShotUseRequest{
		Caster:    live.Character,
		Inventory: inv,
		Item:      inst,
		Template:  tmpl,
		Destroyer: l.inventory,
	})
	switch outcome {
	case itemhandler.FishShotApplied:
		if skillID != 0 {
			self := skillCastObject(live)
			l.broadcastLiveFrame(live, func() wire.Frame {
				return serverpackets.FrameMagicSkillUse(self, self, skillID, 1, 0, 0, false)
			})
		}
	case itemhandler.FishShotAlreadyCharged:
		// A pure no-op, as for the weapon shots: fully silent.
	case itemhandler.FishShotNoRod:
		// The specified handler is silent; the click is released.
		live.SendFrame(serverpackets.FrameActionFailed())
	case itemhandler.FishShotGradeMismatch:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWrongFishingShotGrade))
		live.SendFrame(serverpackets.FrameActionFailed())
	case itemhandler.FishShotNotEnoughItems:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotEnoughSoulshots))
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	return true
}
