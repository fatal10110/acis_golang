package event

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// NPCInfoChanged reports that an NPC's full client view is stale.
// ServerObject selects the static-object view an immobile NPC uses.
type NPCInfoChanged struct{ ServerObject bool }

// StatusKind names one value an NPC status update carries.
type StatusKind uint8

const (
	StatusCurrentHP StatusKind = iota
	StatusMaxHP
	StatusPhysicalSpeed
	StatusMagicSpeed
)

// StatusAttr is one changed status value.
type StatusAttr struct {
	Kind  StatusKind
	Value int
}

// Status reports changed status values observers must see.
type Status struct{ Attrs []StatusAttr }

// SkillLaunched reports a cast reaching its launch with the targets it hits.
type SkillLaunched struct {
	SkillID, Level int32
	TargetIDs      []int32
}

// SkillCanceled reports the cast-cancel animation for ObjectID.
type SkillCanceled struct{ ObjectID int32 }

// ShotRecharged reports an NPC recharging a shot, shown by the self-cast
// animation of SkillID at At to observers nearby.
type ShotRecharged struct {
	SkillID int32
	At      location.Location
}

// MoveTypeChanged reports a walk/run stance change.
type MoveTypeChanged struct{ Running bool }

// SocialAction reports a social animation.
type SocialAction struct{ ID int32 }

// NpcSay reports an NPC chat line.
type NpcSay struct {
	NpcID int
	Text  string
}

func (NPCInfoChanged) event()  {}
func (Status) event()          {}
func (SkillLaunched) event()   {}
func (SkillCanceled) event()   {}
func (ShotRecharged) event()   {}
func (MoveTypeChanged) event() {}
func (SocialAction) event()    {}
func (NpcSay) event()          {}
