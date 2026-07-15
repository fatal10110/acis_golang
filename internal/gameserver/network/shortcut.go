package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func (l *GameClientLink) registerShortcut(ctx context.Context, live *livePlayer, req clientpackets.RequestShortCutReg) {
	if live == nil || req.Page < 0 || req.Page > 10 {
		return
	}
	typ := shortcut.Type(req.Type)
	if typ < shortcut.Item || typ > shortcut.Recipe {
		return
	}

	level := int32(-1)
	if typ == shortcut.Skill {
		level = int32(live.SkillLevel(int(req.ID)))
		if level <= 0 {
			return
		}
	}

	sc := shortcut.Shortcut{
		Slot:          req.Slot,
		Page:          req.Page,
		Type:          typ,
		ID:            req.ID,
		Level:         level,
		CharacterType: req.CharacterType,
	}
	if l.shortcuts != nil {
		if err := l.shortcuts.Save(ctx, live.ObjectID(), sc); err != nil {
			l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("register shortcut")
			return
		}
	}
	live.shortcuts.Register(sc)
	live.SendFrame(serverpackets.FrameShortCutRegister(serverShortcut(sc)))
}

func (l *GameClientLink) deleteShortcut(ctx context.Context, live *livePlayer, req clientpackets.RequestShortCutDel) {
	if live == nil || req.Page < 0 || req.Page > 9 {
		return
	}
	if !hasShortcutAt(live.shortcuts, req.Slot, req.Page) {
		return
	}
	if l.shortcuts != nil {
		if err := l.shortcuts.Delete(ctx, live.ObjectID(), req.Slot, req.Page); err != nil {
			l.log.Error().Err(err).Int32("object_id", live.ObjectID()).Msg("delete shortcut")
			return
		}
	}
	live.shortcuts.Delete(req.Slot, req.Page)
	live.SendFrame(serverpackets.FrameShortCutDelete(req.Slot, req.Page))
}

func hasShortcutAt(list *shortcut.List, slot, page int32) bool {
	for _, sc := range list.All() {
		if sc.Slot == slot && sc.Page == page {
			return true
		}
	}
	return false
}

func serverShortcutList(shortcuts []shortcut.Shortcut) []serverpackets.Shortcut {
	out := make([]serverpackets.Shortcut, 0, len(shortcuts))
	for _, sc := range shortcuts {
		out = append(out, serverShortcut(sc))
	}
	return out
}

func serverShortcut(sc shortcut.Shortcut) serverpackets.Shortcut {
	return serverpackets.Shortcut{
		Slot:             sc.Slot,
		Page:             sc.Page,
		ID:               sc.ID,
		Type:             serverShortcutType(sc.Type),
		CharacterType:    sc.CharacterType,
		Level:            sc.Level,
		SharedReuseGroup: -1,
	}
}

func serverShortcutType(typ shortcut.Type) serverpackets.ShortcutType {
	switch typ {
	case shortcut.Item:
		return serverpackets.ShortcutItem
	case shortcut.Skill:
		return serverpackets.ShortcutSkill
	case shortcut.Action:
		return serverpackets.ShortcutAction
	case shortcut.Macro:
		return serverpackets.ShortcutMacro
	case shortcut.Recipe:
		return serverpackets.ShortcutRecipe
	default:
		return serverpackets.ShortcutNone
	}
}
