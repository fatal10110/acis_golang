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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

func (l *GameClientLink) authenticate(ctx context.Context, client *Client, req clientpackets.AuthLogin) (bool, error) {
	loginLink := l.loginLink()
	if loginLink == nil {
		client.Session.SendFrame(serverpackets.FrameAuthLoginFail(serverpackets.LoginFailSystemErrorTryLater))
		return false, nil
	}
	// A second AuthLogin for an account already claimed takes it over: the
	// prior connection's own in-flight validation (if any) is unblocked with
	// a rejection so it releases the account promptly, then its session is
	// closed, matching LoginServerThread.addClient's closeNow() ahead of the
	// new PlayerAuthRequest (LoginServerThread.java:292-304).
	if l.clients != nil {
		if evicted, replaced := l.clients.Take(req.LoginName, client); replaced {
			l.validator.Resolve(req.LoginName, false)
			evicted.Session.Close()
		}
	}
	ok, err := l.validator.Validate(ctx, client, req, loginLink)
	if l.clients != nil && (!ok || err != nil) {
		l.clients.Release(req.LoginName, client)
	}
	return ok, err
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

// restoreItemRows resolves the rows a login restores an inventory from
// against the changes the lazy item persistence task has not written yet, and
// returns the items the inventory is actually rebuilt from.
//
// The lazy task is what makes this necessary. Destroying a whole stack takes
// the instance out of its container in memory and leaves the row's delete to
// the next tick, and the detach flush only writes the items the container
// still holds — so a logout inside the tick window leaves the destroyed
// item's row in place. Without a check here the next login reads that row
// back and hands the player the stack again.
//
// The pending set outranks the table either way: an entry means the row is
// stale. What an entry does not say on its own is whether the item left the
// container or whether its write simply has not landed — the detach flush
// keeps a container pending when its write fails or times out, exactly so the
// next tick still has it — and those two need opposite answers here. The
// write's own state settles it, because that state is what the row will hold
// once the write lands, so the task resolves them and hands back the ones
// still owned (ItemInstances.ClaimRestoredItems): those are restored from that
// state rather than from the row, which is both the freshest description of
// the item and the one the failed write was carrying. Claiming them also
// makes the rebuilt instance the row's only writer from here on.
//
// A row whose item template is no longer loaded is dropped so that a datapack
// downgrade costs the player that one item rather than the whole login.
//
// A departed row is logged because this is the one place in the login path
// that removes rows a player may still legitimately own. The normal case is a
// small count — one destroyed or traded stack — and a count covering the whole
// inventory would mean the pending set and the state it holds disagree about
// the same items, which is a bug here rather than a detach flush that never
// landed: those items are claimed and restored, not dropped.
func (l *GameClientLink) restoreItemRows(ownerID int32, items []*item.Instance) []*item.Instance {
	var claimed map[int32]item.InstanceState
	if l.itemInstances != nil {
		ids := make([]int32, 0, len(items))
		for _, inst := range items {
			if inst != nil {
				ids = append(ids, inst.ObjectID)
			}
		}
		claimed = l.itemInstances.ClaimRestoredItems(ownerID, ids)
	}

	kept := items[:0]
	var stale, unknown int
	for _, inst := range items {
		if inst == nil {
			continue
		}
		if st, ok := claimed[inst.ObjectID]; ok {
			inst = st.Instance()
		} else if l.itemInstances != nil && l.itemInstances.ContainsID(inst.ObjectID) {
			stale++
			continue
		}
		// A row whose item template is no longer loaded — what a datapack
		// downgrade leaves behind — is dropped from the restore and the
		// login carries on. Inventory.restore() does the same: the
		// ResultSet constructor dereferences the missing template,
		// restoreFromDb swallows that and returns null, and the restore
		// loop skips the row (Inventory.java:119-124,
		// ItemInstance.java:108-124 and 718-735). The row itself is left
		// alone, so the item returns when its template does.
		if l.itemTemplates != nil {
			if _, ok := l.itemTemplates.Get(inst.TemplateID); !ok {
				unknown++
				continue
			}
		}
		kept = append(kept, inst)
	}
	if stale > 0 {
		l.log.Warn().Int32("object_id", ownerID).Int("dropped", stale).Int("restored", len(kept)).
			Msg("restore inventory: skipped item rows with an unflushed change")
	}
	if len(claimed) > 0 {
		l.log.Warn().Int32("object_id", ownerID).Int("claimed", len(claimed)).Int("restored", len(kept)).
			Msg("restore inventory: restored item rows from an unflushed change")
	}
	if unknown > 0 {
		l.log.Error().Int32("object_id", ownerID).Int("dropped", unknown).Int("restored", len(kept)).
			Msg("restore inventory: skipped item rows with no loaded template")
	}
	return kept
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
	items = l.restoreItemRows(c.ID, items)
	if l.skills != nil {
		if err := l.skills.RestoreKnownSkills(ctx, c); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: restore known skills")
		}
		if err := l.skills.RestoreSkillState(ctx, c); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: restore skill state")
		}
		// Re-derive level-unlocked skills on every login (Player.java:4139
		// calls giveSkills() right after restoreCharData()), so a free grant
		// added by an in-session level-up — which lives in memory only —
		// comes back instead of vanishing on relog.
		if err := l.giveOrRewardSkills(c, tmpl); err != nil {
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
	if l.hennas != nil {
		rows, listErr := l.hennas.ListByOwner(ctx, c.ID)
		if listErr != nil {
			l.log.Error().Err(listErr).Msg("enter world: list hennas")
		} else {
			lookup := func(symbolID int) (henna.Henna, bool) {
				if l.hennaTable == nil {
					return henna.Henna{}, false
				}
				return l.hennaTable.Find(symbolID)
			}
			c.RestoreHennas(rows, lookup)
		}
	} else {
		c.RestoreHennas(nil, func(int) (henna.Henna, bool) { return henna.Henna{}, false })
	}

	live, err := l.attachLivePlayer(ctx, client, c, tmpl, items, shortcuts)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: attach live player")
		return nil, false
	}
	if l.roster != nil {
		// Mark the row online at login (the reference updates the online
		// status when a client enters the world), so external DB consumers
		// see online=1 without waiting for the first periodic save.
		if err := l.roster.SaveOnlineRecency(ctx, c); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: save player online recency")
		}
	}
	// From here on the player has a queue: the rest of the login runs on it,
	// and this goroutine waits, so the burst keeps its order.
	entered := false
	onLive(live, func() { entered = l.finishEnterWorld(client, c, live) })
	if !entered {
		// Hand the partially attached player back rather than nil: by now it
		// owns an actor queue, shadow-item tracking and, past world.Spawn,
		// world/clock/autosave registrations. The caller's deferred
		// detachLivePlayer is what releases them. Dropping live here would
		// leave the queue behind and let the shadow-item task keep decaying
		// — and finally destroy — an offline character's equipment.
		return live, false
	}
	return live, true
}

