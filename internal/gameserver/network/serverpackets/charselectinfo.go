package serverpackets

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// OpcodeCharSelectInfo is the wire opcode for CharSelectInfo, the character
// list shown at login and refreshed after every create/delete/restore.
const OpcodeCharSelectInfo = 0x13

// rhandPaperdollIndex is the equip-array position item.Slot.PaperdollIndex
// resolves a right-hand (or two-handed) weapon to.
const rhandPaperdollIndex = 7

// maxDisplayedEnchant is the highest enchant level the client's enchant-level
// byte field can carry: the field is a signed byte, so a value above this
// would wrap negative on the wire.
const maxDisplayedEnchant = 127

// paperdollWriteOrder is the equip-array position CharSelectInfo writes at
// each of its 17 paperdoll fields, in the client's expected order. Position
// 7 (the weapon hand) appears twice; that duplication is the client's own
// contract, not a mistake introduced here.
var paperdollWriteOrder = [...]int{16, 2, 1, 3, 5, 4, 6, 7, 8, 9, 10, 11, 12, 13, 7, 15, 14}

// CharacterSlot is one character-list entry: everything CharSelectInfo
// needs about one character, already resolved from the characters and
// items tables.
type CharacterSlot struct {
	Name     string
	ObjectID int32
	ClanID   int32

	Sex     player.Sex
	Race    player.Race
	ClassID int32

	X, Y, Z int32

	CurHP, CurMP float64
	MaxHP, MaxMP float64

	SP    int32
	Exp   int64
	Level int32

	Karma, PKKills, PvPKills int32

	HairStyle, HairColor, Face int32

	AccessLevel int32
	LastAccess  int64

	// DeleteTimerSeconds counts down to scheduled deletion (0 if none is
	// scheduled), or -1 for a banned character (AccessLevel < 0).
	DeleteTimerSeconds int32

	Paperdoll [item.PaperdollSlots]item.PaperdollEntry
}

// NewCharacterSlot builds a CharacterSlot from a persisted character and
// its items, resolving the deletion countdown display as of now.
func NewCharacterSlot(c *player.Character, items []*item.Instance, now time.Time) CharacterSlot {
	slot := CharacterSlot{
		Name: c.Name, ObjectID: c.ObjectID, ClanID: int32(c.ClanID),
		Sex: c.Sex, Race: c.Race, ClassID: int32(c.ClassID),
		X: int32(c.Position.X), Y: int32(c.Position.Y), Z: int32(c.Position.Z),
		CurHP: c.CurHP, CurMP: c.CurMP, MaxHP: c.MaxHP, MaxMP: c.MaxMP,
		SP: int32(c.SP), Exp: c.Exp, Level: int32(c.Level),
		Karma: int32(c.Karma), PKKills: int32(c.PKKills), PvPKills: int32(c.PvPKills),
		HairStyle: int32(c.HairStyle), HairColor: int32(c.HairColor), Face: int32(c.Face),
		AccessLevel: int32(c.AccessLevel),
		LastAccess:  c.LastAccess,
		Paperdoll:   item.Paperdoll(items),
	}

	switch {
	case c.AccessLevel < 0:
		slot.DeleteTimerSeconds = -1
	case c.DeleteAt > 0:
		remaining := (c.DeleteAt - now.UnixMilli()) / 1000
		if remaining < 0 {
			remaining = 0
		}
		slot.DeleteTimerSeconds = int32(remaining)
	}
	return slot
}

// EncodeCharSelectInfo builds the CharSelectInfo packet listing slots for
// loginName's session. activeID selects which slot the client highlights
// as the active one; -1 means "whichever was played most recently."
func EncodeCharSelectInfo(loginName string, sessionID int32, slots []CharacterSlot, activeID int32) []byte {
	w := newWriter(OpcodeCharSelectInfo)
	writeCharSelectInfo(w, loginName, sessionID, slots, activeID)
	return w.Bytes()
}

// FrameCharSelectInfo builds the CharSelectInfo packet as an owned frame.
func FrameCharSelectInfo(loginName string, sessionID int32, slots []CharacterSlot, activeID int32) wire.Frame {
	w := newFrameWriter(OpcodeCharSelectInfo)
	writeCharSelectInfo(w, loginName, sessionID, slots, activeID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeCharSelectInfo(w *wire.Writer, loginName string, sessionID int32, slots []CharacterSlot, activeID int32) {
	if activeID == -1 {
		var lastAccess int64
		for i, s := range slots {
			if s.LastAccess > lastAccess {
				lastAccess = s.LastAccess
				activeID = int32(i)
			}
		}
	}

	w.WriteInt32(int32(len(slots)))

	for i, s := range slots {
		w.WriteString(s.Name)
		w.WriteInt32(s.ObjectID)
		w.WriteString(loginName)
		w.WriteInt32(sessionID)
		w.WriteInt32(s.ClanID)
		w.WriteInt32(0) // builder level: GM builder mode is not modeled

		w.WriteInt32(int32(s.Sex))
		w.WriteInt32(int32(s.Race))
		w.WriteInt32(s.ClassID)

		w.WriteInt32(1)

		w.WriteInt32(s.X)
		w.WriteInt32(s.Y)
		w.WriteInt32(s.Z)

		w.WriteFloat64(s.CurHP)
		w.WriteFloat64(s.CurMP)

		w.WriteInt32(s.SP)
		w.WriteInt64(s.Exp)
		w.WriteInt32(s.Level)

		w.WriteInt32(s.Karma)
		w.WriteInt32(s.PKKills)
		w.WriteInt32(s.PvPKills)

		for j := 0; j < 7; j++ {
			w.WriteInt32(0)
		}

		for _, pos := range paperdollWriteOrder {
			w.WriteInt32(s.Paperdoll[pos].ObjectID)
		}
		for _, pos := range paperdollWriteOrder {
			w.WriteInt32(s.Paperdoll[pos].TemplateID)
		}

		w.WriteInt32(s.HairStyle)
		w.WriteInt32(s.HairColor)
		w.WriteInt32(s.Face)

		w.WriteFloat64(s.MaxHP)
		w.WriteFloat64(s.MaxMP)

		w.WriteInt32(s.DeleteTimerSeconds)
		w.WriteInt32(s.ClassID)
		w.WriteInt32(boolInt32(int32(i) == activeID))

		enchant := s.Paperdoll[rhandPaperdollIndex].EnchantLevel
		if enchant > maxDisplayedEnchant {
			enchant = maxDisplayedEnchant
		}
		w.WriteUint8(byte(enchant))
		w.WriteInt32(0) // augmentation id: item augmentation is not modeled
	}
}

func boolInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
