package network

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/manager"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (l *GameClientLink) authenticate(ctx context.Context, client *Client, req clientpackets.AuthLogin) (bool, error) {
	loginLink := l.loginLink()
	if loginLink == nil {
		client.Session.SendFrame(serverpackets.FrameAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater))
		return false, nil
	}
	return l.validator.Validate(ctx, client, req, loginLink)
}

// sendCharSelectInfo lists client's characters, sends the resulting
// CharSelectInfo, and returns the list so the caller can cache it for
// subsequent slot-addressed requests.
func (l *GameClientLink) sendCharSelectInfo(ctx context.Context, client *Client) ([]*player.Character, error) {
	chars, err := l.roster.List(ctx, client.AccountName())
	if err != nil {
		return nil, err
	}

	slots := make([]serverpackets.CharacterSlot, len(chars))
	now := time.Now()
	for i, c := range chars {
		items, err := l.items.ListByOwner(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		slots[i] = serverpackets.NewCharacterSlot(c, items, now)
	}

	client.Session.SendFrame(serverpackets.FrameCharSelectInfo(client.AccountName(), client.SessionKey().PlayKey1, slots, -1))
	return chars, nil
}

// enterWorld sends the EnterWorld packet burst for c and registers it in the
// live world state.
func (l *GameClientLink) enterWorld(ctx context.Context, client *Client, c *player.Character) (*livePlayer, bool) {
	tmpl, ok := l.templates.Get(c.ClassID)
	if !ok {
		l.log.Error().Int("class_id", c.ClassID).Msg("enter world: no template loaded")
		return nil, false
	}
	items, err := l.items.ListByOwner(ctx, c.ID)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: list items")
		return nil, false
	}
	if l.skills != nil {
		if err := l.skills.Restore(ctx, c); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: restore skill state")
			return nil, false
		}
		// Re-derive level-unlocked skills on every login (Player.java:4139
		// calls giveSkills() right after restoreCharData()), so a free grant
		// added by an in-session level-up — which lives in memory only —
		// comes back instead of vanishing on relog.
		if err := l.giveOrRewardSkills(ctx, c, tmpl); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: give skills")
			return nil, false
		}
		if level := c.DeathPenaltyLevel(); level > 0 {
			if err := l.skills.ApplyTransientPassiveSkill(c, 5076, 0, level); err != nil {
				l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: restore death-penalty passive stats")
				return nil, false
			}
		}
	}
	if c.ResourceValues().CurrentHP < 0.5 {
		c.MarkDead()
	}
	shortcuts := shortcut.Starter()
	if l.shortcuts != nil {
		restored, listErr := l.shortcuts.ListByOwner(ctx, c.ID)
		if listErr != nil {
			l.log.Error().Err(listErr).Msg("enter world: list shortcuts")
			shortcuts = nil
		} else {
			shortcuts = restored
		}
	}

	itemListFrame, err := serverpackets.FrameItemList(items, l.itemTemplates, false)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: build ItemList")
		return nil, false
	}
	live, err := l.attachLivePlayer(ctx, client, c, tmpl, items, shortcuts)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: attach live player")
		return nil, false
	}
	l.activateSpawnProtection(live)
	if l.skills != nil {
		if err := l.skills.RestoreEquippedItemStats(c, c.Inventory()); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: restore equipped item stats")
			return nil, false
		}
	}
	// Computed after RestoreEquippedItemStats so an item's equip-delay reuse
	// timer, armed by that restore, is included in the login SkillCoolTime
	// snapshot rather than missed.
	now := time.Now()
	coolTimes := skillCoolTimeEntries(c.SkillReuseTimers(now), now)
	c.RefreshWeightPenalty()
	skillList := skillListEntries(c, l.skills)
	if l.world != nil {
		x, y, z := c.Position()
		l.world.Spawn(live, x, y, z, c.LastHeading)
		l.world.AddPlayer(live)
		if l.zones != nil && live.zoneActor != nil {
			live.zoneActor.revalidate(l.zones)
		}
	}
	// Track this player for the in-game clock's activity reminder so the
	// PLAYING_FOR_LONG_TIME send reaches them every 720 game minutes.
	if l.playerClock != nil {
		l.playerClock.Add(live)
	}
	// Reproduce GameClient's _autoSaveInDB: first full-stat save 5 minutes
	// after entering the world, then every 15 minutes while still online.
	if l.autosave != nil {
		l.autosave.Add(live)
	}

	client.Session.SendFrame(serverpackets.FrameExStorageMaxCount(c))
	client.Session.SendFrame(serverpackets.FrameHennaInfo(c.ClassID))
	// Replay restored buffs into the live effect list here, matching
	// EnterWorld.java:100's player.updateEffectIcons() position: List.Add's
	// notifyAbnormalUpdate hook fires the resulting AbnormalStatusUpdate
	// frame (if any effect was restored) right where the reference sends it,
	// ahead of EtcStatusUpdate.
	if l.skills != nil {
		l.skills.ReplayEffects(c)
	}
	client.Session.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{WeightPenalty: int32(c.WeightPenalty()), GradePenalty: c.WeaponGradePenalty() || c.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(c.DeathPenaltyLevel())}))
	client.Session.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWelcomeToLineage))
	if l.playerClock != nil && c.Race == player.RaceDarkElf {
		l.playerClock.NotifyShadowSenseState(live)
	}
	client.Session.SendFrame(serverpackets.FrameQuestList(nil))
	client.Session.SendFrame(serverpackets.FrameSkillList(skillList))
	client.Session.SendFrame(serverpackets.FrameFriendList(nil))
	client.Session.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{Character: c, Template: tmpl, Items: items, IsGM: live.isGM}))
	client.Session.SendFrame(itemListFrame)
	client.Session.SendFrame(serverpackets.FrameShortCutInit(serverShortcutList(live.shortcuts.All())))
	if c.Dead() {
		client.Session.SendFrame(serverpackets.FrameDie(c.ObjectID(), l.dieOptions(c)))
	}
	client.Session.SendFrame(serverpackets.FrameSkillCoolTime(coolTimes))
	client.Session.SendFrame(serverpackets.FrameActionFailed())
	return live, true
}

