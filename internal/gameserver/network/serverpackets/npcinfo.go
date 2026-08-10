package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
)

// OpcodeNPCInfo is the wire opcode for a visible NPC entering sight.
const OpcodeNPCInfo = 0x16

// NPCInfoSnapshot is everything NPCInfo needs for one visible NPC.
type NPCInfoSnapshot = npcinfo.Snapshot

// FrameNPCInfo builds the NPC info packet sent when a live NPC enters a
// player's visible region.
func FrameNPCInfo(s NPCInfoSnapshot) wire.Frame {
	w := newFrameWriter(OpcodeNPCInfo)
	w.WriteInt32(s.ObjectID)
	w.WriteInt32(int32(s.TemplateID + 1000000))
	w.WriteInt32(boolInt32(s.Attackable))
	w.WriteInt32(int32(s.X))
	w.WriteInt32(int32(s.Y))
	w.WriteInt32(int32(s.Z))
	w.WriteInt32(int32(s.Heading))
	w.WriteInt32(0)
	w.WriteInt32(int32(s.MAtkSpd))
	w.WriteInt32(int32(s.PAtkSpd))
	w.WriteInt32(int32(s.RunSpd))
	w.WriteInt32(int32(s.WalkSpd))
	w.WriteInt32(int32(s.RunSpd))
	w.WriteInt32(int32(s.WalkSpd))
	w.WriteInt32(int32(s.RunSpd))
	w.WriteInt32(int32(s.WalkSpd))
	w.WriteInt32(int32(s.RunSpd))
	w.WriteInt32(int32(s.WalkSpd))
	w.WriteFloat64(1)
	w.WriteFloat64(1)
	w.WriteFloat64(s.CollisionRadius)
	w.WriteFloat64(s.CollisionHeight)
	w.WriteInt32(int32(s.RightHand))
	w.WriteInt32(int32(s.Chest))
	w.WriteInt32(int32(s.LeftHand))
	w.WriteUint8(1)
	w.WriteUint8(boolUint8(s.Running))
	w.WriteUint8(boolUint8(s.InCombat))
	w.WriteUint8(boolUint8(s.AlikeDead))
	w.WriteUint8(uint8(s.SummonAnimation))
	w.WriteString(s.Name)
	w.WriteString(s.Title)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(int32(s.AbnormalEffect))
	w.WriteInt32(int32(s.ClanID))
	w.WriteInt32(int32(s.ClanCrest))
	w.WriteInt32(int32(s.AllyID))
	w.WriteInt32(int32(s.AllyCrest))
	w.WriteUint8(uint8(s.MoveType))
	w.WriteUint8(uint8(s.Team))
	w.WriteFloat64(s.CollisionRadius)
	w.WriteFloat64(s.CollisionHeight)
	w.WriteInt32(int32(s.EnchantEffect))
	w.WriteInt32(boolInt32(s.Flying))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
