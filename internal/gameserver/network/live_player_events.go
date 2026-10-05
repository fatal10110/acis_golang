package network

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// SendFrame sends frame to this player's client. Once the session has
// detached it releases frame and reports false. While frames wait for the
// player's PvP flag changes, frame waits behind them (sendBehindPvPChanges).
func (p *livePlayer) SendFrame(frame wire.Frame) bool {
	if p.session == nil || p.SessionDetached() {
		frame.Release()
		return false
	}
	if p.pvpChanges.hold(frame, false) {
		return true
	}
	return p.session(frame)
}

// sendFrameNow sends frame through the client path visibility names,
// without waiting behind held frames.
func (p *livePlayer) sendFrameNow(frame wire.Frame, visibility bool) bool {
	send := p.session
	if visibility {
		send = p.visibilitySend
	}
	if send == nil || (p.Character != nil && p.SessionDetached()) {
		frame.Release()
		return false
	}
	return send(frame)
}

// sendHeldFrame sends a frame that waited behind the player's PvP flag
// changes.
func (p *livePlayer) sendHeldFrame(f heldFrame) {
	p.sendFrameNow(f.frame, f.visibility)
}

// BroadcastFrame delivers one copy of another actor's broadcast to this
// player's client, with SendFrame's detach behavior.
func (p *livePlayer) BroadcastFrame(frame wire.Frame) bool {
	return p.SendFrame(frame)
}

// sessionOnly reports whether ev is delivered only while p's session is
// attached. Every other event keeps reaching observers after detach.
func sessionOnly(ev event.Event) bool {
	switch ev.(type) {
	case event.Attack, event.BowDrawn, event.Died, event.DeathSettled, event.HerbConsumed, event.ItemObtained,
		event.RegenMax, event.EffectRemovedLackHP, event.EffectRemovedLackMP,
		event.RelaxHPFull, event.Restored, event.EffectEnded, event.EffectFelt, event.SpoilResult,
		event.ServitorVanished, event.ShieldBlocked, event.AttackFailed, event.HitDealt,
		event.SkillResisted, event.MagicResisted, event.DamageReceived, event.ServitorDamageShared, event.SkillDamageDealt, event.UserInfoChanged,
		event.PvPFlagged, event.RelationChanged, event.LevelChanged,
		event.WeightPenaltyChanged, event.VitalsChanged, event.Evaded, event.MountFeedGauge:
		return true
	}
	return false
}