func (l *GameClientLink) dieOptions(c *player.Character) serverpackets.DieOptions {
	return serverpackets.DieOptions{FixedRes: resolveFixedRes(l.admin, c.AccessLevel)}
}

// socialActionLevelUp is the social animation id played for everyone who can
// see a character that just gained a level.
const socialActionLevelUp = 15

// expSpGainMessage picks the single system message that reports one
// experience/SP gain. The amounts pick the message: SP alone, experience
// alone, or the combined one, which also covers a gain of nothing.
func expSpGainMessage(exp int64, sp int) wire.Frame {
	switch {
	case exp == 0 && sp > 0:
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageAcquiredS1SP, int32(sp))
	case exp > 0 && sp == 0:
		return serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageEarnedS1Experience, int32(exp))
	default:
		return serverpackets.FrameSystemMessageTwoNumbers(serverpackets.SystemMessageYouEarnedS1ExpAndS2SP, int32(exp), int32(sp))
	}
}

// sendExpSpLossFrames tells live's own client how much experience and SP a
// removal took. PlayerStatus.setSp sends StatusUpdate(SP) synchronously
// during PlayableStatus.removeExpAndSp, before removeExpAndSp's own system
// messages go out, so a combined removal orders StatusUpdate(SP) ahead of
// EXP_DECREASED_BY_S1 (PlayerStatus.java:583-603, PlayableStatus.java:133-145,
// PlayerStatus.java:881-891).
func sendExpSpLossFrames(live *livePlayer, exp int64, sp int) {
	if sp > 0 {
		live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusSP, Value: live.SP},
		}))
	}
	if exp > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageExpDecreasedByS1, int32(exp)))
	}
	if sp > 0 {
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageSPDecreasedS1, int32(sp)))
	}
}

// sendKarmaChangeFrames tells live's own client its new karma total:
// SystemMessage(YOUR_KARMA_HAS_BEEN_CHANGED_TO_S1) followed by
// StatusUpdate(KARMA), the same order Player.setKarma sends them in
// (Player.java:1076-1080).
func sendKarmaChangeFrames(live *livePlayer, karma int) {
	live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageYourKarmaHasBeenChangedToS1, int32(karma)))
	live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusKarma, Value: karma},
	}))
}

// giveOrRewardSkills re-derives c's level-unlocked skills, calling
// RewardSkills instead of GiveSkills whenever the server grants every
// available skill automatically (Player.java:3256-3257).
func (l *GameClientLink) giveOrRewardSkills(ctx context.Context, c *player.Character, tmpl *player.Template) error {
	refresh := l.skills.GiveSkills
	if l.playerConfig.AutoLearnSkills {
		refresh = l.skills.RewardSkills
	}
	return refresh(ctx, c, tmpl)
}

