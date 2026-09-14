package network

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// SendFrame sends frame to this player's client. Once the session has
// detached it releases frame and reports false.
func (p *livePlayer) SendFrame(frame wire.Frame) bool {
	if p.session == nil || p.SessionDetached() {
		frame.Release()
		return false
	}
	return p.session(frame)
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
	case event.Attack, event.BowDrawn, event.Died, event.HerbConsumed,
		event.RegenMax, event.EffectRemovedLackHP, event.EffectRemovedLackMP,
		event.RelaxHPFull, event.Restored, event.EffectEnded, event.SpoilResult,
		event.ServitorVanished, event.ShieldBlocked, event.AttackFailed,
		event.SkillResisted, event.MagicResisted, event.UserInfoChanged,
		event.PvPFlagged, event.RelationChanged, event.LevelChanged,
		event.WeightPenaltyChanged:
		return true
	}
	return false
}

// Emit maps one of p's character events to its packets and follow-up
// actions. Each arm keeps the send order its packets reach clients in.
func (p *livePlayer) Emit(ev event.Event) {
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
	case event.FakeDeathRevived:
		l.broadcastLiveRevive(live)
	case event.Died:
		if l.water != nil {
			l.water.Remove(live)
		}
		l.broadcastLiveDie(live)
	case event.VitalsChanged:
		if e.IncludeMP {
			l.broadcastLiveMPStatus(live)
			return
		}
		l.broadcastLiveStatus(live)
	case event.EffectIconsChanged:
		l.updateLiveAbnormalEffect(live)
	case event.AbnormalEffectChanged:
		l.broadcastCharacterInfo(live)
	case event.ExpSPGained:
		live.SendFrame(expSpGainMessage(e.Exp, e.SP))
	case event.ExpSPLost:
		sendExpSpLossFrames(live, e.Exp, e.SP)
	case event.KarmaChanged:
		sendKarmaChangeFrames(live, e.Karma)
	case event.RelationChanged:
		l.broadcastRelations(live)
	case event.PvPFlagged:
		if l.pvpFlags == nil {
			return
		}
		if e.UseFlaggedDuration {
			l.pvpFlags.AddFlagged(live.Character)
			return
		}
		l.pvpFlags.AddNormal(live.Character)
	case event.LeveledUp:
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameSocialAction(live.ObjectID(), socialActionLevelUp)
		})
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouIncreasedYourLevel))
	case event.UserInfoChanged:
		live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	case event.ChargesChanged:
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{Charges: int32(live.Charges()), WeightPenalty: int32(live.WeightPenalty()), GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(live.DeathPenaltyLevel())}))
	case event.ChargeMessage:
		if e.Maxed {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageForceMaxLevelReached))
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageForceIncreasedToS1, int32(e.Charges)))
	case event.GradePenaltyChanged:
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(live.DeathPenaltyLevel())}))
		l.refreshLiveItemStats(live)
	case event.WeightPenaltyChanged:
		l.sendLiveWeightPenalty(live)
	case event.DeathPenaltyChanged:
		l.applyLiveDeathPenalty(live, e)
	case event.LevelChanged:
		l.refreshLiveLevelSkills(p.ctx, live)
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
	case event.SpoilResult:
		if e.Already {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAlreadySpoiled))
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSpoilSuccess))
	case event.OverHit:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageOverHit))
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
	case event.SkillResisted:
		live.SendFrame(serverpackets.FrameSystemMessageStringSkillName(serverpackets.SystemMessageS1ResistedYourS2, e.TargetName, int32(e.SkillID), int32(e.Level)))
	case event.MagicResisted:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageResistedS1Magic, e.AttackerName))
	case event.AttackRequested:
		if target, ok := e.Target.(world.Tracked); ok {
			l.attackLiveTarget(live, target)
		}
	case event.Retargeted:
		target, _ := e.Target.(world.Tracked)
		if target == nil {
			l.clearLiveTarget(live)
			return
		}
		l.selectLiveTarget(live, target)
	case event.HerbConsumed:
		l.consumeHerb(live, e.ItemID)
	case event.SummonConfirmRequested:
		live.SendFrame(serverpackets.FrameConfirmDlgSummonFriendRequest(e.CasterName, e.CasterID, int32(e.X), int32(e.Y), int32(e.Z), e.Timeout))
	case event.TeleportRequested:
		l.teleportLivePlayer(live, location.Location{X: e.X, Y: e.Y, Z: e.Z}, e.Radius)
	case event.Relocated:
		l.revalidateZones(live, e.Previous)
	case event.AttackStarted:
		l.startLiveAutoAttack(live)
	case event.AttackFinished:
		l.finishDeferredPickup(live)
		l.finishDeferredMagicSkill(live)
		l.finishDeferredItemAICast(live)
		live.combat.Think()
	case event.Arrived:
		// CreatureMove tracks position for its own timing only; push the
		// arrived position into the world-grid presence range checks
		// actually read before re-thinking the attack intention, or it
		// re-evaluates against a stale position forever.
		pos := live.move.Position()
		l.updateLivePlayerPosition(live, pos, live.CurrentHeading())
		l.finishLiveGroundPickup(live)
		l.finishPetInteract(live)
		l.finishDeferredMagicSkill(live)
		live.combat.Think()
	case event.MoveBlocked:
		if !l.onPlayerArrivedBlocked(live) {
			live.move.BroadcastBlockedCorrection()
		}
	case event.CastAborted:
		l.broadcastCastAborted(live, e.Interrupted)
	case event.CastStopAck:
		sendMagicActionFailed(live)
	case event.CastFinished:
		l.finishLiveCast(live, e.Skill)
	case event.PetSummonRequested:
		spawner := p.summonSpawner.Load()
		if spawner == nil {
			return
		}
		if controlItem, ok := e.ControlItem.(*item.Instance); ok {
			spawner.SpawnPet(live.Character, controlItem)
		}
	case event.ServitorSummonRequested:
		if spawner := p.summonSpawner.Load(); spawner != nil {
			spawner.SpawnServitor(live.Character, e.Skill)
		}
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
		now := time.Now()
		live.SendFrame(serverpackets.FrameSkillCoolTime(skillCoolTimeEntries(live.SkillReuseTimers(now), now)))
	}
}

func (l *GameClientLink) sendLiveWeightPenalty(live *livePlayer) {
	items := live.inventoryItems()
	live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	live.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{WeightPenalty: int32(live.WeightPenalty()), GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(live.DeathPenaltyLevel())}))
	if l.world == nil {
		return
	}
	info := serverpackets.CharInfoSnapshot{Character: live.Character, Template: live.template, Items: items}
	broadcastFrame(func() wire.Frame { return serverpackets.FrameCharInfo(info) }, func(send func(frameReceiver)) {
		l.world.ForEachKnown(live, func(o world.Tracked) {
			if receiver, ok := o.(frameReceiver); ok {
				send(receiver)
			}
		})
	})
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
	etc := serverpackets.EtcStatus{WeightPenalty: int32(live.WeightPenalty()), GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(e.New)}
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

// finishLiveCast resumes live's intentions once an in-flight cast ends.
func (l *GameClientLink) finishLiveCast(live *livePlayer, def modelskill.Definition) {
	if l.finishDeferredItemAICast(live) {
		return
	}
	if live.combat == nil {
		return
	}
	if live.combat.ResumeAfterCast() {
		return
	}
	// A queued CAST already ran above. With no next intention, a finished
	// CAST only re-engages the attack when the skill carries
	// nextActionAttack; anything else goes idle.
	if def.NextActionIsAttack {
		live.combat.Think()
		return
	}
	live.combat.Stop()
}