// Emit maps one of p's character events to its packets and follow-up
// actions. Each arm keeps the send order its packets reach clients in.
func (p *livePlayer) Emit(ev event.Event) {
	if _, died := ev.(event.Died); died {
		// Death goes idle whether or not a client watches: no walk, pickup,
		// interact, follow, toggle or queued request outlives it to be
		// re-run after a revive.
		p.clearParkedApproaches()
		// Every cubic ends with its owner's life. The removal itself
		// sends no character-info update.
		p.removeAllCubics()
	}
	if p.SessionDetached() && sessionOnly(ev) {
		return
	}
	l, live := p.link, p
	switch e := ev.(type) {
	case event.Attack:
		l.broadcastAttack(live, e)
	case event.BowDrawn:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageGettingReadyToShootAnArrow))
		live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeRed, e.GaugeMs, e.GaugeMs))
	case event.MagicSkillUse:
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameMagicSkillUse(
				serverpackets.SkillCastObject{ObjectID: e.CasterID, Location: e.CasterAt},
				serverpackets.SkillCastObject{ObjectID: e.TargetID, Location: e.TargetAt},
				e.SkillID, e.Level, e.HitTime, e.ReuseDelay, false,
			)
		})
	case event.SkillLaunched:
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameMagicSkillLaunched(live.ObjectID(), e.SkillID, e.Level, e.TargetIDs)
		})
	case event.HitDamageApplied:
		// The hit's own task runs on live's queue: a PK kill it just made
		// takes its items off and resets its flag before anything else the
		// hit sends.
		l.settlePvPChanges(live)
	case event.HitLanded:
		l.chance.AttackHit(live.Character, e)
	case event.ShotsRechargeRequested:
		l.rechargeShots(live, live.Inventory(), e.Physical, e.Magic)
	case event.Move:
		l.broadcastLiveMoveEvent(live, e)
	case event.Flight:
		at := live.CurrentLocation()
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameFlyToLocation(live.ObjectID(), e.Dest, at, e.Flight)
		})
	case event.PositionCorrected:
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameValidateLocation(live.ObjectID(), live.CurrentLocation(), live.CurrentHeading())
		})
	case event.Stopped:
		// Stopping a move revalidates the zones at once, before observers
		// see the stop.
		l.revalidateZones(live, live.CurrentLocation(), revalidateForce)
		x, y, z := live.Position()
		l.broadcastLiveStopMove(live, location.Location{X: x, Y: y, Z: z}, live.CurrentHeading())
	case event.AutoAttackStopped:
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameAutoAttackStop(live.ObjectID())
		})
	case event.StanceChanged:
		waitType := serverpackets.WaitSitting
		switch e.Stance {
		case event.StanceStanding:
			waitType = serverpackets.WaitStanding
		case event.StanceFakeDeathStart:
			waitType = serverpackets.WaitFakeDeathStart
		case event.StanceFakeDeathStop:
			waitType = serverpackets.WaitFakeDeathStop
		}
		x, y, z := live.Position()
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameChangeWaitType(live.ObjectID(), waitType, location.Location{X: x, Y: y, Z: z})
		})
	case event.FakeDeathRevived, event.Revived:
		l.broadcastLiveRevive(live)
	case event.PostureSettled:
		l.settleLivePosture(live, e.StoodUp)
	case event.ReviveRequested:
		live.SendFrame(serverpackets.FrameConfirmDlgResurrectionRequest(e.ReviverName))
	case event.ReviveRefused:
		live.SendFrame(serverpackets.FrameSystemMessage(reviveRefusalMessage(e.Reason)))
	case event.Died:
		l.broadcastLiveDie(live)
	case event.FusionCastersStopRequested:
		l.abortFusionTargeting(live)
	case event.DeathItemDrop:
		l.dropItemsOnDeath(live, e)
	case event.ClanKill:
		l.creditClanKill(live, e)
	case event.DeathSettled:
		if l.water != nil {
			l.water.Remove(live)
		}
	case event.VitalsChanged:
		// The restored ticks the replay runs change the vitals before the
		// player is in the world; the EnterWorld UserInfo carries them.
		if !live.replayingEffects.Load() {
			sendLiveStatus(live)
			l.sendPartyVitals(live)
		}
	case event.EffectIconsChanged:
		l.updateEffectIcons(live)
	case event.AbnormalEffectChanged:
		if !live.replayingEffects.Load() {
			l.broadcastCharacterInfo(live)
		}
	case event.ExpSPGained:
		live.SendFrame(expSpGainMessage(e.Exp, e.SP))
	case event.ExpSPLost:
		sendExpSpLossFrames(live, e)
	case event.SPChanged:
		live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusSP, Value: e.SP},
		}))
	case event.KarmaChanged:
		sendKarmaChangeFrames(live, e.Karma)
	case event.RelationChanged:
		l.broadcastRelations(live)
	case event.PvPFlagged:
		l.applyPvPFlag(live, e)
	case event.PKKarmaGained:
		l.applyPKKarmaSideEffects(live)
	case event.DuelDefeated:
		if l.duels != nil {
			l.duels.Defeat(live)
		}
	case event.LeveledUp:
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameSocialAction(live.ObjectID(), socialActionLevelUp)
		})
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouIncreasedYourLevel))
	case event.MountFeedGauge:
		live.SendFrame(serverpackets.FrameSetupGauge(serverpackets.GaugeGreen, e.Current, e.Max))
	case event.MountFoodDue:
		l.feedMountFood(live, e.ObjectID)
	case event.Dismounted:
		l.broadcastDismount(live)
		if e.PetControlItemID != 0 {
			l.storePetFood(e.PetControlItemID, e.Fed)
		}
	case event.MountOutOfFeed:
		l.throwStarvedRider(live, e.WasFlying)
	case event.UserInfoChanged:
		live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	case event.RunSpeedChanged, event.EffectsStripped:
		l.broadcastFullStatus(live)
	case event.StatsModified:
		l.sendModifiedStats(live, e.Attrs)
	case event.ChargesChanged, event.EtcStatusChanged:
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
	case event.EtcStatusBroadcast:
		// The saved effects EnterWorld replays have no observers yet, and
		// the EnterWorld EtcStatusUpdate that follows carries their flags.
		if !live.replayingEffects.Load() {
			l.broadcastLiveFrame(live, func() wire.Frame { return serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)) })
		}
	case event.ChargeMessage:
		if e.Maxed {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageForceMaxLevelReached))
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageForceIncreasedToS1, int32(e.Charges)))
	case event.GradePenaltyChanged:
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
		l.refreshLiveItemStats(live)
	case event.WeightPenaltyChanged:
		// The login decides the band inside the replay window, where the
		// burst's own frames carry it.
		if !live.replayingEffects.Load() {
			l.sendLiveWeightPenalty(live)
		}
	case event.DeathPenaltyChanged:
		l.applyLiveDeathPenalty(live, e)
	case event.LevelChanged:
		l.refreshLiveLevelSkills(live)
		if l.parties != nil {
			l.parties.RecalculateLevel(live.ObjectID())
		}
		l.refreshClanMemberLevel(live)
	case event.ShortBuff:
		live.SendFrame(serverpackets.FrameShortBuffStatusUpdate(e.SkillID, e.Level, e.DurationSeconds))
	case event.RegenMax:
		live.SendFrame(serverpackets.FrameExRegenMax(e.Count, e.Period, e.HPRegen))
	case event.EffectRemovedLackHP:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillRemovedDueLackHP))
	case event.EffectRemovedLackMP:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillRemovedDueLackMP))
	case event.RelaxHPFull:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillDeactivatedHPFull))
	case event.Restored:
		sendRestoredFrame(live, e)
	case event.EffectEnded:
		messageID := serverpackets.SystemMessageS1HasWornOff
		switch e.Reason {
		case event.EffectDisappeared:
			messageID = serverpackets.SystemMessageEffectS1Disappeared
		case event.EffectAborted:
			messageID = serverpackets.SystemMessageS1HasBeenAborted
		}
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(messageID, int32(e.SkillID), int32(e.Level)))
	case event.EffectFelt:
		live.SendFrame(serverpackets.FrameSystemMessageSkillName(serverpackets.SystemMessageYouFeelS1Effect, int32(e.SkillID), int32(e.Level)))
	case event.SpoilResult:
		if e.Already {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAlreadySpoiled))
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSpoilSuccess))
	case event.OverHit:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOverHit))
	case event.ClanGateOpened:
		l.announceClanGate(live)
	case event.QuestListChanged:
		live.SendFrame(questListEntriesFrame(e.Entries))
	case event.QuestMarked:
		live.SendFrame(serverpackets.FrameExShowQuestMark(e.QuestID))
	case event.ServitorVanished:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageServitorHasVanished))
	case event.ShieldBlocked:
		if e.Perfect {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExcellentShieldDefenseSuccess))
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageShieldDefenceSuccessful))
	case event.AttackFailed:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAttackFailed))
	case event.HitDealt:
		if e.Miss {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageMissedTarget))
			return
		}
		sendDamageMessage(live, hitDamage(e, skillhandler.DamageByPlayer))
	case event.SkillResisted:
		live.SendFrame(serverpackets.FrameSystemMessageStringSkillName(serverpackets.SystemMessageS1ResistedYourS2, e.TargetName, int32(e.SkillID), int32(e.Level)))
	case event.MagicResisted:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageResistedS1Magic, e.AttackerName))
	case event.DamageReceived:
		live.SendFrame(serverpackets.FrameSystemMessageStringNumber(serverpackets.SystemMessageS1GaveYouS2Dmg, e.AttackerName, int32(e.Amount)))
	case event.ServitorDamageShared:
		live.SendFrame(serverpackets.FrameSystemMessageTwoNumbers(serverpackets.SystemMessageGivenS1DamageToTargetS2ToServitor, int32(e.TargetDamage), int32(e.ServitorDamage)))
	case event.SkillDamageDealt:
		sendDamageMessage(live, skillhandler.Damage{
			Source: skillhandler.DamageByPlayer, Amount: int32(e.Amount),
			MagicCrit: e.MagicCrit, Blocked: e.Blocked, Petrified: e.Petrified,
		})
	case event.AttackTargetRefused:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageTargetIncorrect))
	case event.AttackWeaponRefused:
		live.SendFrame(serverpackets.FrameSystemMessage(attackWeaponRefusalMessage(e.Reason)))
	case event.AttackRequested:
		if e.Target != nil {
			// An attack an effect forces on the player holds no shift.
			l.attackLiveTarget(live, e.Target, false)
		}
	case event.FleeRequested:
		l.fleeLivePlayer(live, e)
	case event.Retargeted:
		if e.Target == nil {
			l.clearLiveTarget(live)
			return
		}
		l.selectLiveTarget(live, e.Target)
	case event.HerbConsumed:
		l.consumeHerb(live, e.ItemID)
	case event.ItemObtained:
		live.SendFrame(itemObtainedFrame(e))
		l.obtainCursedWeapon(live, e.ItemID)
	case event.CursedWeaponKill:
		l.feedCursedWeapon(live)
	case event.CursedWeaponLost:
		l.loseCursedWeapon(live)
	case event.SummonConfirmRequested:
		live.SendFrame(serverpackets.FrameConfirmDlgSummonFriendRequest(e.CasterName, e.CasterID, int32(e.X), int32(e.Y), int32(e.Z), e.Timeout))
	case event.TeleportRequested:
		l.teleportLivePlayer(live, location.Location{X: e.X, Y: e.Y, Z: e.Z}, e.Radius)
	case event.RecallRequested:
		l.recallLivePlayer(live, e.Destination)
	case event.Relocated:
		reason := revalidateStep
		if e.Placed {
			reason = revalidatePlaced
		}
		l.revalidateZones(live, e.Previous, reason)
	case event.AttackStanceRequested:
		l.startLiveAutoAttack(live)
	case event.Attacked:
		l.standAttackedLivePlayer(live)
		l.startLiveAutoAttack(live)
	case event.Evaded:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageAvoidedS1Attack, e.Attacker.CharacterName()))
	case event.AttackFinished:
		if l.finishDeferredAction(live) {
			return
		}
		// A queued pickup, cast, follow or summon interact took the
		// intention the swing had when it was queued; whether it started
		// now, was refused, or is held for PostureSettled, the attack does
		// not swing again. A queued equip toggle resumes the attack itself
		// once it has run.
		if l.finishQueuedBehindAttack(live) {
			return
		}
		live.finishAttack()
	case event.BowShotFinished:
		if l.finishDeferredAction(live) {
			return
		}
		// A shot's end runs whatever was queued behind it, while the bow
		// still reloads: a pickup, cast, follow, summon interact or equip
		// toggle starts now, and an attack is re-thought, which the running
		// reuse answers with ActionFailed. The toggle re-thinks the attack
		// itself.
		if l.finishQueuedBehindAttack(live) {
			return
		}
		if live.combat != nil && live.combat.ThinkQueued() {
			live.SendFrame(serverpackets.FrameActionFailed())
		}
	case event.Arrived:
		l.arriveAtBoatEntrance(live)
		// CreatureMove tracks position for its own timing only; push the
		// arrived position into the world-grid presence range checks
		// actually read before re-thinking the attack intention, or it
		// re-evaluates against a stale position forever.
		pos := live.move.Position()
		l.updateLivePlayerPosition(live, pos, live.CurrentHeading())
		l.finishLiveGroundPickup(live)
		l.finishInteract(live)
		l.finishDeferredMagicSkill(live)
		l.finishDeferredItemAICast(live)
		live.thinkAttack()
		l.arriveHeldIntention(live)
	case event.MoveBlocked:
		live.SetBoatMovement(false)
		if !l.onPlayerArrivedBlocked(live) {
			live.move.BroadcastBlockedCorrection()
		}
	case event.ActionsStopRequested:
		l.stopLiveActions(live, e)
	case event.IdleRequested:
		live.tryToIdle(e.AIDenied)
	case event.ThinkRequested:
		l.thinkCurrentIntention(live)
	case event.CastAborted:
		l.broadcastCastAborted(live)
	case event.CastStopAck:
		live.endCastStop(e)
	case event.SkillMasteryProc:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillReadyToUseAgain))
	case event.CastFinished:
		l.finishLiveCast(live, e.Skill, e.Target, e.Interrupted)
	case event.PetSummonRequested:
		if controlItem, ok := e.ControlItem.(*item.Instance); ok {
			(&gameSummonSpawner{link: l, live: live}).SpawnPet(live.Character, controlItem)
		}
	case event.ServitorSummonRequested:
		(&gameSummonSpawner{link: l, live: live}).SpawnServitor(live.Character, e.Skill)
	}
}