// refreshLiveLevelSkills re-derives the skills live's new level entitles it
// to and hands the client the resulting list. It runs on every level change,
// up or down, as the level refresher attachLivePlayer registers.
//
// The skill list goes out even when the refresh failed part-way: the
// character's in-memory skills have already moved, so the client's copy is
// stale either way, and resending is what makes the two agree again.
func (l *GameClientLink) refreshLiveLevelSkills(ctx context.Context, live *livePlayer) {
	if l.skills == nil || live == nil {
		return
	}
	rewarding := l.playerConfig.AutoLearnSkills
	before := live.SkillLevels()
	if err := l.giveOrRewardSkills(ctx, live.Character, live.template); err != nil {
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("level change: refresh level skills")
	}
	live.RefreshExpertisePenalty()
	live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))

	// RewardSkills is the Go equivalent of Player.rewardSkills, whose grant
	// loop calls addSkill(..., updateShortcuts=true) (Player.java:3283) only
	// for skills the grant loop's own filter (getSkillLevel(i) < s.getValue(),
	// Player.java:3423) restricts to level increases; GiveSkills's addSkill
	// calls always pass false (Player.java:3262), and RewardSkills' own
	// pull-back correction (correctInvalidSkills, mirroring
	// removeInvalidSkills' addSkill(..., true) two-arg call at
	// Player.java:3333,3337) also passes false. So only an actual level
	// increase refreshes shortcuts; a pull-back's level decrease must not.
	if rewarding {
		after := live.SkillLevels()
		for id, level := range after {
			if level > before[id] {
				l.refreshSkillShortcuts(ctx, live, int32(id), int32(level))
			}
		}
	}
}

func skillCoolTimeEntries(timers []effect.ReuseTimer, now time.Time) []serverpackets.SkillCoolTimeEntry {
	if len(timers) == 0 {
		return nil
	}
	nowMillis := now.UnixMilli()
	entries := make([]serverpackets.SkillCoolTimeEntry, 0, len(timers))
	for _, timer := range timers {
		remaining := timer.ExpiresAt - nowMillis
		if remaining <= 0 {
			continue
		}
		entries = append(entries, serverpackets.SkillCoolTimeEntry{
			SkillID:          int32(timer.Skill.ID),
			Level:            int32(timer.Skill.Level),
			ReuseSeconds:     int32(timer.Delay / 1000),
			RemainingSeconds: int32(remaining / 1000),
		})
	}
	return entries
}

