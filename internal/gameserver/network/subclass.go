package network

import (
	"context"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
)

// subclassMinLevel is the level the active class and every subclass held
// must have reached before another subclass may be added.
const subclassMinLevel = 75

// classChangeTimeout bounds the persistence work of one class change.
const classChangeTimeout = 5 * time.Second

// Plain chat lines of the subclass dialog.
const (
	noSubclassesText     = "There are no sub classes available at this time."
	subclassRevertedText = "The sub class could not be added, you have been reverted to your base class."
)

// classChangeKind is what a class change does before it switches class.
type classChangeKind int

const (
	// classChangeSwitch switches to a class already held.
	classChangeSwitch classChangeKind = iota
	// classChangeAdd adds a subclass in a new slot, then switches to it.
	classChangeAdd
	// classChangeReplace replaces the subclass of a slot, then switches to
	// it.
	classChangeReplace
)

// classChange is one class change between its request, on the player's
// queue, and the switch, back on that queue once its rows are written and
// the new class's read.
type classChange struct {
	kind  classChangeKind
	index int
	// sub is the subclass an add or a replace puts in slot index.
	sub    player.SubClass
	folk   *npc.Folk
	done   chan classChangeRows
	queued bool
}

// classChangeRows is what the persistence lane hands back: whether an add
// or a replace wrote its subclass, and the rows of the class switched to.
type classChangeRows struct {
	// aborted is set when the persistence job ended without returning,
	// having panicked: what it wrote is unknown, so the change is undone
	// as one that never ran.
	aborted   bool
	written   bool
	skills    skillstate.ClassSkills
	shortcuts []shortcut.Shortcut
	hennas    []henna.Row
	recipes   []int
}

// subclassBypass runs a village master's Subclass command for live: the
// subclass menus, and the add, change and replace actions. It reports false
// when it has started a class change, whose end sends the closing
// ActionFailed once the new class is in place.
func (l *GameClientLink) subclassBypass(live *livePlayer, f *npc.Folk, command string) bool {
	// A summon cast still resolving its pet holds the casting state.
	if live.CastingNow() || live.AllSkillsDisabled() || l.restoringSummon(live) {
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSubclassNoChangeWhileSkillInUse))
		return true
	}
	// A subclass command makes a registered noble ineligible to compete.
	l.leaveOlympiadOnSubclass(live)
	cmd := npc.ParseSubclassCommand(command)
	subs := live.Subclasses()
	base := live.BaseClassID()
	var page string
	switch cmd.Choice {
	case 0:
		page = f.SubclassPage(setPages{l.html}, "SubClass", "")
	case 1:
		if !l.subclassMenuAllowed(live) {
			return true
		}
		if len(subs) >= player.MaxSubclasses {
			page = f.SubclassPage(setPages{l.html}, "SubClass_Fail", "")
			break
		}
		avail := f.AvailableSubclasses(base, subs)
		if len(avail) == 0 {
			live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, noSubclassesText))
			return true
		}
		page = f.SubclassPage(setPages{l.html}, "SubClass_Add", npc.SubclassAddList(avail))
	case 2:
		if !l.subclassMenuAllowed(live) {
			return true
		}
		if len(subs) == 0 {
			page = f.SubclassPage(setPages{l.html}, "SubClass_ChangeNo", "")
			break
		}
		if list := f.SubclassChangeList(base, subs); list != "" {
			page = f.SubclassPage(setPages{l.html}, "SubClass_Change", list)
		} else {
			page = f.SubclassPage(setPages{l.html}, "SubClass_ChangeNotFound", "")
		}
	case 3:
		if len(subs) == 0 {
			page = f.SubclassPage(setPages{l.html}, "SubClass_ModifyEmpty", "")
			break
		}
		page = f.SubclassModifyPage(setPages{l.html}, subs)
	case 4:
		if !l.subclassActionAllowed(live) {
			return true
		}
		if !l.mayAddSubclass(live, subs) || !f.ValidNewSubclass(base, subs, cmd.One) {
			page = f.SubclassPage(setPages{l.html}, "SubClass_Fail", "")
			break
		}
		return l.beginClassChange(live, &classChange{kind: classChangeAdd, index: len(subs) + 1, sub: player.NewSubClass(cmd.One, len(subs)+1, l.levels), folk: f})
	case 5:
		if !l.subclassActionAllowed(live) {
			return true
		}
		if live.ClassIndex() == cmd.One {
			page = f.SubclassPage(setPages{l.html}, "SubClass_Current", "")
			break
		}
		classID := base
		if cmd.One != 0 {
			sub, ok := live.Subclass(cmd.One)
			if !ok {
				return true
			}
			classID = sub.ClassID
		}
		if !f.Teaches(classID) {
			return true
		}
		live.EffectList().StopAll()
		l.broadcastCharacterInfo(live)
		return l.beginClassChange(live, &classChange{kind: classChangeSwitch, index: cmd.One, folk: f})
	case 6:
		if cmd.One < 1 || cmd.One > player.MaxSubclasses {
			return true
		}
		avail := f.AvailableSubclasses(base, subs)
		if len(avail) == 0 {
			live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, noSubclassesText))
			return true
		}
		page = f.SubclassPage(setPages{l.html}, "SubClass_ModifyChoice"+string(rune('0'+cmd.One)), npc.SubclassReplaceList(cmd.One, avail))
	case 7:
		if !l.subclassActionAllowed(live) {
			return true
		}
		// The slot comes from a replace menu link, which names 1 to 3.
		if cmd.One < 1 || cmd.One > player.MaxSubclasses || !f.ValidNewSubclass(base, subs, cmd.Two) {
			return true
		}
		return l.beginClassChange(live, &classChange{kind: classChangeReplace, index: cmd.One, sub: player.NewSubClass(cmd.Two, cmd.One, l.levels), folk: f})
	default:
		// Only choices 0 to 7 are ever offered, and a command must be one
		// of the links last sent.
		return true
	}
	sendFilledHTML(live, f.ObjectID(), page, 0)
	return true
}