// hitDamage is an auto-attack hit's damage feedback from source.
func hitDamage(e event.HitDealt, source skillhandler.DamageSource) skillhandler.Damage {
	return skillhandler.Damage{
		Source:       source,
		Amount:       int32(e.Damage),
		PhysicalCrit: e.Crit,
		Blocked:      e.Blocked,
		Petrified:    e.Petrified,
	}
}

func sendRestoredFrame(live *livePlayer, e event.Restored) {
	var byOther, self int
	switch e.Resource {
	case event.ResourceHP:
		byOther, self = serverpackets.SystemMessageS2HPRestoredByS1, serverpackets.SystemMessageS1HPRestored
	case event.ResourceMP:
		byOther, self = serverpackets.SystemMessageS2MPRestoredByS1, serverpackets.SystemMessageS1MPRestored
	default:
		byOther, self = serverpackets.SystemMessageS2CPWillBeRestoredByS1, serverpackets.SystemMessageS1CPWillBeRestored
	}
	if e.ByOther {
		live.SendFrame(serverpackets.FrameSystemMessageStringNumber(byOther, e.HealerName, int32(e.Amount)))
		return
	}
	live.SendFrame(serverpackets.FrameSystemMessageNumber(self, int32(e.Amount)))
}

func (l *GameClientLink) refreshLiveItemStats(live *livePlayer) {
	if l.skills == nil {
		return
	}
	skillsChanged, timersChanged, err := l.skills.RefreshEquippedItemStats(live.Character, live.Inventory())
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("refresh grade-penalty item stats")
	}
	if skillsChanged {
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	}
	if timersChanged {
		now := live.Now()
		live.SendFrame(serverpackets.FrameSkillCoolTime(skillCoolTimeEntries(live.SkillReuseTimers(now), now)))
	}
}