func skillListEntries(c *player.Character, skills *skillstate.Persistence) []serverpackets.SkillListEntry {
	if c == nil {
		return nil
	}
	levels := c.SkillLevels()
	if len(levels) == 0 {
		return nil
	}
	ids := make([]int, 0, len(levels))
	for id := range levels {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	entries := make([]serverpackets.SkillListEntry, 0, len(ids))
	for _, id := range ids {
		level := levels[id]
		if level <= 0 {
			continue
		}
		entry := serverpackets.SkillListEntry{ID: int32(id), Level: int32(level)}
		if skills != nil {
			if def, ok := skills.Definition(modelskill.Ref{ID: modelskill.ID(id), Level: level}); ok {
				entry.Passive = def.Activation == modelskill.ActivationPassive
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func (l *GameClientLink) attachLivePlayer(ctx context.Context, client *Client, c *player.Character, tmpl *player.Template, items []*item.Instance, shortcuts []shortcut.Shortcut) (*livePlayer, error) {
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, l.itemTemplates, items))
	// Filter/populate ITEM shortcuts against the live inventory just attached
	// above, mirroring ShortcutList.restore() (ShortcutList.java:173-209): a
	// stale ITEM shortcut (its item consumed/traded/destroyed since last
	// logout) is dropped, and every surviving one gets SharedReuseGroup from
	// its item's etc-item data.
	shortcuts = shortcut.RestoreItemShortcuts(shortcuts, func(objectID int32) (int32, bool) {
		inst := c.Inventory().ItemByObjectID(objectID)
		if inst == nil {
			return 0, false
		}
		tmpl, ok := l.itemTemplates.Get(inst.TemplateID)
		if !ok || tmpl.EtcItem == nil {
			return -1, true
		}
		return tmpl.EtcItem.SharedReuseGroup, true
	})
	c.SetWeightLimitMultiplier(l.playerConfig.WeightLimitMultiplier)
	c.SetDeathPenaltyChance(l.playerConfig.DeathPenaltyChance)
	c.SetPerfectShieldBlockRate(l.playerConfig.PerfectShieldBlockRate)
	c.RefreshWeightPenalty()
	c.RefreshExpertisePenalty()
	c.SetWorld(l.world)
	if los, ok := l.geo.(player.LineOfSight); ok {
		c.SetLineOfSight(los)
	}
	if l.zones != nil {
		c.SetZones(l.zones)
	}
	c.SetFrameSender(client.Session.SendFrame)
	c.SetBroadcastFrameSender(client.Session.TrySendFrame)
	c.SetLogger(l.log)

	x, y, z := c.Position()
	creatureLive, err := creature.NewLive(location.Location{X: x, Y: y, Z: z}, c.RunSpeed(), l.geo, c)
	if err != nil {
		return nil, fmt.Errorf("attach live player: %w", err)
	}
	c.AttachLive(creatureLive)
	moveCtl, err := move.NewController(c.Move(), c)
	if err != nil {
		return nil, fmt.Errorf("attach live player: %w", err)
	}
	moveCtl.SetPositionUpdates(l.positions)
	attackCtl := attack.NewPlayer(c)
	c.Move().SetLogger(l.log)
	attackCtl.SetLogger(l.log)
	combat := ai.NewPlayerAttack(c, moveCtl, attackCtl)

	c.SetCanGiveDamage(resolveCanGiveDamage(l.admin, c.AccessLevel))
	live := &livePlayer{Character: c, template: tmpl, npcs: l.npcs, items: items, attack: attackCtl, move: moveCtl, combat: combat, shortcuts: shortcut.NewList(shortcuts), isGM: resolveIsGM(l.admin, c.AccessLevel), visibilitySend: client.Session.TrySendFrame, stopAttack: l.stopLiveAutoAttack, log: l.log}
	live.zoneActor = &liveZoneActor{live: live}
	c.SetSummonConfirmSender(func(casterName string, casterID int32, x, y, z int, timeout time.Duration) {
		live.SendFrame(serverpackets.FrameConfirmDlgSummonFriendRequest(casterName, casterID, int32(x), int32(y), int32(z), timeout))
	})
	c.SetTeleportHook(func(x, y, z, radius int) {
		l.teleportLivePlayer(live, location.Location{X: x, Y: y, Z: z}, radius)
	})
	// Build cast eagerly, like attackCtl above: pickup-lock's timer goroutine
	// reads live.cast unguarded, so a lazy first write from the read-loop
	// goroutine would race it (issue #1183).
	l.castController(live)
	c.SetZoneRevalidator(func(previous location.Location) {
		l.revalidateZones(live, previous)
	})
	attackCtl.SetFinished(func() {
		l.finishDeferredPickup(live)
		l.finishDeferredMagicSkill(live)
		l.finishDeferredItemAICast(live)
		combat.Think()
	})
	attackCtl.SetStarted(func() {
		l.startLiveAutoAttack(live)
	})
	moveCtl.SetArrived(func() {
		// CreatureMove tracks position for its own timing only; push the
		// arrived position into the world-grid presence range checks
		// actually read before re-thinking the attack intention, or it
		// re-evaluates against a stale position forever.
		pos := moveCtl.Position()
		l.updateLivePlayerPosition(live, pos, live.CurrentHeading())
		l.finishLiveGroundPickup(live)
		l.finishPetInteract(live)
		l.finishDeferredMagicSkill(live)
		combat.Think()
	})
	c.SetAttackBroadcaster(func(snapshot attack.Snapshot) {
		l.broadcastAttack(live, snapshot)
	})
	c.SetMoveBroadcaster(func(event move.Event) {
		l.broadcastLiveMoveEvent(live, event)
	})
	c.SetFlightBroadcaster(func(dest location.Location, flight modelskill.Flight) {
		at := live.CurrentLocation()
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameFlyToLocation(live.ObjectID(), dest, at, flight)
		})
	})
	c.SetPositionBroadcaster(func() {
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameValidateLocation(live.ObjectID(), live.CurrentLocation(), live.CurrentHeading())
		})
	})
	c.SetStopBroadcaster(func() {
		x, y, z := live.Position()
		l.broadcastLiveStopMove(live, location.Location{X: x, Y: y, Z: z}, live.CurrentHeading())
	})
	c.SetStanceBroadcaster(func(stance player.Stance) {
		waitType := serverpackets.WaitSitting
		switch stance {
		case player.StanceStanding:
			waitType = serverpackets.WaitStanding
		case player.StanceFakeDeathStart:
			waitType = serverpackets.WaitFakeDeathStart
		case player.StanceFakeDeathStop:
			waitType = serverpackets.WaitFakeDeathStop
		}
		x, y, z := live.Position()
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameChangeWaitType(live.ObjectID(), waitType, location.Location{X: x, Y: y, Z: z})
		})
	})
	c.SetFakeDeathReviveBroadcaster(func() { l.broadcastLiveRevive(live) })
	c.SetDieBroadcaster(func() {
		if l.water != nil {
			l.water.Remove(live)
		}
		l.broadcastLiveDie(live)
	})
	c.SetStatusBroadcaster(func() {
		l.broadcastLiveStatus(live)
	})
	c.SetMPStatusBroadcaster(func() {
		l.broadcastLiveMPStatus(live)
	})
	c.SetAbnormalEffectUpdater(func() {
		l.updateLiveAbnormalEffect(live)
	})
	c.SetAbnormalEffectBroadcaster(func() {
		l.broadcastCharacterInfo(live)
	})
	c.SetExpSpGainNotifier(func(exp int64, sp int) {
		live.SendFrame(expSpGainMessage(exp, sp))
	})
	c.SetExpSpLossNotifier(func(exp int64, sp int) {
		sendExpSpLossFrames(live, exp, sp)
	})
	c.SetKarmaChangeNotifier(func(karma int) {
		sendKarmaChangeFrames(live, karma)
	})
	c.SetAwardPKKillPVPPoint(l.playerConfig.AwardPKKillPVPPoint)
	c.SetRelationBroadcaster(func() {
		l.broadcastRelations(live)
	})
	c.SetPvPFlagHook(func(useFlaggedDuration bool) {
		if l.pvpFlags == nil {
			return
		}
		if useFlaggedDuration {
			l.pvpFlags.AddFlagged(c)
			return
		}
		l.pvpFlags.AddNormal(c)
	})
	c.SetLevelUpBroadcaster(func() {
		l.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameSocialAction(live.ObjectID(), socialActionLevelUp)
		})
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageYouIncreasedYourLevel))
	})
	c.SetUserInfoUpdater(func() {
		live.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{
			Character: live.Character, Template: live.template, Items: live.inventoryItems(), IsGM: live.isGM,
		}))
	})
	c.SetChargesUpdater(func() {
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{Charges: int32(live.Charges()), WeightPenalty: int32(live.WeightPenalty()), GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(live.DeathPenaltyLevel())}))
	})
	c.SetChargeMessageSender(func(charges int, maxed bool) {
		if maxed {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageForceMaxLevelReached))
			return
		}
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageForceIncreasedToS1, int32(charges)))
	})
	c.SetGradePenaltyUpdater(func() {
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(live.DeathPenaltyLevel())}))
	})
	c.SetWeightPenaltyUpdater(func() {
		items := live.inventoryItems()
		live.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{Character: live.Character, Template: live.template, Items: items, IsGM: live.isGM}))
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{WeightPenalty: int32(live.WeightPenalty()), GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(live.DeathPenaltyLevel())}))
		if l.world != nil {
			info := serverpackets.CharInfoSnapshot{Character: live.Character, Template: live.template, Items: items}
			broadcastFrame(func() wire.Frame { return serverpackets.FrameCharInfo(info) }, func(send func(frameReceiver)) {
				l.world.ForEachKnown(live, func(o world.Tracked) {
					if receiver, ok := o.(frameReceiver); ok {
						send(receiver)
					}
				})
			})
		}
	})
	c.SetDeathPenaltyRaisedUpdater(func(level int) {
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{WeightPenalty: int32(live.WeightPenalty()), GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(level)}))
		live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDeathPenaltyLevelS1Added, int32(level)))
	})
	c.SetDeathPenaltyReducedUpdater(func(level int) {
		if level > 0 {
			live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageDeathPenaltyLevelS1Added, int32(level)))
		} else {
			live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageDeathPenaltyLifted))
		}
		live.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{WeightPenalty: int32(live.WeightPenalty()), GradePenalty: live.WeaponGradePenalty() || live.ArmorGradePenalty() > 0, DeathPenaltyLevel: int32(level)}))
	})
	if l.skills != nil {
		c.SetDeathPenaltySkillUpdater(func(oldLevel, level int) {
			if err := l.skills.ApplyTransientPassiveSkill(c, 5076, oldLevel, level); err != nil {
				l.log.Error().Err(err).Int32("object_id", c.ID).Msg("update death-penalty passive stats")
			}
		})
	}
	c.SetItemStatsRefresher(func() {
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
	})
	c.SetLevelRefresher(func() { l.refreshLiveLevelSkills(ctx, live) })
	// Register the inventory with the batching task the moment it queues an
	// update, matching the reference's Inventory.addUpdate registering with
	// InventoryUpdateTaskManager on every mutation. The task is the only
	// drainer; it sends InventoryUpdate on its own 333ms cadence.
	if inv := c.Inventory(); inv != nil && l.inventoryUpdates != nil {
		inv.SetUpdateNotifier(func() {
			l.inventoryUpdates.Add(inv, live)
		})
	}
	if inv := c.Inventory(); inv != nil {
		inv.SetWeightNotifier(func() {
			live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
				{Type: serverpackets.StatusCurrentLoad, Value: inv.TotalWeight()},
			}))
			c.RefreshWeightPenalty()
		})
		// Restored rows never queue update notifications, so totalWeight stays
		// 0 unless recomputed here, matching the reference's ItemList
		// constructor calling PcInventory.updateWeight() on every send
		// (including the one EnterWorld makes right after this).
		inv.UpdateWeight()
	}
	// Register every item mutation with the lazy persistence task, matching
	// the reference registering an item with ItemInstanceTaskManager from
	// the item's own setters: a count, location, enchant or mana change made
	// without a client request still reaches the items table on the task's
	// own cadence.
	if inv := c.Inventory(); inv != nil && l.itemInstances != nil {
		inv.SetItemPersister(l.itemInstances.Add)
	}
	if inv := c.Inventory(); inv != nil && l.shadowItems != nil {
		for _, inst := range inv.PaperdollItems() {
			tmpl, ok := inv.Templates().Get(inst.TemplateID)
			if ok {
				l.shadowItems.Track(live.ObjectID(), inst, tmpl)
			}
		}
	}
	c.SetShortBuffBroadcaster(func(update player.ShortBuffUpdate) {
		live.SendFrame(serverpackets.FrameShortBuffStatusUpdate(update.SkillID, update.Level, update.DurationSeconds))
	})
	c.SetRegenMaxSender(func(count, period int32, hpRegen float64) {
		live.SendFrame(serverpackets.FrameExRegenMax(count, period, hpRegen))
	})
	c.SetLackHPNotifier(func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillRemovedDueLackHP))
	})
	c.SetLackMPNotifier(func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillRemovedDueLackMP))
	})
	c.SetRelaxHPFullNotifier(func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSkillDeactivatedHPFull))
	})
	c.SetSpoilNotifiers(func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAlreadySpoiled))
	}, func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSpoilSuccess))
	})
	c.SetServitorVanishedNotifier(func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageServitorHasVanished))
	})
	c.SetShieldBlockNotifiers(func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageShieldDefenceSuccessful))
	}, func() {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageExcellentShieldDefenseSuccess))
	})
	c.SetAttackTargetHook(func(target world.Tracked) {
		l.attackLiveTarget(live, target)
	})
	c.SetRetargetHook(func(target world.Tracked) {
		if target == nil {
			l.clearLiveTarget(live)
			return
		}
		l.selectLiveTarget(live, target)
	})
	c.SetHerbConsumer(func(itemID int32) {
		l.consumeHerb(live, itemID)
	})
	return live, nil
}

func slotCharacter(chars []*player.Character, slot int32) (*player.Character, bool) {
	if slot < 0 || int(slot) >= len(chars) {
		return nil, false
	}
	return chars[slot], true
}

func createFailReason(outcome manager.CreateOutcome) serverpackets.CharCreateFailReason {
	switch outcome {
	case manager.CreateTooManyCharacters:
		return serverpackets.CharCreateFailReasonTooManyCharacters
	case manager.CreateNameTaken:
		return serverpackets.CharCreateFailReasonNameAlreadyExists
	case manager.CreateInvalidName:
		return serverpackets.CharCreateFailReasonIncorrectName
	default:
		return serverpackets.CharCreateFailReasonCreationFailed
	}
}