// subclassMenuAllowed applies the add and change menus' gates: no summon
// out and not overweight, each refused with its message.
func (l *GameClientLink) subclassMenuAllowed(live *livePlayer) bool {
	switch {
	case l.hasActiveSummon(live):
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageCantSubclassWithSummonedServitor))
		return false
	case live.Overweight():
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageNotSubclassWhileOverweight))
		return false
	}
	return true
}

// subclassActionAllowed is the reuse gate of the add, change and replace
// actions: one per SubclassDelay. A refused action answers nothing of its
// own.
func (l *GameClientLink) subclassActionAllowed(live *livePlayer) bool {
	delay := l.playerConfig.SubclassDelay
	if delay <= 0 {
		return true
	}
	now := time.Now() // a reuse gate on wall time, as the other client gates are
	if live.subclassReuseUntil.After(now) {
		return false
	}
	live.subclassReuseUntil = now.Add(delay)
	return true
}

// mayAddSubclass reports whether live may add a subclass: fewer than the
// most it may hold, the active class and every subclass at least level 75,
// and the quests the subclass needs done.
func (l *GameClientLink) mayAddSubclass(live *livePlayer, subs []player.SubClass) bool {
	if len(subs) >= player.MaxSubclasses || live.Level() < subclassMinLevel {
		return false
	}
	for _, sub := range subs {
		if sub.Level < subclassMinLevel {
			return false
		}
	}
	// A noble, or a character who has completed Fate's Whisper and Mimir's
	// Elixir, may add one without SubClassWithoutQuests. Neither gate is
	// wired here yet: noble status exists (player.Character.IsNoble) but
	// checking it is #3070's to wire, and quest states do not exist yet,
	// so none passes that way.
	return l.playerConfig.SubclassWithoutQuests
}

// beginClassChange starts ch on live's queue: it takes the class-change
// lock, puts an added or replacing subclass in its slot, copies the state
// the change saves, and queues the subclass rows' writes, that save and the
// new class's reads on live's persistence lane. The connection then waits
// for them (finishPendingClassChange) before the switch runs on live's
// queue again. A lock already held, or a class with no template, ends the
// request as the reference's refused change does.
func (l *GameClientLink) beginClassChange(live *livePlayer, ch *classChange) bool {
	classID := ch.sub.ClassID
	if ch.kind == classChangeSwitch {
		classID = live.BaseClassID()
		if ch.index != 0 {
			sub, _ := live.Subclass(ch.index)
			classID = sub.ClassID
		}
	}
	tmpl, ok := l.templates.Get(classID)
	if !ok {
		l.log.Error().Int32("object_id", live.ObjectID()).Int("class_id", classID).Msg("class change: no template loaded")
		return true
	}
	if !live.TryLockClassChange() {
		l.refusedClassChange(live, ch)
		return true
	}
	switch ch.kind {
	case classChangeAdd:
		if !live.AddSubclass(ch.sub) {
			live.UnlockClassChange()
			return true
		}
	case classChangeReplace:
		live.RemoveSubclass(ch.index)
		live.AddSubclass(ch.sub)
		if ch.index == live.ClassIndex() {
			// The slot played is replaced in place: from here on it plays
			// as the fresh subclass.
			live.ReplaceActiveProgression(ch.sub)
		}
	}
	// The change saves the class left, with its effects and reuse timers,
	// before the class switched to is read.
	l.abortFusionTargeting(live)
	charState := live.SaveState()
	var skillState skillstate.SaveState
	if l.skills != nil {
		skillState = l.skills.SaveState(live.Character)
	}
	ch.done = make(chan classChangeRows, 1)
	ch.queued = l.persist.Enqueue(live.ObjectID(), func() {
		// The lane recovers a panicking job, so the answer is sent from a
		// defer: a job that never returns its rows still ends the change,
		// as aborted, and the connection waiting on done goes on.
		rows := classChangeRows{aborted: true}
		defer func() { ch.done <- rows }()
		rows = l.writeClassChange(live.ObjectID(), ch, tmpl, charState, skillState)
	})
	live.pendingClassChange = ch
	return false
}

