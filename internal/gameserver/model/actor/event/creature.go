package event

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// AttackHit is one target's result inside an Attack.
type AttackHit struct {
	TargetID int32
	Damage   int
	Flags    uint8
}

// Attack is one resolved physical attack swing, with every hit it landed.
type Attack struct {
	AttackerID int32
	X, Y, Z    int
	Hits       []AttackHit
}

// Move is a server-driven movement start: a straight move to Destination or,
// with FollowTarget set, a target-relative approach.
type Move struct {
	Origin, Destination location.Location
	Speed               float64
	Duration            time.Duration
	FollowTarget        int32
	FollowOffset        int
}

// Stopped reports server-driven movement cancelled mid-flight; the actor now
// stands at its current position.
type Stopped struct{}

// AutoAttackStopped reports that the actor's combat stance expired.
type AutoAttackStopped struct{}

// Died reports the moment the actor died.
type Died struct{}

// MagicSkillUse is a cast-start animation with explicit caster and target.
type MagicSkillUse struct {
	CasterID, TargetID  int32
	CasterAt, TargetAt  location.Location
	SkillID, Level      int32
	HitTime, ReuseDelay int
}

// AbnormalEffectChanged reports that the actor's visible abnormal-effect
// bitmask changed.
type AbnormalEffectChanged struct{}

// Flight is a forced-flight animation toward Dest that does not itself move
// the actor on the server.
type Flight struct {
	Dest   location.Location
	Flight modelskill.Flight
}

// MoveToPawn is a target-relative approach or a rotation-only turn toward
// TargetID from Origin, Distance away.
type MoveToPawn struct {
	TargetID int32
	Distance int
	Origin   location.Location
}

// StatusChanged reports that the actor's status observers see is stale.
type StatusChanged struct{}

// Despawned reports that the actor left the world for good.
type Despawned struct{}

func (MoveToPawn) event()            {}
func (StatusChanged) event()         {}
func (Despawned) event()             {}
func (Attack) event()                {}
func (Move) event()                  {}
func (Stopped) event()               {}
func (AutoAttackStopped) event()     {}
func (Died) event()                  {}
func (MagicSkillUse) event()         {}
func (AbnormalEffectChanged) event() {}
func (Flight) event()                {}