// finishEnterWorld publishes the attached player live into the world and
// sends the rest of the EnterWorld burst, on live's queue. It reports false
// when the login cannot complete.
func (l *GameClientLink) finishEnterWorld(client *Client, c *player.Character, live *livePlayer) bool {
	// Same constructor RequestItemList uses, so the login snapshot comes from
	// the live inventory instead of the raw restored rows. Built before the
	// burst starts and before anything is published into the world, so a
	// missing item template aborts with nothing sent and nothing spawned;
	// enterWorld's caller detaches what attachLivePlayer registered.
	itemListFrame, err := live.buildItemList(l.itemTemplates, false)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: build ItemList")
		return false
	}
	l.activateSpawnProtection(live)
	if l.skills != nil {
		if err := l.skills.RestoreEquippedItemStats(c, c.Inventory()); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("enter world: restore equipped item stats")
			return false
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

	client.Session.SendFrame(serverpackets.FrameSendMacroListEmpty())
	client.Session.SendFrame(serverpackets.FrameExStorageMaxCount(c))
	client.Session.SendFrame(serverpackets.FrameHennaInfo(c.HennaSnapshot()))
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
	if l.sevenSigns != nil {
		client.Session.SendFrame(serverpackets.FrameSystemMessage(sevenSignsPeriodMessage(l.sevenSigns.CurrentPeriod())))
	}
	if l.playerClock != nil && c.Race == player.RaceDarkElf {
		l.playerClock.NotifyShadowSenseState(live)
	}
	client.Session.SendFrame(serverpackets.FrameQuestList(nil))
	client.Session.SendFrame(serverpackets.FrameSkillList(skillList))
	client.Session.SendFrame(serverpackets.FrameFriendList(nil))
	client.Session.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	client.Session.SendFrame(itemListFrame)
	client.Session.SendFrame(serverpackets.FrameShortCutInit(serverShortcutList(live.shortcuts.All())))
	if c.Dead() {
		client.Session.SendFrame(serverpackets.FrameDie(c.ObjectID(), l.dieOptions(c)))
	}
	client.Session.SendFrame(serverpackets.FrameSkillCoolTime(coolTimes))
	client.Session.SendFrame(serverpackets.FrameActionFailed())
	return true
}