// refusedClassChange answers a change whose lock is already held, as the
// reference answers its refused add, switch and replace.
func (l *GameClientLink) refusedClassChange(live *livePlayer, ch *classChange) {
	switch ch.kind {
	case classChangeSwitch:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSubclassTransferCompleted))
	case classChangeReplace:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, subclassRevertedText))
	}
}

// writeClassChange runs ch's persistence on charID's lane: an added
// subclass's row and starting skills, or a replaced slot's rows cleared and
// the new subclass's written; then the save of the class left; then the
// reads of the class switched to. A replace that fails reads the base
// class instead, which the change then falls back to.
func (l *GameClientLink) writeClassChange(charID int32, ch *classChange, tmpl *player.Template, charState player.SaveState, skillState skillstate.SaveState) classChangeRows {
	ctx, cancel := context.WithTimeout(context.Background(), classChangeTimeout)
	defer cancel()
	rows := classChangeRows{written: true}
	if ch.kind != classChangeSwitch && l.subclasses != nil {
		var err error
		if ch.kind == classChangeReplace {
			err = l.subclasses.Delete(ctx, charID, ch.index)
		}
		if err == nil {
			err = l.subclasses.Insert(ctx, charID, ch.sub)
		}
		if err != nil {
			l.log.Error().Err(err).Int32("object_id", charID).Int("class_index", ch.index).Msg("class change: write subclass")
			rows.written = false
		} else if l.skills != nil {
			if err := l.skills.StoreClassSkills(ctx, charID, int32(ch.index), tmpl.StartingSubclassSkills()); err != nil {
				l.log.Error().Err(err).Int32("object_id", charID).Int("class_index", ch.index).Msg("class change: store starting skills")
			}
		}
	}
	if !rows.written {
		if ch.kind == classChangeAdd {
			return rows
		}
		// The slot is gone: its subclass row must not be written back.
		kept := charState.Subclasses[:0]
		for _, sub := range charState.Subclasses {
			if sub.Index != ch.index {
				kept = append(kept, sub)
			}
		}
		charState.Subclasses = kept
	}
	if l.roster != nil {
		if err := l.roster.Save(ctx, charState); err != nil {
			l.log.Error().Err(err).Int32("object_id", charID).Msg("class change: save player")
		}
	}
	if l.skills != nil {
		if err := l.skills.Save(ctx, skillState); err != nil {
			l.log.Error().Err(err).Int32("object_id", charID).Msg("class change: save skill state")
		}
	}
	index := ch.index
	if !rows.written {
		index = 0
	}
	l.readClassRows(ctx, charID, index, &rows)
	return rows
}

// readClassRows reads into rows what class index keeps: its learned skills
// and saved skill state, its shortcuts and hennas, and, for the base class,
// the recipe book. A failed read leaves that part empty; it is logged.
func (l *GameClientLink) readClassRows(ctx context.Context, charID int32, index int, rows *classChangeRows) {
	var err error
	if l.skills != nil {
		if rows.skills, err = l.skills.LoadClassSkills(ctx, charID, int32(index)); err != nil {
			l.log.Error().Err(err).Int32("object_id", charID).Msg("class change: load skills")
		}
	}
	if l.shortcuts != nil {
		if rows.shortcuts, err = l.shortcuts.ListByOwner(ctx, charID, index); err != nil {
			l.log.Error().Err(err).Int32("object_id", charID).Msg("class change: list shortcuts")
		}
	}
	if l.hennas != nil {
		if rows.hennas, err = l.hennas.ListByOwner(ctx, charID, index); err != nil {
			l.log.Error().Err(err).Int32("object_id", charID).Msg("class change: list hennas")
		}
	}
	if index == 0 && l.recipeBooks != nil {
		if rows.recipes, err = l.recipeBooks.ListByOwner(ctx, charID); err != nil {
			l.log.Error().Err(err).Int32("object_id", charID).Msg("class change: list recipe book")
		}
	}
}

