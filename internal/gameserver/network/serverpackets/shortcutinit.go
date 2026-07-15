package serverpackets

import "github.com/fatal10110/acis_golang/internal/commons/wire"

const (
	// OpcodeShortCutRegister is the wire opcode for ShortCutRegister.
	OpcodeShortCutRegister = 0x44
	// OpcodeShortCutInit is the wire opcode for ShortCutInit.
	OpcodeShortCutInit = 0x45
	// OpcodeShortCutDelete is the wire opcode for ShortCutDelete.
	OpcodeShortCutDelete = 0x46
)

// ShortcutType is the client shortcut category ordinal.
type ShortcutType int32

const (
	ShortcutNone ShortcutType = iota
	ShortcutItem
	ShortcutSkill
	ShortcutAction
	ShortcutMacro
	ShortcutRecipe
)

// Shortcut is one client shortcut bar entry.
type Shortcut struct {
	Slot             int32
	Page             int32
	ID               int32
	Type             ShortcutType
	CharacterType    int32
	Level            int32
	SharedReuseGroup int32
	ReuseSeconds     int32
	RemainingSeconds int32
	AugmentationID   int32
}

// StarterShortcuts are the basic action shortcuts a new character starts
// with until shortcut persistence is ported.
func StarterShortcuts() []Shortcut {
	return []Shortcut{
		{Slot: 0, Page: 0, Type: ShortcutAction, ID: 2, Level: -1, CharacterType: 1, SharedReuseGroup: -1},
		{Slot: 3, Page: 0, Type: ShortcutAction, ID: 5, Level: -1, CharacterType: 1, SharedReuseGroup: -1},
		{Slot: 10, Page: 0, Type: ShortcutAction, ID: 0, Level: -1, CharacterType: 1, SharedReuseGroup: -1},
	}
}

// FrameShortCutInit builds the shortcut initialization packet.
func FrameShortCutInit(shortcuts []Shortcut) wire.Frame {
	w := newFrameWriter(OpcodeShortCutInit)
	w.WriteInt32(int32(len(shortcuts)))
	for _, shortcut := range shortcuts {
		writeShortcut(w, shortcut)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameShortCutRegister builds the single-shortcut registration packet.
func FrameShortCutRegister(shortcut Shortcut) wire.Frame {
	w := newFrameWriter(OpcodeShortCutRegister)
	writeShortcut(w, shortcut)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameShortCutDelete builds the shortcut deletion packet.
func FrameShortCutDelete(slot, page int32) wire.Frame {
	w := newFrameWriter(OpcodeShortCutDelete)
	w.WriteInt32(slot + page*12)
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeShortcut(w *wire.Writer, shortcut Shortcut) {
	w.WriteInt32(int32(shortcut.Type))
	w.WriteInt32(shortcut.Slot + shortcut.Page*12)
	switch shortcut.Type {
	case ShortcutItem:
		w.WriteInt32(shortcut.ID)
		w.WriteInt32(shortcut.CharacterType)
		w.WriteInt32(shortcut.SharedReuseGroup)
		w.WriteInt32(shortcut.RemainingSeconds)
		w.WriteInt32(shortcut.ReuseSeconds)
		w.WriteInt32(shortcut.AugmentationID)
	case ShortcutSkill:
		w.WriteInt32(shortcut.ID)
		w.WriteInt32(shortcut.Level)
		w.WriteUint8(0)
		w.WriteInt32(shortcut.CharacterType)
	default:
		w.WriteInt32(shortcut.ID)
		w.WriteInt32(shortcut.CharacterType)
	}
}
