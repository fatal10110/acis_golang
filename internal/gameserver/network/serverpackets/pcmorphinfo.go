package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// polymorphTitle is the title a polymorphed character reads on itself.
const polymorphTitle = "Morphed"

// FramePcMorphInfo builds the NPCInfo other players get for c shown in
// form: the NPC id, held items and body are the form's, the speeds, stance
// and visual effects c's own. It always reads as attackable, names no name,
// title or clan, and shows no summon animation.
func FramePcMorphInfo(c *player.Character, form player.NPCForm) wire.Frame {
	w := newFrameWriter(OpcodeNPCInfo)
	x, y, z := c.Position()
	runSpd, walkSpd, swimSpd := int32(c.BaseRunSpeed()), int32(c.BaseWalkSpeed()), int32(c.BaseSwimSpeed())
	w.WriteInt32(c.ObjectID())
	w.WriteInt32(form.NPCID + 1000000)
	w.WriteInt32(1)
	w.WriteInt32(int32(x))
	w.WriteInt32(int32(y))
	w.WriteInt32(int32(z))
	w.WriteInt32(int32(c.CurrentHeading()))
	w.WriteInt32(0)
	w.WriteInt32(int32(c.MagicAttackSpeed()))
	w.WriteInt32(int32(c.AttackSpeed()))
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(swimSpd)
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	w.WriteInt32(runSpd)
	w.WriteInt32(walkSpd)
	w.WriteFloat64(float64(c.MovementSpeedMultiplier()))
	w.WriteFloat64(float64(c.AttackSpeedMultiplier()))
	w.WriteFloat64(form.CollisionRadius)
	w.WriteFloat64(form.CollisionHeight)
	w.WriteInt32(form.RightHand)
	w.WriteInt32(0)
	w.WriteInt32(form.LeftHand)
	w.WriteUint8(1)
	w.WriteUint8(boolUint8(c.Running()))
	w.WriteUint8(boolUint8(c.InCombat()))
	w.WriteUint8(boolUint8(c.AlikeDead()))
	w.WriteUint8(0)
	w.WriteString("")
	w.WriteString("")
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(int32(c.AbnormalEffect()))
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(uint8(c.MoveType()))
	w.WriteUint8(0)
	w.WriteFloat64(form.CollisionRadius)
	w.WriteFloat64(form.CollisionHeight)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
