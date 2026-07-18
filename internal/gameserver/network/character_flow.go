package network

import (
	"context"
	"fmt"
	"sort"
	"time"

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
	}
	if c.ResourceValues().CurrentHP < 0.5 {
		c.MarkDead()
	}
	shortcuts := shortcut.Starter()
	if l.shortcuts != nil {
		shortcuts, err = l.shortcuts.ListByOwner(ctx, c.ID)
		if err != nil {
			l.log.Error().Err(err).Msg("enter world: list shortcuts")
			return nil, false
		}
	}

	itemListFrame, err := serverpackets.FrameItemList(items, l.itemTemplates, false)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: build ItemList")
		return nil, false
	}
	now := time.Now()
	coolTimes := skillCoolTimeEntries(c.SkillReuseTimers(now), now)
	skillList := skillListEntries(c, l.skills)

	live, err := l.attachLivePlayer(client, c, tmpl, items, shortcuts)
	if err != nil {
		l.log.Error().Err(err).Msg("enter world: attach live player")
		return nil, false
	}
	if l.world != nil {
		x, y, z := c.Position()
		l.world.Spawn(live, x, y, z, c.LastHeading)
		l.world.AddPlayer(live)
	}

	client.Session.SendFrame(serverpackets.FrameExStorageMaxCount(c))
	client.Session.SendFrame(serverpackets.FrameHennaInfo(c.ClassID))
	client.Session.SendFrame(serverpackets.FrameEtcStatusUpdate(serverpackets.EtcStatus{}))
	client.Session.SendFrame(serverpackets.FrameSystemMessage(serverpackets.SystemMessageWelcomeToLineage))
	client.Session.SendFrame(serverpackets.FrameQuestList(nil))
	client.Session.SendFrame(serverpackets.FrameSkillList(skillList))
	client.Session.SendFrame(serverpackets.FrameFriendList(nil))
	client.Session.SendFrame(serverpackets.FrameUserInfo(serverpackets.UserInfoSnapshot{Character: c, Template: tmpl, Items: items}))
	client.Session.SendFrame(itemListFrame)
	client.Session.SendFrame(serverpackets.FrameShortCutInit(serverShortcutList(shortcuts)))
	if c.Dead() {
		client.Session.SendFrame(serverpackets.FrameDie(c.ObjectID(), serverpackets.DieOptions{}))
	}
	client.Session.SendFrame(serverpackets.FrameSkillCoolTime(coolTimes))
	client.Session.SendFrame(serverpackets.FrameActionFailed())
	return live, true
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

func (l *GameClientLink) attachLivePlayer(client *Client, c *player.Character, tmpl *player.Template, items []*item.Instance, shortcuts []shortcut.Shortcut) (*livePlayer, error) {
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, l.itemTemplates, items))
	c.SetWorld(l.world)
	c.SetFrameSender(client.Session.SendFrame)

	x, y, z := c.Position()
	creatureLive, err := creature.NewLive(location.Location{X: x, Y: y, Z: z}, tmpl.RunSpeed, l.geo)
	if err != nil {
		return nil, fmt.Errorf("attach live player: %w", err)
	}
	c.Live = creatureLive
	moveCtl, err := move.NewController(c.Move(), c)
	if err != nil {
		return nil, fmt.Errorf("attach live player: %w", err)
	}
	moveCtl.SetPositionUpdates(l.positions)
	attackCtl := attack.NewPlayer(c)
	combat := ai.NewPlayerAttack(c, moveCtl, attackCtl)
	attackCtl.SetFinished(combat.Think)

	live := &livePlayer{Character: c, template: tmpl, items: items, attack: attackCtl, move: moveCtl, combat: combat, shortcuts: shortcut.NewList(shortcuts), stopAttack: l.stopLiveAutoAttack}
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
		combat.Think()
	})
	c.SetAttackBroadcaster(func(snapshot attack.Snapshot) {
		l.broadcastAttack(live, snapshot)
	})
	c.SetMoveBroadcaster(func(event move.Event) {
		l.broadcastLiveMoveEvent(live, event)
	})
	c.SetStopBroadcaster(func() {
		x, y, z := live.Position()
		l.broadcastLiveStopMove(live, location.Location{X: x, Y: y, Z: z}, live.CurrentHeading())
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
