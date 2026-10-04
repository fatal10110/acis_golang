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
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
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
	// closed ahead of the new player-auth request to the login server.
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
	now := time.Now() // character select runs on the connection goroutine, off any actor queue
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
// the item and the one the failed write was carrying.
//
// Claiming hands the row's write over without cancelling it: the entry stays
// pending, retargeted at the instance restored here, so the state is still
// scheduled if this login goes no further and the row still has exactly one
// writer. Only rows this restore actually keeps are offered for claiming
// (restoredItemLocation), so a row the inventory would discard is never handed
// a writer that discards it.
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
	return l.restoreRows(ownerID, items, restoredItemLocation)
}

// restoreRows is restoreItemRows for a container that rebuilds itself from
// the rows at the locations restored reports.
func (l *GameClientLink) restoreRows(ownerID int32, items []*item.Instance, restored func(item.Location) bool) []*item.Instance {
	var claimed map[int32]*item.Instance
	if l.itemInstances != nil {
		ids := make([]int32, 0, len(items))
		for _, inst := range items {
			if inst != nil && restored(inst.Location) {
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
		if restored, ok := claimed[inst.ObjectID]; ok {
			inst = restored
		} else if l.itemInstances != nil && l.itemInstances.ContainsID(inst.ObjectID) {
			stale++
			continue
		}
		// A row whose item template is no longer loaded — what a datapack
		// downgrade leaves behind — is dropped from the restore and the
		// login carries on: a row without its template cannot be rebuilt
		// into an item, so the restore loop skips it. The row itself is left
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

// restoredItemLocation reports whether a row at loc is one a login rebuilds
// a container from: the inventory's base and equip locations, the private
// warehouse and the player's own freight. The row query is not
// location-filtered, so a row at any other location reaches here too;
// claiming one would hand its write to an instance nothing restores,
// leaving the row with no writer at all.
func restoredItemLocation(loc item.Location) bool {
	switch loc {
	case item.LocationInventory, item.LocationPaperdoll, item.LocationWarehouse, item.LocationFreight:
		return true
	}
	return false
}

// findHenna resolves a saved henna row's symbol.
func (l *GameClientLink) findHenna(symbolID int) (henna.Henna, bool) {
	if l.hennaTable == nil {
		return henna.Henna{}, false
	}
	return l.hennaTable.Find(symbolID)
}

// restoreSelected restores the character c a selection picked: its items,
// skills, shortcuts, hennas, macros, recipes, recommendations and storage,
// attached as a live player with its own queue, and its row marked online.
// The player is neither spawned nor registered in the world here; the
// selection registers it and EnterWorld spawns it. Every failure returns
// nil: no live player is handed back, and none is registered.
func (l *GameClientLink) restoreSelected(ctx context.Context, client *Client, c *player.Character) (*livePlayer, bool) {
	tmpl, ok := l.templates.Get(c.ClassID())
	if !ok {
		l.log.Error().Int("class_id", c.ClassID()).Msg("select character: no template loaded")
		return nil, false
	}
	items, err := l.items.ListByOwner(ctx, c.ID)
	if err != nil {
		l.log.Error().Err(err).Msg("select character: list items")
		return nil, false
	}
	items = l.restoreItemRows(c.ID, items)
	if l.skills != nil {
		if err := l.skills.RestoreKnownSkills(ctx, c); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("select character: restore known skills")
		}
		// Re-derive level-unlocked skills on every login, right after the
		// character data is restored, so a free grant
		// added by an in-session level-up — which lives in memory only —
		// comes back instead of vanishing on relog.
		if err := l.giveOrRewardSkills(c, tmpl); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("select character: give skills")
			return nil, false
		}
		l.giveNobleSkills(c)
		if level := c.DeathPenaltyLevel(); level > 0 {
			if err := l.skills.ApplyTransientPassiveSkill(c, 5076, 0, level); err != nil {
				l.log.Error().Err(err).Int32("object_id", c.ID).Msg("select character: restore death-penalty passive stats")
				return nil, false
			}
		}
	}
	l.restoreHeroStatus(c)
	l.restoreCursedWeapon(c)
	if c.ResourceValues().CurrentHP < 0.5 {
		c.MarkDead()
	}
	shortcuts := shortcut.Starter()
	if l.shortcuts != nil {
		restored, listErr := l.shortcuts.ListByOwner(ctx, c.ID, c.ClassIndex())
		if listErr != nil {
			l.log.Error().Err(listErr).Msg("select character: list shortcuts")
			shortcuts = nil
		} else {
			shortcuts = restored
		}
	}
	if l.hennas != nil {
		rows, listErr := l.hennas.ListByOwner(ctx, c.ID, c.ClassIndex())
		if listErr != nil {
			l.log.Error().Err(listErr).Msg("select character: list hennas")
		} else {
			c.RestoreHennas(rows, l.findHenna)
		}
	} else {
		c.RestoreHennas(nil, func(int) (henna.Henna, bool) { return henna.Henna{}, false })
	}
	macros := l.restoreMacros(ctx, c.ID)
	l.restoreRecipeBook(ctx, c)
	l.restoreRecommended(ctx, c)

	// Split off before the inventory restore below takes its rows: the
	// warehouse and freight rows are the rest of the same set.
	storage := l.restoreStorage(ctx, client.AccountName(), c, items)
	live, err := l.attachLivePlayer(ctx, client, c, tmpl, items, shortcuts)
	if err != nil {
		l.log.Error().Err(err).Msg("select character: attach live player")
		return nil, false
	}
	live.storage = storage
	live.macros = macros
	if l.skills != nil {
		// Restored last, once nothing left in the selection can fail: the
		// restore consumes the saved rows, and only an attached player
		// saves them back when its session ends.
		if err := l.skills.RestoreSkillState(ctx, c); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("select character: restore skill state")
		}
	}
	if l.roster != nil {
		// The row is marked online at selection, so external DB consumers
		// see online=1 without waiting for the first periodic save.
		if err := l.roster.SaveOnlineRecency(ctx, c); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("select character: save player online recency")
		}
	}
	return live, true
}