// sevenSignsPeriodMessage maps a Seven Signs period onto the system message
// announcing that it has begun.
func sevenSignsPeriodMessage(p sevensigns.Period) int {
	switch p {
	case sevensigns.Recruiting:
		return serverpackets.SystemMessagePreparationsPeriodBegun
	case sevensigns.Competition:
		return serverpackets.SystemMessageCompetitionPeriodBegun
	case sevensigns.Results:
		return serverpackets.SystemMessageResultsPeriodBegun
	default:
		return serverpackets.SystemMessageValidationPeriodBegun
	}
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
func (l *GameClientLink) giveOrRewardSkills(c *player.Character, tmpl *player.Template) error {
	refresh := l.skills.GiveSkills
	if l.playerConfig.AutoLearnSkills {
		refresh = l.skills.RewardSkills
	}
	return refresh(c, tmpl)
}

// refreshLiveLevelSkills re-derives the skills live's new level entitles it
// to and hands the client the resulting list. It runs on every level change,
// up or down, as the level refresher attachLivePlayer registers.
//
// The skill list goes out even when the refresh failed part-way: the
// character's in-memory skills have already moved, so the client's copy is
// stale either way, and resending is what makes the two agree again.
func (l *GameClientLink) refreshLiveLevelSkills(live *livePlayer) {
	if l.skills == nil || live == nil {
		return
	}
	rewarding := l.playerConfig.AutoLearnSkills
	before := live.SkillLevels()
	if err := l.giveOrRewardSkills(live.Character, live.template); err != nil {
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
				l.refreshSkillShortcuts(live, int32(id), int32(level))
			}
		}
	}
}

// userInfoSnapshot builds the UserInfo snapshot for live, deriving the
// spawn-protection team byte the client sees while protection holds.
func (l *GameClientLink) userInfoSnapshot(live *livePlayer) serverpackets.UserInfoSnapshot {
	return serverpackets.UserInfoSnapshot{
		Character:          live.Character,
		Template:           live.template,
		Items:              live.inventoryItems(),
		IsGM:               live.isGM,
		SpawnProtectedTeam: l.playerConfig.SpawnProtection > 0 && live.SpawnProtected(),
	}
}

