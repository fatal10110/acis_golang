package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodePetInfo is the wire opcode for the owner-only pet/servitor spawn and
// refresh window.
const OpcodePetInfo = 0xb1

// PetInfoSnapshot is everything PetInfo needs for one live pet or servitor,
// sent only to its owner (a non-owner observer gets SummonInfo instead,
// tracked separately).
type PetInfoSnapshot struct {
	SummonType        int
	ObjectID          int32
	TemplateID        int
	X, Y, Z, Heading  int
	MAtkSpd, PAtkSpd  int
	RunSpd, WalkSpd   int
	CollisionRadius   float64
	CollisionHeight   float64
	InCombat          bool
	AlikeDead         bool
	Name, Title       string
	PvpFlag           int
	Karma             int
	CurFed, MaxFed    int
	CurHP, MaxHP      int
	CurMP, MaxMP      int
	SP                int
	Level             int
	Exp               int64
	ExpForThisLevel   int64
	ExpForNextLevel   int64
	TotalWeight       int
	WeightLimit       int
	PAtk, PDef        int
	MAtk, MDef        int
	Accuracy          int
	EvasionRate       int
	CriticalHit       int
	MoveSpeed         int
	Mountable         bool
	Team              int
	SoulShotsPerHit   int
	SpiritShotsPerHit int
}

// FramePetInfo builds a PetInfo packet for s, sent to a pet/servitor's owner
// when it spawns or its owner-visible state needs a full refresh.
func FramePetInfo(s PetInfoSnapshot) wire.Frame {
	w := newFrameWriter(OpcodePetInfo)
	w.WriteInt32(int32(s.SummonType))
	w.WriteInt32(s.ObjectID)
	w.WriteInt32(int32(s.TemplateID + 1000000))
	w.WriteInt32(0) // 1=attackable; a summon is never itself attackable

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

	w.WriteFloat64(1) // movement speed multiplier: no summon-specific haste modeled yet, matches NpcInfo's own constant 1
	w.WriteFloat64(1) // attack speed multiplier: same as above
	w.WriteFloat64(s.CollisionRadius)
	w.WriteFloat64(s.CollisionHeight)
	w.WriteInt32(0) // weapon: Summon.getWeapon() is always 0 (base class, no override)
	w.WriteInt32(0) // armor: Summon.getArmor() is always 0 (base class, no override)
	w.WriteInt32(0)
	w.WriteUint8(1) // owner is always present: PetInfo is only ever sent to a live summon's own owner
	w.WriteUint8(1)
	w.WriteUint8(boolUint8(s.InCombat))
	w.WriteUint8(boolUint8(s.AlikeDead))
	// isShowSummonAnimation() is set true once at Summon construction and
	// never cleared anywhere in the reference, so this byte is always 2
	// regardless of the caller-supplied _val (PetInfo.java:73).
	w.WriteUint8(2)
	w.WriteString(s.Name)
	w.WriteString(s.Title)
	w.WriteInt32(1)
	w.WriteInt32(int32(s.PvpFlag))
	w.WriteInt32(int32(s.Karma))
	w.WriteInt32(int32(s.CurFed))
	w.WriteInt32(int32(s.MaxFed))
	w.WriteInt32(int32(s.CurHP))
	w.WriteInt32(int32(s.MaxHP))
	w.WriteInt32(int32(s.CurMP))
	w.WriteInt32(int32(s.MaxMP))
	w.WriteInt32(int32(s.SP))
	w.WriteInt32(int32(s.Level))
	w.WriteInt64(s.Exp)
	w.WriteInt64(s.ExpForThisLevel)
	w.WriteInt64(s.ExpForNextLevel)
	w.WriteInt32(int32(s.TotalWeight))
	w.WriteInt32(int32(s.WeightLimit))
	w.WriteInt32(int32(s.PAtk))
	w.WriteInt32(int32(s.PDef))
	w.WriteInt32(int32(s.MAtk))
	w.WriteInt32(int32(s.MDef))
	w.WriteInt32(int32(s.Accuracy))
	w.WriteInt32(int32(s.EvasionRate))
	w.WriteInt32(int32(s.CriticalHit))
	w.WriteInt32(int32(s.MoveSpeed))
	w.WriteInt32(int32(s.PAtkSpd))
	w.WriteInt32(int32(s.MAtkSpd))

	// Abnormal-effect icon bitmask: not computed yet, matching NpcInfo's
	// own unwired AbnormalEffect field (npcInfoSnapshot never sets it
	// either). The owner-invisible stealth-mask special case
	// (PetInfo.java:103) is out of scope until this is wired.
	w.WriteInt32(0)
	w.WriteUint16(uint16(boolInt32(s.Mountable)))
	w.WriteUint8(0) // move type: 0 is MoveType.GROUND, the default _moveTypes state (CreatureMove.java:76-84); no swim/fly state is modeled for summons yet

	w.WriteUint16(0)
	w.WriteUint8(uint8(s.Team)) // team/CTF system is not ported yet; always 0 (TeamType.NONE)
	w.WriteInt32(int32(s.SoulShotsPerHit))
	w.WriteInt32(int32(s.SpiritShotsPerHit))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
