package event

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// VitalsChanged reports a change to the actor's current HP, MP or CP.
type VitalsChanged struct{}

// ActionsStopRequested asks for the named in-progress actions to be stopped
// by the server rather than the client: the target selection, then
// movement, then the attack, then the cast. AIDenied reports that the
// character was already unable to take AI actions before the effect that
// requested the stop landed.
type ActionsStopRequested struct{ ClearTarget, Move, Attack, Cast, AIDenied bool }

// IdleRequested asks for the character to be sent idle, the way an effect
// that disables it does. AIDenied reports that the character was already
// unable to take AI actions before that effect landed.
type IdleRequested struct{ AIDenied bool }

// ThinkRequested asks for the character's current intention to be
// re-evaluated once.
type ThinkRequested struct{}

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

// PostureSettled reports that a sit-down or stand-up transition ended and
// the character takes control back.
type PostureSettled struct{}

// FusionCastersStopRequested asks that every other character channelling a
// fusion skill on this character stop its cast.
type FusionCastersStopRequested struct{}

func (FusionCastersStopRequested) event() {}

// DeathSettled reports that a character's death sequence has applied every
// cost that follows its Died: killer credit, charges, experience/karma loss,
// and the death-penalty level.
type DeathSettled struct{}

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

// RunSpeedChanged reports that a stat func change moved the character's
// RUN_SPEED, so its own and its observers' views of it are stale.
type RunSpeedChanged struct{}

// EffectsStripped reports that a stop-all ended the character's effects
// without reporting each one's stat change: its own and its observers'
// full views of it are stale.
type EffectsStripped struct{}

// StatsModified reports a stat func change that left RUN_SPEED alone: the
// character's own view is stale, and Attrs holds the new P.Atk. and cast
// speeds, one per changed func on those stats, its observers must see.
type StatsModified struct{ Attrs []StatusAttr }

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

// SPChanged reports that an SP addition went through. SP is the total it
// left, read under the progression lock.
type SPChanged struct{ SP int }