// finishPendingClassChange waits, on the connection's goroutine, for the
// persistence of a class change the last request started, then completes it
// on live's queue. The connection reads nothing more from the client
// meanwhile, so the change ends before the player's next request runs.
func (l *GameClientLink) finishPendingClassChange(live *livePlayer) {
	if live == nil {
		return
	}
	ch := live.pendingClassChange
	if ch == nil {
		return
	}
	live.pendingClassChange = nil
	var rows classChangeRows
	if ch.queued {
		rows = <-ch.done
	}
	onLive(live, func() { l.completeClassChange(live, ch, rows) })
}

// completeClassChange switches live to ch's class with the rows read for
// it, releases the class-change lock and answers the request: a subclass
// added or replaced is announced and the master's confirmation shown, a
// switch is announced alone. An add whose subclass could not be written
// takes it back and changes nothing; a replace that could not be written
// falls back to the base class. A change whose persistence never ran, or
// aborted, takes back the slot it filled and releases the lock alone.
func (l *GameClientLink) completeClassChange(live *livePlayer, ch *classChange, rows classChangeRows) {
	defer live.SendFrame(serverpackets.FrameActionFailed())
	if !ch.queued || rows.aborted {
		if ch.kind != classChangeSwitch {
			live.RemoveSubclass(ch.index)
		}
		live.UnlockClassChange()
		return
	}
	index := ch.index
	if !rows.written {
		live.RemoveSubclass(ch.index)
		if ch.kind == classChangeAdd {
			live.UnlockClassChange()
			return
		}
		index = 0
	}
	l.switchClass(live, index, rows)
	live.UnlockClassChange()
	switch {
	case !rows.written:
		live.SendFrame(serverpackets.FrameSystemMessageString(serverpackets.SystemMessageS1, subclassRevertedText))
	case ch.kind == classChangeSwitch:
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageSubclassTransferCompleted))
	default:
		name := "SubClass_AddOk"
		if ch.kind == classChangeReplace {
			name = "SubClass_ModifyOk"
		}
		live.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageAddNewSubclass))
		sendFilledHTML(live, ch.folk.ObjectID(), ch.folk.SubclassPage(setPages{l.html}, name, ""), 0)
	}
}

