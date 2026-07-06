package serverpackets

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"

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

// EncodeCharSelected builds the CharSelected packet for s. The day/night
// cycle is not modeled, so the game-time field always reports 0.
func EncodeCharSelected(s CharSelectedSnapshot) []byte {
	c, t := s.Character, s.Template

	w := newWriter(OpcodeCharSelected)
	w.WriteString(c.Name)
	w.WriteInt32(c.ObjectID)
	w.WriteString(c.Title)
	w.WriteInt32(s.SessionID)
	w.WriteInt32(int32(c.ClanID))
	w.WriteInt32(0) // unknown

	w.WriteInt32(int32(c.Sex))
	w.WriteInt32(int32(c.Race))
	w.WriteInt32(int32(c.ClassID))

	w.WriteInt32(1)

	w.WriteInt32(int32(c.Position.X))
	w.WriteInt32(int32(c.Position.Y))
	w.WriteInt32(int32(c.Position.Z))
	w.WriteFloat64(c.CurHP)
	w.WriteFloat64(c.CurMP)
	w.WriteInt32(int32(c.SP))
	w.WriteInt64(c.Exp)
	w.WriteInt32(int32(c.Level))
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

	return w.Bytes()
}
