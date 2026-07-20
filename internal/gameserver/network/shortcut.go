package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// registerShortcut mirrors the reference behavior for shortcut registration:
// it accepts only well-formed items, actions, macros, recipes, and learned
// skills, and silently rejects everything else (bad page range, unknown type,
// or a skill-shortcut for a skill the player doesn't have). Shortcuts are a
// client-side UI convenience with no server-authoritative action lock — a
// rejected registration can't freeze client input the way the silent-drop bug
// class behind #829 freezes it, and the reference handler itself stays silent
// on every rejection path — so this is left intentionally silent instead of
// patched with ActionFailed the way the action-locked handlers in #873 were.
func (l *GameClientLink) registerShortcut(ctx context.Context, live *livePlayer, req clientpackets.RequestShortCutReg) {
	if live == nil {
		return
	}
	sc, ok := shortcut.NewRegistration(req.Slot, req.Page, shortcut.Type(req.Type), req.ID, req.CharacterType, func(id int32) int {
		return live.SkillLevel(int(id))
	})
	if !ok {
		return
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

// deleteShortcut mirrors the reference behavior: a delete on a page outside
// the valid range, or for a slot the player has nothing in, returns nothing.
// Same reasoning as registerShortcut above — silent rejection is intentional
// Java parity for a UI packet that doesn't lock client input.
func (l *GameClientLink) deleteShortcut(ctx context.Context, live *livePlayer, req clientpackets.RequestShortCutDel) {
	if live == nil || !shortcut.ValidDeletePage(req.Page) {
		return
	}
	if !live.shortcuts.Has(req.Slot, req.Page) {
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
