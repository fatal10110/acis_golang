// Package npcinfo defines the model-owned state serialized in NPCInfo.
package npcinfo

// Snapshot is everything an NPCInfo packet needs for one visible NPC.
// MoveMultiplier is the current move speed over the base speed the stance
// picks; the client scales RunSpd/WalkSpd by it. AtkSpdMultiplier is
// the value AttackSpeedMultiplier returns for PAtkSpd; the client scales its
// attack animation by it.
type Snapshot struct {
	ObjectID                     int32
	TemplateID                   int
	Attackable                   bool
	X, Y, Z                      int
	Heading                      int
	MAtkSpd, PAtkSpd             int
	RunSpd, WalkSpd              int
	MoveMultiplier               float64
	AtkSpdMultiplier             float64
	CurrentHP, MaxHP             int
	CollisionRadius              float64
	CollisionHeight              float64
	RightHand, Chest, LeftHand   int
	Running, InCombat, AlikeDead bool
	SummonAnimation              int
	Summon                       bool
	PvpFlag, Karma               int
	AbnormalEffect               int
	ClanID, ClanCrest            int
	AllyID, AllyCrest            int
	MoveType, Team               int
	EnchantEffect                int
	Flying                       bool
	Name, Title                  string
}

// AttackSpeedMultiplier is 1.1 times the live P.Atk. speed pAtkSpd over
// the template's base P.Atk. speed, at float32 precision, or 0 for a
// template whose base is 0.
func AttackSpeedMultiplier(pAtkSpd int, base float64) float64 {
	if base == 0 {
		return 0
	}
	return float64(float32(1.1 * float64(pAtkSpd) / base))
}

// StatusType is a client-visible StatusUpdate attribute identifier.
type StatusType int32

const (
	StatusCurrentHP     StatusType = 9
	StatusMaxHP         StatusType = 10
	StatusPhysicalSpeed StatusType = 18
	StatusMagicSpeed    StatusType = 24
)

// StatusAttribute is one changed value in an NPC StatusUpdate.
type StatusAttribute struct {
	Type  StatusType
	Value int
}
