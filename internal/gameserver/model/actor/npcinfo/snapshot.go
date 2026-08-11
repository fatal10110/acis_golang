// Package npcinfo defines the model-owned state serialized in NPCInfo.
package npcinfo

// Snapshot is everything an NPCInfo packet needs for one visible NPC.
type Snapshot struct {
	ObjectID                     int32
	TemplateID                   int
	Attackable                   bool
	X, Y, Z                      int
	Heading                      int
	MAtkSpd, PAtkSpd             int
	RunSpd, WalkSpd              int
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