// gameTime returns the game clock's current minute of day, 0 when no clock
// is wired (tests without the gameclock task).
func (l *GameClientLink) gameTime() int32 {
	if l.gameClock == nil {
		return 0
	}
	return int32(l.gameClock.TimeOfDay())
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

func setWaterSurface(mover *move.CreatureMove, zones *zone.Index) {
	if mover == nil || zones == nil {
		return
	}
	mover.SetWaterSurface(func(position location.Location, groundZ int) (int, bool) {
		water, ok := zone.FindAt[*zone.Water](zones, position.X, position.Y, position.Z)
		if !ok || groundZ-water.WaterLevel() >= -20 {
			return 0, false
		}
		return water.WaterLevel(), true
	})
}

func (l *GameClientLink) attachLivePlayer(ctx context.Context, client *Client, c *player.Character, tmpl *player.Template, items []*item.Instance, shortcuts []shortcut.Shortcut) (*livePlayer, error) {
	delivery := &playerInventoryDelivery{updates: l.inventoryUpdates, character: c}
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventoryWithDelivery(c.ID, l.itemTemplates, items, delivery, l.itemPersister(c.ID)))
	// The characters row stores finalized max snapshots (Save writes
	// ResourceValues), but the vitals fields are raw calculator bases once a
	// template is attached — re-seed them from the class tables so the CON/MEN
	// finalize applies exactly once per login instead of compounding across
	// save→load cycles. Current HP/MP/CP stay as restored from the row.
	c.RestoreVitals(tmpl)
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
	rt := player.Runtime{
		World:  l.world,
		Skills: l.skills,
		Levels: l.levels,
		Log:    l.log,
		Rules: player.Rules{
			RateKarmaExpLost:       l.playerConfig.RateKarmaExpLost,
			WeightLimitMultiplier:  l.playerConfig.WeightLimitMultiplier,
			PerfectShieldBlockRate: l.playerConfig.PerfectShieldBlockRate,
			MaxBuffsAmount:         l.playerConfig.MaxBuffsAmount,
			DeathPenaltyChance:     l.playerConfig.DeathPenaltyChance,
			AllowDelevel:           l.playerConfig.AllowDelevel,
			RaidCursesDisabled:     l.disableRaidCurse,
			AwardPKKillPVPPoint:    l.playerConfig.AwardPKKillPVPPoint,
		},
	}
	if los, ok := l.geo.(player.LineOfSight); ok {
		rt.LOS = los
	}
	if l.zones != nil {
		rt.Zones = l.zones
	}
	c.Configure(rt)
	c.RefreshWeightPenalty()
	c.RefreshExpertisePenalty()

	x, y, z := c.Position()
	creatureLive, err := creature.NewLive(location.Location{X: x, Y: y, Z: z}, c.RunSpeed(), l.geo, c, effect.WithActivityRegistry(l.effects))
	if err != nil {
		return nil, fmt.Errorf("attach live player: %w", err)
	}
	setWaterSurface(creatureLive.Move(), l.zones)
	if l.queues != nil {
		creatureLive.SetQueue(l.queues.NewQueue(fmt.Sprintf("player-%d", c.ObjectID())))
	}
	live := &livePlayer{Character: c, link: l, ctx: ctx, session: client.Session.SendFrame, template: tmpl, npcs: l.npcs, items: items, shortcuts: shortcut.NewList(shortcuts), isGM: resolveIsGM(l.admin, c.AccessLevel), visibilitySend: client.Session.SendFrame, stopAttack: l.stopLiveAutoAttack, log: l.log}
	delivery.live = live
	c.Attach(creatureLive, live)
	moveCtl, err := move.NewController(c.Move(), c, live)
	if err != nil {
		return nil, fmt.Errorf("attach live player: %w", err)
	}
	moveCtl.SetPositionUpdates(l.positions)
	attackCtl := attack.NewPlayer(c, live)
	c.Move().SetLogger(l.log)
	attackCtl.SetLogger(l.log)
	if q := creatureLive.Queue(); q != nil {
		attackCtl.SetQueue(q)
	}
	combat := ai.NewPlayerAttack(c, moveCtl, attackCtl)

	c.SetCanGiveDamage(resolveCanGiveDamage(l.admin, c.AccessLevel))
	live.attack, live.move, live.combat = attackCtl, moveCtl, combat
	live.kick = client.Session.Close
	live.zoneActor = &liveZoneActor{live: live}
	// Build cast eagerly, like attackCtl above: pickup-lock's timer goroutine
	// reads live.cast unguarded, so a lazy first write from the read-loop
	// goroutine would race it (issue #1183).
	l.castController(live)
	if inv := c.Inventory(); inv != nil {
		// Restored rows never queue update notifications, so totalWeight stays
		// 0 unless recomputed here, matching the reference's ItemList
		// constructor calling PcInventory.updateWeight() on every send
		// (including the one EnterWorld makes right after this).
		inv.UpdateWeight()
	}
	if inv := c.Inventory(); inv != nil && l.shadowItems != nil {
		for _, inst := range inv.PaperdollItems() {
			tmpl, ok := inv.Templates().Get(inst.TemplateID)
			if ok {
				l.shadowItems.Track(live.ObjectID(), inst, tmpl)
			}
		}
	}
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
