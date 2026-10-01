package network

import (
	"context"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// macroStore reads and writes the character_macroses rows of one player.
type macroStore interface {
	ListByOwner(ctx context.Context, ownerID int32) ([]macro.Row, error)
	Save(ctx context.Context, ownerID int32, m macro.Macro) error
	Delete(ctx context.Context, ownerID, id int32) error
}

// Macro edit refusal messages.
const (
	systemMessageMacroLimit           = 797 // You may create up to 24 macros.
	systemMessageInvalidMacro         = 810 // Invalid macro. Refer to the Help file for instructions.
	systemMessageMacroDescriptionMax  = 837 // Macro descriptions may contain up to 32 characters.
	systemMessageEnterMacroName       = 838 // Enter the name of the macro.
	systemMessageMacroNameAlreadyUsed = 839 // That name is already assigned to another macro.
)

// restoreMacros loads the player's macros for world entry. A failed read
// leaves the list empty; a malformed row keeps the macros read before it.
// Either way the failure is logged and the login goes on.
func (l *GameClientLink) restoreMacros(ctx context.Context, ownerID int32) *macro.List {
	if l.macros == nil {
		return macro.NewList()
	}
	rows, err := l.macros.ListByOwner(ctx, ownerID)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", ownerID).Msg("enter world: list macros")
		return macro.NewList()
	}
	list, err := macro.Restore(rows)
	if err != nil {
		l.log.Error().Err(err).Int32("object_id", ownerID).Msg("enter world: restore macros")
	}
	return list
}

// macroListFrames advances list's revision and builds the macro window's
// refresh: one SendMacroList per macro, or a single empty one.
func macroListFrames(list *macro.List) []wire.Frame {
	revision := list.NextRevision()
	all := list.All()
	if len(all) == 0 {
		return []wire.Frame{serverpackets.FrameSendMacroList(revision, 0, nil)}
	}
	frames := make([]wire.Frame, len(all))
	for i := range all {
		frames[i] = serverpackets.FrameSendMacroList(revision, len(all), &all[i])
	}
	return frames
}

func (l *GameClientLink) sendMacroList(live *livePlayer) {
	for _, frame := range macroListFrames(live.macros) {
		live.SendFrame(frame)
	}
}

// makeMacro answers RequestMakeMacro: a refused edit gets its system
// message; an accepted one is stored, written on the persistence lane, and
// answered with the refreshed macro list.
func (l *GameClientLink) makeMacro(live *livePlayer, req clientpackets.RequestMakeMacro) {
	if live == nil || live.macros == nil {
		return
	}
	m := macro.Macro{
		ID:          req.ID,
		Icon:        req.Icon,
		Name:        req.Name,
		Description: req.Description,
		Acronym:     req.Acronym,
		Commands:    make([]macro.Command, len(req.Commands)),
	}
	commandText := 0
	for i, c := range req.Commands {
		m.Commands[i] = macro.Command{Type: c.Type, D1: c.D1, D2: c.D2, Text: c.Text}
		commandText += macro.UTF16Len(c.Text)
	}
	if msg, refused := macroRefusalMessage(live.macros.Check(m, commandText)); refused {
		live.SendFrame(serverpackets.FrameSystemMessage(msg))
		return
	}
	m = live.macros.Register(m)
	if l.macros != nil {
		l.queueRowWrite(live.ObjectID(), "save macro", func(ctx context.Context, ownerID int32) error {
			return l.macros.Save(ctx, ownerID, m)
		})
	}
	l.sendMacroList(live)
}

func macroRefusalMessage(r macro.Refusal) (int, bool) {
	switch r {
	case macro.CommandsTooLong:
		return systemMessageInvalidMacro, true
	case macro.TooMany:
		return systemMessageMacroLimit, true
	case macro.NameMissing:
		return systemMessageEnterMacroName, true
	case macro.NameTaken:
		return systemMessageMacroNameAlreadyUsed, true
	case macro.DescriptionTooLong:
		return systemMessageMacroDescriptionMax, true
	}
	return 0, false
}

// deleteMacro answers RequestDeleteMacro: the macro's row and every
// shortcut bound to it go, then the refreshed macro list is sent. Deleting
// an id the list does not hold answers nothing, as the reference does: the
// macro window registers no pending action and the list it shows is
// unchanged.
func (l *GameClientLink) deleteMacro(live *livePlayer, req clientpackets.RequestDeleteMacro) {
	if live == nil || !live.macros.Delete(req.ID) {
		return
	}
	if l.macros != nil {
		id := req.ID
		l.queueRowWrite(live.ObjectID(), "delete macro", func(ctx context.Context, ownerID int32) error {
			return l.macros.Delete(ctx, ownerID, id)
		})
	}
	l.deleteTargetShortcuts(live, shortcut.Macro, req.ID)
	l.sendMacroList(live)
}
