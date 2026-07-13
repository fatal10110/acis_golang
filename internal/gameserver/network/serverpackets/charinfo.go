package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// OpcodeCharInfo is the wire opcode for a visible player other than self.
const OpcodeCharInfo = 0x03

var charInfoPaperdollOrder = [...]int{16, 6, 7, 8, 9, 10, 11, 12, 13, 7, 15, 14}

// CharInfoSnapshot is everything CharInfo needs for one visible player.
type CharInfoSnapshot struct {
	Character *player.Character
	Template  *player.Template
	Items     []*item.Instance
}

// FrameCharInfo builds a CharInfo packet for a visible player.
func FrameCharInfo(s CharInfoSnapshot) wire.Frame {
	w := newFrameWriter(OpcodeCharInfo)
	writeCharInfo(w, s)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeCharInfo(w *wire.Writer, s CharInfoSnapshot) {
	c, t := s.Character, s.Template
	x, y, z := c.Position()
	paperdoll := item.Paperdoll(s.Items)

	collisionRadius, collisionHeight := t.CollisionRadius, t.CollisionHeight
	if c.Sex == player.SexFemale {
		collisionRadius, collisionHeight = t.CollisionRadiusFemale, t.CollisionHeightFemale
	}

	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(int32(c.Heading))
	w.WriteInt32(0) // boat object id
	w.WriteInt32(c.ObjectID())
	w.WriteString(c.Name)
	w.WriteInt32(int32(c.Race))
	w.WriteInt32(int32(c.Sex))
	w.WriteInt32(int32(c.ClassID))

	for _, pos := range charInfoPaperdollOrder {
		w.WriteInt32(paperdoll[pos].TemplateID)
	}

	for i := 0; i < 4; i++ {
		w.WriteUint16(0)
	}
	w.WriteInt32(0) // right-hand augmentation id
	for i := 0; i < 12; i++ {
		w.WriteUint16(0)
	}
	w.WriteInt32(0) // left-hand augmentation id
	for i := 0; i < 4; i++ {
		w.WriteUint16(0)
	}

	w.WriteInt32(0) // pvp flag
	w.WriteInt32(int32(c.Karma))
	w.WriteInt32(0) // M.Atk speed: not modeled
	w.WriteInt32(int32(c.AttackSpeed()))
	w.WriteInt32(0) // pvp flag repeated
	w.WriteInt32(int32(c.Karma))

	runSpd := int32(t.RunSpeed)
	walkSpd := int32(t.WalkSpeed)
	swimSpd := int32(t.SwimSpeed)
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	w.WriteInt32(0) // flying run speed
	w.WriteInt32(0) // flying walk speed

	w.WriteFloat32(1)
	w.WriteFloat32(1)
	w.WriteFloat32(float32(collisionRadius))
	w.WriteFloat32(float32(collisionHeight))

	w.WriteInt32(int32(c.HairStyle))
	w.WriteInt32(int32(c.HairColor))
	w.WriteInt32(int32(c.Face))
	w.WriteString(c.Title)
	w.WriteInt32(int32(c.ClanID))
	w.WriteInt32(0) // clan crest id
	w.WriteInt32(0) // ally id
	w.WriteInt32(0) // ally crest id
	w.WriteInt32(0) // relation flags
	w.WriteUint8(1) // standing
	w.WriteUint8(1) // running
	w.WriteUint8(0) // in combat
	w.WriteUint8(boolUint8(c.AlikeDead()))
	w.WriteUint8(0)  // invisible
	w.WriteUint8(0)  // mount type
	w.WriteUint8(0)  // private store/craft mode
	w.WriteUint16(0) // cubic count
	w.WriteUint8(0)  // party match room
	w.WriteInt32(0)  // abnormal effect
	w.WriteUint8(0)  // recommendations left
	w.WriteUint16(0) // recommendations received
	w.WriteInt32(int32(c.ClassID))
	w.WriteInt32(int32(c.MaxCP))
	w.WriteInt32(int32(c.CurCP))
	w.WriteUint8(0) // enchant effect
	w.WriteUint8(0) // team
	w.WriteInt32(0) // large clan crest id
	w.WriteUint8(0) // noble
	w.WriteUint8(0) // hero
	w.WriteUint8(0) // fishing
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(defaultNameColor)
	w.WriteInt32(int32(c.Heading))
	w.WriteInt32(0) // pledge class
	w.WriteInt32(0) // pledge type
	w.WriteInt32(defaultTitleColor)
	w.WriteInt32(0) // cursed weapon stage
}

func boolUint8(v bool) uint8 {
	if v {
		return 1
	}
	return 0
}
