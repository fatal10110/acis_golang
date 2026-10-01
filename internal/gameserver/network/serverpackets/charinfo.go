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
	// Clan is Character's clan as others see it; Leader and Privileges are
	// not shown.
	Clan ClanFields
}

// FrameCharInfo builds a CharInfo packet for a visible player.
func FrameCharInfo(s CharInfoSnapshot) wire.Frame {
	w := newFrameWriter(OpcodeCharInfo)
	if err := writeCharInfo(w, s); err != nil {
		releaseFrameWriter(w)
		return wire.InvalidFrame(err)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeCharInfo(w *wire.Writer, s CharInfoSnapshot) error {
	c, t := s.Character, s.Template
	x, y, z := c.Position()
	resources := c.ResourceValues()
	paperdoll := item.Paperdoll(s.Items)

	collisionRadius, collisionHeight := t.CollisionRadius, t.CollisionHeight
	if c.Sex == player.SexFemale {
		collisionRadius, collisionHeight = t.CollisionRadiusFemale, t.CollisionHeightFemale
	}

	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(0) // boat object id
	w.WriteInt32(c.ObjectID())
	w.WriteString(c.Name)
	w.WriteInt32(int32(c.Race))
	w.WriteInt32(int32(c.Sex))
	w.WriteInt32(int32(c.VisibleBaseClassID()))

	for _, pos := range charInfoPaperdollOrder {
		w.WriteInt32(paperdoll[pos].TemplateID)
	}

	for i := 0; i < 4; i++ {
		w.WriteUint16(0)
	}
	w.WriteInt32(paperdoll[rhandPaperdollIndex].AugmentationID)
	for i := 0; i < 12; i++ {
		w.WriteUint16(0)
	}
	w.WriteInt32(paperdoll[lhandPaperdollIndex].AugmentationID)
	for i := 0; i < 4; i++ {
		w.WriteUint16(0)
	}

	w.WriteInt32(int32(c.PvPFlagState()))
	w.WriteInt32(int32(c.Karma()))
	w.WriteInt32(int32(c.MagicAttackSpeed()))
	w.WriteInt32(int32(c.AttackSpeed()))
	w.WriteInt32(int32(c.PvPFlagState()))
	w.WriteInt32(int32(c.Karma()))

	runSpd := int32(c.BaseRunSpeed())
	walkSpd := int32(c.BaseWalkSpeed())
	swimSpd := int32(c.BaseSwimSpeed())
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	if c.Flying() {
		w.WriteInt32(runSpd)
		w.WriteInt32(walkSpd)
	} else {
		w.WriteInt32(0)
		w.WriteInt32(0)
	}

	w.WriteFloat64(float64(c.MovementSpeedMultiplier()))
	w.WriteFloat64(float64(c.AttackSpeedMultiplier()))
	w.WriteFloat64(collisionRadius)
	w.WriteFloat64(collisionHeight)

	w.WriteInt32(int32(c.HairStyle))
	w.WriteInt32(int32(c.HairColor))
	w.WriteInt32(int32(c.Face))
	w.WriteString(c.Title())
	w.WriteInt32(c.ClanID())
	w.WriteInt32(s.Clan.CrestID)
	w.WriteInt32(s.Clan.AllyID)
	w.WriteInt32(s.Clan.AllyCrestID)
	w.WriteInt32(0) // relation flags
	w.WriteUint8(boolUint8(c.Standing()))
	w.WriteUint8(boolUint8(c.Running()))
	w.WriteUint8(boolUint8(c.InCombat()))
	w.WriteUint8(boolUint8(c.AlikeDead()))
	w.WriteUint8(0) // invisible
	w.WriteUint8(uint8(c.MountType()))
	w.WriteUint8(uint8(c.OperateType()))
	cubicIDs := c.CubicIDs()
	count, err := wire.Uint16Count(len(cubicIDs))
	if err != nil {
		return err
	}
	w.WriteUint16(count)
	for _, id := range cubicIDs {
		w.WriteUint16(uint16(id))
	}
	w.WriteUint8(boolUint8(c.PartyRoom() > 0))
	w.WriteInt32(int32(c.AbnormalEffect()))
	w.WriteUint8(uint8(c.RecommendationsLeft()))
	w.WriteUint16(uint16(c.RecommendationsHave()))
	w.WriteInt32(int32(c.ClassID()))
	w.WriteInt32(int32(resources.MaxCP))
	w.WriteInt32(int32(resources.CurrentCP))
	w.WriteUint8(0) // enchant effect
	w.WriteUint8(0) // team
	w.WriteInt32(s.Clan.CrestLargeID)
	w.WriteUint8(0) // noble
	w.WriteUint8(0) // hero
	w.WriteUint8(0) // fishing
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(c.NameColor())
	w.WriteInt32(int32(c.CurrentHeading()))
	w.WriteInt32(int32(c.PledgeClass()))
	w.WriteInt32(s.Clan.PledgeType)
	w.WriteInt32(c.TitleColor())
	w.WriteInt32(0) // cursed weapon stage
	return nil
}

func boolUint8(v bool) uint8 {
	return wire.BoolByte(v)
}