// enterWorld spawns the selected, already registered live into the world
// and sends the EnterWorld packet burst. It reports false when the login
// cannot complete; live, attached at selection, stays the caller's to
// detach either way.
func (l *GameClientLink) enterWorld(client *Client, live *livePlayer) bool {
	// The player has a queue: the login runs on it, and this goroutine
	// waits, so the burst keeps its order.
	entered := false
	onLive(live, func() { entered = l.finishEnterWorld(client, live.Character, live) })
	return entered
}

// finishEnterWorld spawns the attached player live into the world and sends
// the EnterWorld burst, on live's queue. It reports false when the login
// cannot complete.
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
	now := c.Now()
	coolTimes := skillCoolTimeEntries(c.SkillReuseTimers(now), now)
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

	l.applyGMLoginModes(live)
	l.registerGM(live)
	for _, frame := range macroListFrames(live.macros) {
		client.Session.SendFrame(frame)
	}
	client.Session.SendFrame(serverpackets.FrameExStorageMaxCount(c))
	client.Session.SendFrame(serverpackets.FrameHennaInfo(c.HennaSnapshot()))
	// Replay restored buffs into the live effect list here, at the effect
	// icon refresh's place in the enter-world sequence: List.Add's
	// notifyAbnormalUpdate hook fires the resulting AbnormalStatusUpdate
	// frame (if any effect was restored) right where it belongs, ahead of
	// EtcStatusUpdate.
	live.replayingEffects.Store(true)
	if l.skills != nil {
		l.skills.ReplayEffects(c)
	}
	// Decide the restored load's penalty band only now that the replayed
	// effects and the equipped items have set the weight limit, and inside
	// the silent replay window: the EtcStatusUpdate below and every later
	// login frame carry the band, and sendLoginWeight reports the change
	// between UserInfo and ItemList.
	c.RefreshWeightPenalty()
	live.replayingEffects.Store(false)
	client.Session.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(c)))
	l.enterWorldClan(client, live)
	// Taken once the clan block has given the clan's skills, so the login
	// SkillList carries them.
	skillList := skillListEntries(c, l.skills)
	if l.world != nil {
		x, y, z := c.Position()
		l.world.Spawn(live, x, y, z, c.LastHeading)
		// Registered since its selection; from here on it is in the world
		// for the view refreshes its burst carried until now.
		live.entered.Store(true)
		if live.zoneActor != nil {
			live.zoneActor.revalidate(l.zones)
		}
	}
	client.Session.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWelcomeToLineage))
	if l.sevenSigns != nil {
		client.Session.SendFrame(serverpackets.FrameSystemMessage(sevenSignsPeriodMessage(l.sevenSigns.CurrentPeriod())))
	}
	l.sendLoginAnnouncements(live)
	if l.playerClock != nil && c.Race == player.RaceDarkElf {
		l.playerClock.NotifyShadowSenseState(live)
	}
	client.Session.SendFrame(serverpackets.FrameQuestList(nil))
	client.Session.SendFrame(serverpackets.FrameSkillList(skillList))
	client.Session.SendFrame(serverpackets.FrameFriendList(l.friendListEntries(c.ID)))
	client.Session.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
	l.sendLoginWeight(live)
	client.Session.SendFrame(itemListFrame)
	client.Session.SendFrame(serverpackets.FrameShortCutInit(serverShortcutList(live.Inventory(), live.shortcuts.All())))
	if c.Dead() {
		client.Session.SendFrame(serverpackets.FrameDie(c.ObjectID(), dieOptions(live)))
	}
	l.sendLoginBoardPages(client, live)
	// The chat of a petition still active is replayed after the login
	// board pages, before the friends hear of the entry.
	l.enterWorldPetition(client, live)
	// A cursed weapon's holder is announced as it enters.
	l.enterWorldCursedWeapon(live)
	// A punishment served resumes its timer, and a jailed player outside
	// the jail is taken back.
	l.enterWorldPunishment(live)
	// A game master hears which login modes it is in.
	sendGMLoginModes(live)
	// Friends hear of the entry last, just ahead of the reuse timers.
	l.notifyFriends(live, true)
	client.Session.SendFrame(serverpackets.FrameSkillCoolTime(coolTimes))
	// A character still serving a clan join penalty is reminded it was
	// expelled or left.
	if c.ClanJoinExpiryTime() > time.Now().UnixMilli() {
		client.Session.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageClanMembershipTerminated))
	}
	// ponytail: an attacker or spectator (siege state below 2) logging in
	// on a battlefield under siege is sent to town here; it needs the login
	// siege state, so both land together (#3150).
	client.Session.SendFrame(serverpackets.FrameActionFailed())
	return true
}