func (l *GameClientLink) sendLiveWeightPenalty(live *livePlayer) {
	items := live.inventoryItems()
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(live.Character)))
	l.broadcastCharInfo(live, items)
}

// applyLiveDeathPenalty replaces the death-penalty skill's transient passive
// stats for the new level, then tells the client: a raise sends
// EtcStatusUpdate before the level message, a reduction the message first.
func (l *GameClientLink) applyLiveDeathPenalty(live *livePlayer, e event.DeathPenaltyChanged) {
	if l.skills != nil {
		if err := l.skills.ApplyTransientPassiveSkill(live.Character, 5076, e.Old, e.New); err != nil {
			l.log.Error().Err(err).Int32("object_id", live.ID).Msg("update death-penalty passive stats")
		}
	}
	etc := etcStatus(live.Character)
	etc.DeathPenaltyLevel = int32(e.New)
	if e.Raised {
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(etc))
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDeathPenaltyLevelS1Added, int32(e.New)))
		return
	}
	if e.New > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDeathPenaltyLevelS1Added, int32(e.New)))
	} else {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageDeathPenaltyLifted))
	}
	live.SendFrame(serverpackets.FrameEtcStatusUpdate(etc))
}

// finishQueuedBehindAttack runs the intention queued behind a swing or a
// bow shot that just ended, if any, and reports whether one was waiting:
// the queue holds at most one, which replaces the attack.
func (l *GameClientLink) finishQueuedBehindAttack(live *livePlayer) bool {
	live.replaceAttackWithQueuedCast()
	return l.finishDeferredPickup(live) ||
		l.finishDeferredMagicSkill(live) ||
		l.finishDeferredItemAICast(live) ||
		l.finishDeferredFollow(live) ||
		l.finishDeferredInteract(live) ||
		l.finishDeferredUseItem(live, resumeAttack)
}