// ExpSPLost reports one experience/SP removal. SPLeft is the SP the
// removal left, read under the progression lock.
type ExpSPLost struct {
	Exp    int64
	SP     int
	SPLeft int
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

// EffectFelt reports an added effect taking over its stack group.
type EffectFelt struct {
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

// DamageReceived reports a hit another creature dealt the character:
// AttackerName and the full damage before CP absorbed any of it.
type DamageReceived struct {
	AttackerName string
	Amount       int
}

// ServitorDamageShared reports a hit the character, or its summon, dealt
// that its target's servitor took a share of: TargetDamage is what the
// target took and ServitorDamage the servitor's share.
type ServitorDamageShared struct {
	TargetDamage   int
	ServitorDamage int
}

// SkillDamageDealt reports a skill hit's damage to the attacking character,
// for hits delivered outside a cast's own handler result. Blocked marks an
// invulnerable target and Petrified one that is also paralyzed.
type SkillDamageDealt struct {
	Amount    int
	MagicCrit bool
	Blocked   bool
	Petrified bool
}

// HerbConsumed reports a received herb whose carried skill must be applied.
type HerbConsumed struct{ ItemID int32 }

// ItemObtained reports items that reached the character's inventory with
// a chat line naming them. Notice picks the line and how its parameters are
// typed; EnchantLevel is read only by ObtainPickup.
type ItemObtained struct {
	ItemID       int32
	Count        int
	EnchantLevel int
	Notice       ObtainNotice
}

// ObtainNotice picks the chat line an ItemObtained sends.
type ObtainNotice uint8

const (
	// ObtainAdena names the adena amount earned.
	ObtainAdena ObtainNotice = iota
	// ObtainPickup names an existing item taken into the inventory, such
	// as one picked up off the ground: a stack's count as a plain number,
	// or a single enchanted item's enchant level.
	ObtainPickup
	// ObtainCreated names an item created by template id, such as an
	// auto-looted kill reward or an opened capsule's product: a stack's
	// count as an item number.
	ObtainCreated
	// ObtainEarned names an item earned by template id, such as a swept
	// spoil: a stack's count as an item number.
	ObtainEarned
)

// AttackRequested reports an aggression effect provoking an attack on Target.
type AttackRequested struct{ Target world.Tracked }

// FleeRequested asks the character to run Distance units directly away from
// From, in run stance, the way a server-driven move is requested. AIDenied
// reports that the character was already unable to take AI actions before the
// effect in progress landed: the request is then refused.
type FleeRequested struct {
	From     location.Location
	Distance int
	AIDenied bool
}

// Retargeted reports a domain-driven selection change; a nil Target clears
// the selection.
type Retargeted struct{ Target world.Tracked }

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
// selects the PvP-versus-PvP duration. ByServitor marks a hit or skill of
// the character's summon, reported from the summon's queue rather than the
// character's own.
type PvPFlagged struct{ UseFlaggedDuration, ByServitor bool }

// RelationChanged reports a PvP flag or karma change observers must see.
type RelationChanged struct{}

// PKKarmaGained reports that a kill made the character a PKer and its
// karma gain has been announced: its equipped items must meet their
// conditions again, and its PvP flag ends.
type PKKarmaGained struct{}

// ChargeMessage reports a Force/Soul charge change to the owning client;
// Maxed means the maximum was reached.
type ChargeMessage struct {
	Charges int
	Maxed   bool
}

// ChargesChanged reports that the Force/Soul charge count changed.
type ChargesChanged struct{}

func (VitalsChanged) event()          {}
func (ActionsStopRequested) event()   {}
func (IdleRequested) event()          {}
func (ThinkRequested) event()         {}
func (BowDrawn) event()               {}
func (StanceChanged) event()          {}
func (FakeDeathRevived) event()       {}
func (PostureSettled) event()         {}
func (DeathSettled) event()           {}
func (EffectIconsChanged) event()     {}
func (PositionCorrected) event()      {}
func (WeightPenaltyChanged) event()   {}
func (UserInfoChanged) event()        {}
func (RunSpeedChanged) event()        {}
func (EffectsStripped) event()        {}
func (StatsModified) event()          {}
func (GradePenaltyChanged) event()    {}
func (DeathPenaltyChanged) event()    {}
func (ExpSPGained) event()            {}
func (ExpSPLost) event()              {}
func (SPChanged) event()              {}
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
func (EffectFelt) event()             {}
func (AttackFailed) event()           {}
func (SkillResisted) event()          {}
func (MagicResisted) event()          {}
func (DamageReceived) event()         {}
func (ServitorDamageShared) event()   {}
func (SkillDamageDealt) event()       {}
func (HerbConsumed) event()           {}
func (ItemObtained) event()           {}
func (AttackRequested) event()        {}
func (FleeRequested) event()          {}
func (Retargeted) event()             {}
func (SummonConfirmRequested) event() {}
func (TeleportRequested) event()      {}
func (Relocated) event()              {}
func (PvPFlagged) event()             {}
func (RelationChanged) event()        {}
func (PKKarmaGained) event()          {}
func (ChargeMessage) event()          {}
func (ChargesChanged) event()         {}

// PetSummonRequested reports a SUMMON_CREATURE cast with a pet collar;
// ControlItem is the collar's item instance.
type PetSummonRequested struct{ ControlItem any }

// ServitorSummonRequested reports a non-cubic SUMMON cast of Skill.
type ServitorSummonRequested struct{ Skill modelskill.Definition }

func (PetSummonRequested) event()      {}
func (ServitorSummonRequested) event() {}

// ReviveRequested reports a resurrection offer awaiting the character's
// answer; ReviverName is who offers it (the character itself for a Phoenix
// Blessing).
type ReviveRequested struct{ ReviverName string }

// ReviveRefusal names why a resurrection offer was not made.
type ReviveRefusal int

const (
	// ReviveAlreadyProposed: the same kind of offer (character or pet) is
	// already pending.
	ReviveAlreadyProposed ReviveRefusal = iota + 1
	// RevivePetWhileOwnerPending: a pet offer while the owner's own is
	// pending.
	RevivePetWhileOwnerPending
	// ReviveOwnerWhilePetPending: an offer for the owner while its pet's is
	// pending.
	ReviveOwnerWhilePetPending
)

// ReviveRefused reports to the would-be reviver that its offer was refused.
type ReviveRefused struct{ Reason ReviveRefusal }

// Revived reports a dead character standing back up, after its HP and MP
// were restored.
type Revived struct{}

// EtcStatusChanged reports a change to the flags the client status window
// shows (charges, penalties, charm of courage).
type EtcStatusChanged struct{}

// EtcStatusBroadcast reports a change to those flags that the character's
// observers are told about as well.
type EtcStatusBroadcast struct{}

func (ReviveRequested) event()    {}
func (ReviveRefused) event()      {}
func (Revived) event()            {}
func (EtcStatusChanged) event()   {}
func (EtcStatusBroadcast) event() {}

// MountFeedGauge reports the ridden mount's feed gauge, in the client's
// gauge units.
type MountFeedGauge struct{ Current, Max int }

// MountFoodDue reports that the ridden mount is hungry and eats the food
// item ObjectID from its rider's inventory.
type MountFoodDue struct{ ObjectID int32 }

// Dismounted reports the character getting off its mount.
type Dismounted struct{}

// MountOutOfFeed reports that the mount threw its rider for lack of feed.
// WasFlying is whether the rider was flying on it.
type MountOutOfFeed struct{ WasFlying bool }

func (MountFeedGauge) event() {}
func (MountFoodDue) event()   {}
func (Dismounted) event()     {}
func (MountOutOfFeed) event() {}
