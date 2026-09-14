package event

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// VitalsChanged reports a change to the actor's current HP (and, with
// IncludeMP, its MP) that observers must see.
type VitalsChanged struct{ IncludeMP bool }

// BowDrawn reports that a bow shot started drawing; GaugeMs covers the attack
// time plus reuse.
type BowDrawn struct{ GaugeMs int }

// Stance is a character's client-visible posture.
type Stance int

const (
	StanceSitting Stance = iota
	StanceStanding
	StanceFakeDeathStart
	StanceFakeDeathStop
)

// StanceChanged reports a posture change.
type StanceChanged struct{ Stance Stance }

// FakeDeathRevived reports a character standing up out of fake death.
type FakeDeathRevived struct{}

// EffectIconsChanged reports that the character's active-effect icon list
// changed.
type EffectIconsChanged struct{}

// PositionCorrected reports a forced-location correction after a flight
// landed.
type PositionCorrected struct{}

// WeightPenaltyChanged reports a change of the carried-weight penalty band.
type WeightPenaltyChanged struct{}

// UserInfoChanged reports that the character's own full self-view is stale.
type UserInfoChanged struct{}

// GradePenaltyChanged reports a change of the equipment grade penalty.
type GradePenaltyChanged struct{}

// DeathPenaltyChanged reports a death-penalty level change from Old to New.
// Raised distinguishes a death raising it from a recovery lowering it.
type DeathPenaltyChanged struct {
	Old, New int
	Raised   bool
}

// ExpSPGained reports one experience/SP addition that was not fully rejected.
type ExpSPGained struct {
	Exp int64
	SP  int
}

// ExpSPLost reports one experience/SP removal.
type ExpSPLost struct {
	Exp int64
	SP  int
}

// KarmaChanged reports the character's new karma total.
type KarmaChanged struct{ Karma int }

// LevelChanged reports any level change, up or down; everything the level
// entitles the character to must be re-derived.
type LevelChanged struct{}

// LeveledUp reports that the character's level went up.
type LeveledUp struct{}

// ShortBuff reports the item-window short-buff countdown state; a zero value
// clears it.
type ShortBuff struct {
	SkillID         int32
	Level           int32
	DurationSeconds int32
}

// RegenMax reports a heal-over-time effect's client regen gauge.
type RegenMax struct {
	Count, Period int32
	HPRegen       float64
}

// EffectRemovedLackHP reports a toggle damage-over-time effect removed
// because its tick would exceed the remaining HP.
type EffectRemovedLackHP struct{}

// EffectRemovedLackMP reports a toggle mana-damage-over-time effect removed
// because its tick would exceed the remaining MP.
type EffectRemovedLackMP struct{}

// RelaxHPFull reports Relax ending at full HP.
type RelaxHPFull struct{}

// Resource names a restorable vital.
type Resource int

const (
	ResourceHP Resource = iota
	ResourceMP
	ResourceCP
)

// Restored reports Amount of Resource restored, by HealerName when ByOther.
type Restored struct {
	Resource   Resource
	HealerName string
	Amount     int
	ByOther    bool
}

// SpoilResult reports a Spoil outcome: the target was already spoiled, or
// this spoil succeeded.
type SpoilResult struct{ Already bool }

// OverHit reports a kill reward that included an overhit bonus.
type OverHit struct{}

// ServitorVanished reports that the character's servitor was erased.
type ServitorVanished struct{}

// ShieldBlocked reports a successful shield block roll on the defender.
type ShieldBlocked struct{ Perfect bool }

// EffectEndReason is why an active effect ended.
type EffectEndReason int

const (
	EffectWornOff EffectEndReason = iota
	EffectDisappeared
	EffectAborted
)

// EffectEnded reports an active effect ending.
type EffectEnded struct {
	Reason  EffectEndReason
	SkillID modelskill.ID
	Level   int
}

// AttackFailed reports a half-damage magic resist on the caster.
type AttackFailed struct{}

// SkillResisted reports TargetName resisting the caster's skill.
type SkillResisted struct {
	TargetName string
	SkillID    modelskill.ID
	Level      int
}

// MagicResisted reports the target resisting AttackerName's magic.
type MagicResisted struct{ AttackerName string }

// HerbConsumed reports a received herb whose carried skill must be applied.
type HerbConsumed struct{ ItemID int32 }

// AttackRequested reports an aggression effect provoking an attack on Target.
type AttackRequested struct{ Target Object }

// Retargeted reports a domain-driven selection change; a nil Target clears
// the selection.
type Retargeted struct{ Target Object }

// SummonConfirmRequested reports a summon-friend request awaiting the
// character's answer.
type SummonConfirmRequested struct {
	CasterName string
	CasterID   int32
	X, Y, Z    int
	Timeout    time.Duration
}

// TeleportRequested reports a discontinuous relocation to (X, Y, Z) within
// Radius.
type TeleportRequested struct{ X, Y, Z, Radius int }

// Relocated reports that the server-authoritative position moved away from
// Previous.
type Relocated struct{ Previous location.Location }

// PvPFlagged reports a hit that flags the character for PvP; UseFlaggedDuration
// selects the PvP-versus-PvP duration.
type PvPFlagged struct{ UseFlaggedDuration bool }

// RelationChanged reports a PvP flag or karma change observers must see.
type RelationChanged struct{}

// ChargeMessage reports a Force/Soul charge change to the owning client;
// Maxed means the maximum was reached.
type ChargeMessage struct {
	Charges int
	Maxed   bool
}

// ChargesChanged reports that the Force/Soul charge count changed.
type ChargesChanged struct{}

func (VitalsChanged) event()          {}
func (BowDrawn) event()               {}
func (StanceChanged) event()          {}
func (FakeDeathRevived) event()       {}
func (EffectIconsChanged) event()     {}
func (PositionCorrected) event()      {}
func (WeightPenaltyChanged) event()   {}
func (UserInfoChanged) event()        {}
func (GradePenaltyChanged) event()    {}
func (DeathPenaltyChanged) event()    {}
func (ExpSPGained) event()            {}
func (ExpSPLost) event()              {}
func (KarmaChanged) event()           {}
func (LevelChanged) event()           {}
func (LeveledUp) event()              {}
func (ShortBuff) event()              {}
func (RegenMax) event()               {}
func (EffectRemovedLackHP) event()    {}
func (EffectRemovedLackMP) event()    {}
func (RelaxHPFull) event()            {}
func (Restored) event()               {}
func (SpoilResult) event()            {}
func (OverHit) event()                {}
func (ServitorVanished) event()       {}
func (ShieldBlocked) event()          {}
func (EffectEnded) event()            {}
func (AttackFailed) event()           {}
func (SkillResisted) event()          {}
func (MagicResisted) event()          {}
func (HerbConsumed) event()           {}
func (AttackRequested) event()        {}
func (Retargeted) event()             {}
func (SummonConfirmRequested) event() {}
func (TeleportRequested) event()      {}
func (Relocated) event()              {}
func (PvPFlagged) event()             {}
func (RelationChanged) event()        {}
func (ChargeMessage) event()          {}
func (ChargesChanged) event()         {}

// PetSummonRequested reports a SUMMON_CREATURE cast with a pet collar;
// ControlItem is the collar's item instance.
type PetSummonRequested struct{ ControlItem any }

// ServitorSummonRequested reports a non-cubic SUMMON cast of Skill.
type ServitorSummonRequested struct{ Skill modelskill.Definition }

func (PetSummonRequested) event()      {}
func (ServitorSummonRequested) event() {}