// finishLiveCast resumes live's intentions once an in-flight cast of def on
// target ends. A stopped cast reports its end while it still counts as in
// flight, so a skill or item cast queued behind it is refused the way a cast
// requested mid-cast is: it is dropped, going idle, with ActionFailed. An
// attack, queued or the stopped cast's nextActionAttack follow-up, likewise
// finds the cast in flight: it swings nothing, and a target in reach is
// answered with ActionFailed. Any other queued intention runs as usual; the
// stop's own idle (endCastStop) then ends whatever walk it started.
func (l *GameClientLink) finishLiveCast(live *livePlayer, def modelskill.Definition, target attackable.Combatant, stopped bool) {
	if stopped && live.dropDeferredCast() {
		sendMagicActionFailed(live)
		return
	}
	if l.finishDeferredAction(live) {
		return
	}
	// A queued pickup, item cast or skill request replaced the cast that just
	// ended as the intention, whether it starts now or not: the ended cast's
	// nextActionAttack follow-up does not run.
	if l.finishDeferredPickup(live) {
		return
	}
	if l.finishDeferredItemAICast(live) {
		return
	}
	if l.finishDeferredMagicSkill(live) {
		return
	}
	if l.finishDeferredFollow(live) {
		return
	}
	if l.finishDeferredInteract(live) {
		return
	}
	if l.finishDeferredUseItem(live, resumeNothing) {
		return
	}
	if live.combat == nil {
		return
	}
	if resumed, actionFailed := live.combat.ResumeAfterCast(stopped); resumed {
		if actionFailed {
			live.SendFrame(serverpackets.FrameActionFailed())
		}
		return
	}
	live.endCastIntention(def, target, stopped)
}