// sendLoginWeight reports the restored load between the login burst's
// UserInfo and its ItemList: StatusUpdate(CUR_LOAD) for any carried weight,
// then the penalty band's refresh (UserInfo, EtcStatusUpdate, CharInfo to
// every player that already sees live) when that load sits in a band.
// Both were decided silently earlier in the login, so every earlier login
// frame already carries them and this is the client's first report of the
// change.
func (l *GameClientLink) sendLoginWeight(live *livePlayer) {
	weight := live.CurrentWeight()
	if weight == 0 {
		return
	}
	live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{{
		Type: serverpackets.StatusCurrentLoad, Value: weight,
	}}))
	if live.WeightPenalty() != 0 {
		l.sendLiveWeightPenalty(live)
	}
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
// removal took. Setting SP sends StatusUpdate(SP) synchronously during the
// removal, before the removal's own system messages go out, so a combined
// removal orders StatusUpdate(SP) ahead of EXP_DECREASED_BY_S1.
func sendExpSpLossFrames(live *livePlayer, e event.ExpSPLost) {
	exp, sp := e.Exp, e.SP
	if sp > 0 {
		live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
			{Type: serverpackets.StatusSP, Value: e.SPLeft},
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
// StatusUpdate(KARMA), in that order.
func sendKarmaChangeFrames(live *livePlayer, karma int) {
	live.SendFrame(serverpackets.FrameSystemMessageNumber(serverpackets.SystemMessageYourKarmaHasBeenChangedToS1, int32(karma)))
	live.SendFrame(serverpackets.FrameStatusUpdate(live.ObjectID(), []serverpackets.StatusAttribute{
		{Type: serverpackets.StatusKarma, Value: karma},
	}))
}

// giveOrRewardSkills re-derives c's level-unlocked skills, calling
// RewardSkills instead of GiveSkills whenever the server grants every
// available skill automatically.
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
	before := live.SkillLevels()
	if err := l.giveOrRewardSkills(live.Character, live.Template()); err != nil {
		l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("level change: refresh level skills")
	}
	live.RefreshExpertisePenalty()

	// RewardSkills' grant loop refreshes shortcuts only for skills its own
	// filter (known level below the granted level) restricts to level
	// increases; GiveSkills never refreshes them, and RewardSkills' own
	// pull-back correction (correctInvalidSkills) does not either. So only
	// an actual level increase refreshes shortcuts — ahead of the skill
	// list, as the grant does — and a pull-back's level decrease must not.
	var refresh func(old, level int) bool
	if l.playerConfig.AutoLearnSkills {
		refresh = raisedSkill
	}
	l.sendSkillChanges(live, before, refresh)
}

// userInfoSnapshot builds the UserInfo snapshot for live, deriving the
// spawn-protection team byte the client sees while protection holds.
func (l *GameClientLink) userInfoSnapshot(live *livePlayer) serverpackets.UserInfoSnapshot {
	return serverpackets.UserInfoSnapshot{
		Character:          live.Character,
		Template:           live.Template(),
		Items:              live.inventoryItems(),
		IsGM:               live.accessLevel().IsGM,
		SpawnProtectedTeam: l.playerConfig.SpawnProtection > 0 && live.SpawnProtected(),
		Clan:               l.clanFields(live.Character),
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
	return skillListEntriesGreyed(c, skills, c.WearingFormalWear())
}

// skillListEntriesGreyed is skillListEntries with the formal wear flag
// given, for a SkillList sent while the paperdoll is still between two
// states.
func skillListEntriesGreyed(c *player.Character, skills *skillstate.Persistence, formalWear bool) []serverpackets.SkillListEntry {
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

	// Worn formal wear greys out every skill. The reference also greys a
	// clan skill while the clan's reputation is negative, but no member
	// holds its clan's skills at 0 reputation or below, so that never shows.
	disabled := formalWear
	entries := make([]serverpackets.SkillListEntry, 0, len(ids))
	for _, id := range ids {
		level := levels[id]
		if level <= 0 {
			continue
		}
		entry := serverpackets.SkillListEntry{ID: int32(id), Level: int32(level), Disabled: disabled}
		if skills != nil {
			if def, ok := skills.Definition(modelskill.Ref{ID: modelskill.ID(id), Level: level}); ok {
				entry.Passive = def.Activation == modelskill.ActivationPassive
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// The production movement geo answers the sight queries a player takes from
// it; a double without them leaves the player seeing everything.
var (
	_ player.LineOfSight         = move.EngineGeo{}
	_ player.LineOfSightIgnoring = move.EngineGeo{}
)

func setWaterSurface(mover *move.CreatureMove, zones *zone.Index) {
	if mover == nil || zones == nil {
		return
	}
	mover.SetWaterSurface(func(position location.Location) (int, bool) {
		water, ok := zone.FindAt[*zone.Water](zones, position.X, position.Y, position.Z)
		if !ok {
			return 0, false
		}
		return water.WaterLevel(), true
	})
}

func (l *GameClientLink) attachLivePlayer(ctx context.Context, client *Client, c *player.Character, tmpl *player.Template, items []*item.Instance, shortcuts []shortcut.Shortcut) (*livePlayer, error) {
	delivery := &playerInventoryDelivery{updates: l.inventoryUpdates, character: c}
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventoryWithDelivery(c.ID, l.itemTemplates, items, delivery, l.itemPersister(c.ID)))
	// The body stays the base class's while a subclass is played.
	if base, ok := l.templates.Get(c.BaseClassID()); ok {
		c.SetBaseTemplate(base)
	}
	// The characters row stores finalized max snapshots (Save writes
	// ResourceValues), but the vitals fields are raw calculator bases once a
	// template is attached — re-seed them from the class tables so the CON/MEN
	// finalize applies exactly once per login instead of compounding across
	// save→load cycles. Current HP/MP/CP stay as restored from the row.
	c.RestoreVitals(tmpl)
	// Filter/populate ITEM shortcuts against the live inventory just attached
	// above: a stale ITEM shortcut (its item consumed/traded/destroyed since last
	// logout) is dropped, and every surviving one gets SharedReuseGroup from
	// its item's etc-item data.
	shortcuts = l.restoreItemShortcuts(c, shortcuts)
	rt := player.Runtime{
		World:  l.world,
		Social: socialGraph{parties: l.parties, clans: l.clans},
		// HallFunctions gives the clan hall recovery bonuses.
		HallFunctions: l.hallFunctions,
		// PartyLoot hands a partied character's auto-loot and sweep to its
		// party's loot rule.
		PartyLoot: l,
		Skills:    l.skills,
		Levels:    l.levels,
		Log:       l.log,
		Rules: player.Rules{
			RateKarmaExpLost:       l.playerConfig.RateKarmaExpLost,
			RespawnRestoreHP:       l.playerConfig.RespawnRestoreHP,
			WeightLimitMultiplier:  l.playerConfig.WeightLimitMultiplier,
			InventorySlots:         l.playerConfig.InventorySlots,
			StorageSlots:           l.playerConfig.StorageSlots,
			PerfectShieldBlockRate: l.playerConfig.PerfectShieldBlockRate,
			MaxBuffsAmount:         l.playerConfig.MaxBuffsAmount,
			DeathPenaltyChance:     l.playerConfig.DeathPenaltyChance,
			AllowDelevel:           l.playerConfig.AllowDelevel,
			RaidCursesDisabled:     l.disableRaidCurse,
			AwardPKKillPVPPoint:    l.playerConfig.AwardPKKillPVPPoint,
			DeathDrop:              l.playerConfig.DeathDrop,
		},
	}
	if los, ok := l.geo.(player.LineOfSight); ok {
		rt.LOS = los
	}
	if l.zones != nil {
		rt.Zones = l.zones
	}
	if l.npcs != nil {
		rt.Mounts = l.npcs
		rt.MountData = mountDataTable{npcs: l.npcs}
	}
	c.Configure(rt)
	// Restored rows leave the carried weight at 0. Compute it here, while the
	// inventory has no live delivery target, so it stays silent: every login
	// frame carries the real load, and finishEnterWorld reports it between
	// UserInfo and ItemList. The penalty band waits for the restored stats
	// that move the weight limit (finishEnterWorld, after the effect replay).
	if inv := c.Inventory(); inv != nil {
		inv.UpdateWeight()
	}
	c.RefreshExpertisePenalty()

	x, y, z := c.Position()
	creatureLive, err := creature.NewLive(location.Location{X: x, Y: y, Z: z}, c.MoveSpeed(), l.geo, c, effect.WithEnv(l.effects))
	if err != nil {
		return nil, fmt.Errorf("attach live player: %w", err)
	}
	setWaterSurface(creatureLive.Move(), l.zones)
	// A player swims while its zones hold it in water, not wherever the
	// water query finds it.
	creatureLive.Move().UseZoneSwim()
	creatureLive.SetQueue(l.queues.NewQueue(fmt.Sprintf("player-%d", c.ObjectID())))
	access := l.admin.Resolve(c.AccessLevel)
	live := &livePlayer{Character: c, link: l, ctx: ctx, session: client.Session.SendFrame, npcs: l.npcs, items: items, shortcuts: shortcut.NewList(shortcuts), visibilitySend: client.Session.SendFrame, log: l.log}
	live.access.Store(&access)
	live.remoteIP = client.Session.remoteIP()
	delivery.live = live
	c.Attach(creatureLive, live)
	moveCtl, err := move.NewController(c.Move(), c, live)
	if err != nil {
		return nil, fmt.Errorf("attach live player: %w", err)
	}
	moveCtl.SetPositionUpdates(l.positions)
	attackCtl := attack.NewPlayer(c, live)
	attackCtl.SetQueue(creatureLive.Queue())
	combat := ai.NewPlayerAttack(c, moveCtl, attackCtl)

	c.SetCanGiveDamage(access.GiveDamage)
	c.SetSeesInvisible(access.IsGM)
	live.attack, live.move, live.combat = attackCtl, moveCtl, combat
	live.kick = client.Session.Close
	live.zoneActor = &liveZoneActor{live: live}
	// Build cast eagerly, like attackCtl above: pickup-lock's timer goroutine
	// reads live.cast unguarded, so a lazy first write from the read-loop
	// goroutine would race it (issue #1183).
	l.castController(live)
	// The intention source reads cast, combat and move, all built above.
	c.SetIntentionSource(live)
	return live, nil
}

// takeOverSelected finishes a selection once its player live is registered
// in the world, on live's queue: a pet its character left behind is its pet
// again, and its equipped shadow items start decaying. Both wait for the
// registration, since each reaches live by its object id from then on: a
// shadow item running dry has its expiry carried out on the registered
// player, and a corpse decaying meanwhile hands its items to live's
// inventory. It reports false when the work did not complete.
func (l *GameClientLink) takeOverSelected(live *livePlayer) bool {
	return onLive(live, func() {
		l.reclaimPetCorpse(live)
		l.trackShadowItems(live)
	})
}

// trackShadowItems starts the mana decay of every shadow item live wears.
func (l *GameClientLink) trackShadowItems(live *livePlayer) {
	inv := live.Inventory()
	if inv == nil || l.shadowItems == nil {
		return
	}
	for _, inst := range inv.PaperdollItems() {
		if tmpl, ok := inv.Templates().Get(inst.TemplateID); ok {
			l.shadowItems.Track(live.ObjectID(), inst, tmpl)
		}
	}
}

// restoreItemShortcuts keeps the ITEM shortcuts of shortcuts whose item c
// still holds, each with its item's shared reuse group.
func (l *GameClientLink) restoreItemShortcuts(c *player.Character, shortcuts []shortcut.Shortcut) []shortcut.Shortcut {
	return shortcut.RestoreItemShortcuts(shortcuts, func(objectID int32) (int32, bool) {
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
