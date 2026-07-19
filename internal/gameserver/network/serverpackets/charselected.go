package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// OpcodeCharSelected is the wire opcode for CharSelected, sent once a
// character slot is chosen and its state has moved to entering. It echoes
// the chosen character and carries the session id the client presents back
// once it sends EnterWorld.
const OpcodeCharSelected = 0x15

// CharSelectedSnapshot is everything CharSelected needs about the character
// entering the world and the session confirming it.
type CharSelectedSnapshot struct {
	Character *player.Character
	Template  *player.Template

	// SessionID is the session-key half the client must present back
	// unchanged once it sends EnterWorld.
	SessionID int32
}

// FrameCharSelected builds the CharSelected packet for s as an owned frame.
func FrameCharSelected(s CharSelectedSnapshot) wire.Frame {
	w := newFrameWriter(OpcodeCharSelected)
	writeCharSelected(w, s)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeCharSelected(w *wire.Writer, s CharSelectedSnapshot) {
	c, t := s.Character, s.Template
	x, y, z := c.Position()
	resources := c.ResourceValues()
	w.WriteString(c.Name)
	w.WriteInt32(c.ObjectID())
	w.WriteString(c.Title)
	w.WriteInt32(s.SessionID)
	w.WriteInt32(int32(c.ClanID))
	w.WriteInt32(0) // unknown

	w.WriteInt32(int32(c.Sex))
	w.WriteInt32(int32(c.Race))
	w.WriteInt32(int32(c.ClassID))

	w.WriteInt32(1)

	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteFloat64(resources.CurrentHP)
	w.WriteFloat64(resources.CurrentMP)
	w.WriteInt32(int32(c.SP))
	w.WriteInt64(c.Exp)
	w.WriteInt32(int32(c.CharLevel))
	w.WriteInt32(int32(c.Karma))
	w.WriteInt32(int32(c.PKKills))
	w.WriteInt32(int32(t.INT))
	w.WriteInt32(int32(t.STR))
	w.WriteInt32(int32(t.CON))
	w.WriteInt32(int32(t.MEN))
	w.WriteInt32(int32(t.DEX))
	w.WriteInt32(int32(t.WIT))

	for i := 0; i < 30; i++ {
		w.WriteInt32(0)
	}
	w.WriteInt32(0) // reserved
	w.WriteInt32(0) // reserved

	w.WriteInt32(0) // game time: day/night cycle is not modeled

	w.WriteInt32(0) // reserved

	w.WriteInt32(int32(c.ClassID))

	w.WriteInt32(0) // reserved
	w.WriteInt32(0) // reserved
	w.WriteInt32(0) // reserved
	w.WriteInt32(0) // reserved
}