// endCastStop ends a cast stop, once the stopped cast's end has run: the
// stop idles the player, then answers ActionFailed, and an interrupt reports
// itself last. An idle refused for a player unable to act answers
// ActionFailed of its own and leaves every intention in place. A stop that
// ended no cast, on a player that can act, idles it as idleAfterCastStop
// does: a swing or posture change under way goes on.
func (live *livePlayer) endCastStop(e event.CastStopAck) {
	if denied := live.Character.DenyAIActionBeforeEffect(); e.InFlight || denied {
		live.tryToIdle(denied)
	} else {
		live.idleAfterCastStop()
	}
	sendMagicActionFailed(live)
	if e.Broken {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCastingInterrupted))
	}
}

func (l *GameClientLink) finishDeferredAction(live *livePlayer) bool {
	return l.runDeferredAction(live, itemAICastBusy)
}

// runDeferredAction runs the queued action, if any, unless busy still holds
// it, and reports whether one was waiting.
func (l *GameClientLink) runDeferredAction(live *livePlayer, busy func(*livePlayer) bool) bool {
	if live == nil || live.detached() || !live.hasDeferredAction() {
		return false
	}
	if busy(live) {
		return true
	}
	if run := live.takeDeferredAction(); run != nil {
		if live.combat != nil {
			live.combat.Replace()
		}
		run()
		return true
	}
	return false
}