// switchClass makes class index live's active class, with rows its saved
// state: the worn items' bonuses and the skills of the class left go, the
// effects that do not outlast death end, a servitor and the cubics leave,
// and the new class's skills, hennas, shortcuts, effects and reuse timers
// come in its place, the recipe book with the base class alone; the party's
// level follows the new class's level and the clan's skills come back. The
// client is shown the result in the order the reference sends it.
func (l *GameClientLink) switchClass(live *livePlayer, index int, rows classChangeRows) {
	c := live.Character
	classID := c.BaseClassID()
	if index != 0 {
		sub, _ := c.Subclass(index)
		classID = sub.ClassID
	}
	tmpl, ok := l.templates.Get(classID)
	if !ok {
		l.log.Error().Int32("object_id", c.ID).Int("class_id", classID).Msg("class change: no template loaded")
		return
	}
	inv := c.Inventory()
	// A cast begun while the change's rows were written ends here, before
	// the skills it casts from go.
	c.StopCast()
	l.unequipItemStats(live)
	c.ClearSkillReuses()
	c.ClearCharges()
	c.SwitchClass(index, tmpl)
	c.RestoreVitals(tmpl)
	// The party's level follows the level of the class switched to.
	if l.parties != nil {
		l.parties.RecalculateLevel(live.ObjectID())
	}
	l.unsummonServitor(live)
	if l.skills != nil {
		l.skills.RemoveAllSkills(c)
	}
	c.EffectList().StopAllExceptThoseThatLastThroughDeath()
	l.broadcastCharacterInfo(live)
	l.stopAllCubics(live)
	if c.SubclassActive() {
		c.RecipeBook().Clear()
	} else {
		l.putRecipes(c, rows.recipes)
	}
	c.RestoreHennas(rows.hennas, l.findHenna)
	if l.skills != nil {
		if err := l.skills.ApplyClassSkills(c, rows.skills); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("class change: restore skills")
		}
		if err := l.giveOrRewardSkills(c, tmpl); err != nil {
			l.log.Error().Err(err).Int32("object_id", c.ID).Msg("class change: give skills")
		}
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(c, l.skills)))
		// The skills held outside the class come back after that list,
		// which does not show them: a noble's skills, with its skill list
		// and UserInfo; a hero's skills on the base class, which the
		// status takes away on a subclass, with its skill list; then the
		// clan's skills, and a clan leader's siege skills after them once
		// sieges exist (#3150).
		if c.IsNoble() {
			l.giveNobleSkills(c)
			live.SendFrame(serverpackets.FrameSkillList(skillListEntries(c, l.skills)))
			live.SendFrame(serverpackets.FrameUserInfo(l.userInfoSnapshot(live)))
		}
		if c.IsHero() {
			l.setHero(live, true)
		}
		if cl, ok := l.clanService().ClanOf(c); ok {
			l.giveClanSkills(live, cl, c.PledgeClass())
		}
		// The death penalty's passive stats are not a learned skill's:
		// they stay through the change, as the reference gives them back.
		l.equipItemStats(live, inv)
	}
	c.ClearDisabledSkills()
	if l.skills != nil {
		l.skills.StageClassSkillState(c, rows.skills)
		l.skills.ReplayEffects(c)
	}
	if c.EffectList().HasHeld() {
		l.updateEffectIcons(live)
	}
	live.SendFrame(serverpackets.FrameEtcStatusUpdate(etcStatus(c)))
	// Repent Your Sins ends here once quests exist (#3070).
	c.ClampResources()
	c.RefreshWeightPenalty()
	c.RefreshExpertisePenalty()
	c.RefreshHennaStats()
	live.SendFrame(serverpackets.FrameHennaInfo(c.HennaSnapshot()))
	l.broadcastCharacterInfo(live)
	for _, shotID := range c.AutoSoulShotIDs() {
		live.SendFrame(serverpackets.FrameExAutoSoulShot(shotID, false))
		live.SendFrame(serverpackets.FrameSystemMessageItemName(serverpackets.SystemMessageAutoUseOfItemCancelled, shotID))
		c.SetAutoSoulShot(shotID, false)
	}
	for _, kind := range []item.ShotKind{item.ShotSoul, item.ShotSpirit, item.ShotBlessedSpirit} {
		c.SetChargedShot(kind, false)
	}
	live.shortcuts.Replace(l.restoreItemShortcuts(c, rows.shortcuts))
	live.SendFrame(serverpackets.FrameShortCutInit(serverShortcutList(inv, live.shortcuts.All())))
	l.broadcastLiveFrame(live, func() wire.Frame {
		return serverpackets.FrameSocialAction(live.ObjectID(), socialActionLevelUp)
	})
	now := c.Now()
	live.SendFrame(serverpackets.FrameSkillCoolTime(skillCoolTimeEntries(c.SkillReuseTimers(now), now)))
}

// unequipItemStats takes every worn item's stats, skills and augmentation
// bonus off live, as a class change does before it saves.
func (l *GameClientLink) unequipItemStats(live *livePlayer) {
	inv := live.Inventory()
	if l.skills == nil || inv == nil {
		return
	}
	for _, inst := range inv.PaperdollItems() {
		if tmpl, ok := inv.Templates().Get(inst.TemplateID); ok {
			l.skills.UnequipItemStats(live.Character, inv, inst, tmpl)
		}
	}
}

// equipItemStats puts every worn item's stats, skills and augmentation
// bonus back on live for its new class, sending the skill list again when
// they brought skills.
func (l *GameClientLink) equipItemStats(live *livePlayer, inv *itemcontainer.Inventory) {
	if inv == nil {
		return
	}
	changed := false
	for _, inst := range inv.PaperdollItems() {
		tmpl, ok := inv.Templates().Get(inst.TemplateID)
		if !ok {
			continue
		}
		skills, _, err := l.skills.EquipItemStats(live.Character, inst, tmpl)
		if err != nil {
			l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("class change: equip item stats")
		}
		changed = changed || skills
	}
	if changed {
		live.SendFrame(serverpackets.FrameSkillList(skillListEntries(live.Character, l.skills)))
	}
}

// unsummonServitor sends live's servitor away; a pet stays.
func (l *GameClientLink) unsummonServitor(live *livePlayer) {
	if l.world == nil {
		return
	}
	obj, ok := l.world.Summon(live.ObjectID())
	if !ok {
		return
	}
	if s, ok := obj.(*summon.Actor); ok && !s.IsPet() {
		s.Unsummon()
	}
}

// stopAllCubics ends every cubic of live, telling its observers once.
func (l *GameClientLink) stopAllCubics(live *livePlayer) {
	if live.removeAllCubics() {
		l.broadcastCharacterInfo(live)
	}
}