// endCastIntention ends the CAST intention a cast of def on target held,
// with nothing queued behind it: a skill carrying nextActionAttack attacks
// target when live may attack it without force, held with the cast's shift
// modifier; anything else, a toggle included, goes idle. A stopped cast
// still counts as in flight for that attack (attackAfterCast).
func (live *livePlayer) endCastIntention(def modelskill.Definition, target attackable.Combatant, stopped bool) {
	if live.combat == nil {
		return
	}
	_, shift := live.Character.CastModifiers()
	if live.attackAfterCast(def, target, shift, stopped) {
		return
	}
	live.combat.Stop()
}

// attackAfterCast starts the attack a nextActionAttack skill hands on to its
// final target, once its cast ends or is refused at its cost and condition
// checks, held with the cast's shift modifier: a shift-held follow-up never
// walks. It reports false, starting nothing, for any other skill, or a
// target live may not attack without force. After a stopped cast the attack
// is thought as if the cast were still in flight: it swings nothing.
func (live *livePlayer) attackAfterCast(def modelskill.Definition, target attackable.Combatant, shift, stopped bool) bool {
	if live.combat == nil || !def.NextActionIsAttack || target == nil {
		return false
	}
	rules, ok := target.(skilltarget.Actor)
	if !ok || !rules.AttackableWithoutForceBy(live.Character) {
		return false
	}
	if live.combat.AttackAfterCast(target, shift, stopped) {
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	return true
}

// stopLiveActions stops what e names in target, movement, attack, cast
// order. Clearing the target leaves the intentions alone. Stopping the
// attack sends the character idle, then answers ActionFailed; stopping the
// cast answers MagicSkillCanceled (when one was running), sends the
// character idle and answers ActionFailed (endCastStop).
func (l *GameClientLink) stopLiveActions(live *livePlayer, e event.ActionsStopRequested) {
	if e.ClearTarget {
		old := live.Target()
		live.StoreTarget(nil)
		l.announceTargetCleared(live, old)
	}
	if e.Move {
		live.move.Stop()
	}
	if e.Attack {
		live.attack.Stop()
		live.tryToIdle(e.AIDenied)
		live.SendFrame(serverpackets.FrameActionFailed())
	}
	if e.Cast {
		live.stopCastInFlight()
	}
}

// reviveRefusalMessage is the system message a refused resurrection offer
// sends its reviver.
func reviveRefusalMessage(reason event.ReviveRefusal) int {
	switch reason {
	case event.RevivePetWhileOwnerPending:
		return serverpackets.SystemMessageCannotResPet2
	case event.ReviveOwnerWhilePetPending:
		return serverpackets.SystemMessageMasterCannotRes
	default:
		return serverpackets.SystemMessageResHasAlreadyBeenProposed
	}
}

// etcStatus is c's status-window flags as EtcStatusUpdate reports them.
func etcStatus(c *player.Character) serverpackets.EtcStatus {
	return serverpackets.EtcStatus{
		Charges:           int32(c.Charges()),
		WeightPenalty:     int32(c.WeightPenalty()),
		Blocked:           c.BlockingAll() || c.ChatBanned(),
		DangerArea:        c.InDangerArea(),
		GradePenalty:      c.WeaponGradePenalty() || c.ArmorGradePenalty() > 0,
		CharmOfCourage:    c.CharmOfCourage(),
		DeathPenaltyLevel: int32(c.DeathPenaltyLevel()),
	}
}
