package task

import (
	"sort"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/config"
)

// PvPFlagTick is the fixed PvP flag expiry interval.
const PvPFlagTick = time.Second

// PvPFlagState is the client-visible PvP flag state.
type PvPFlagState uint8

const (
	// PvPFlagNone clears the PvP flag.
	PvPFlagNone PvPFlagState = iota
	// PvPFlagOn marks a player as attackable by other players.
	PvPFlagOn
	// PvPFlagBlinking marks a PvP flag close to expiry.
	PvPFlagBlinking
)

// PvPFlagActor is the narrow player surface the PvP flag task updates.
type PvPFlagActor interface {
	ObjectID() int32
	Queued
	UpdatePvPFlag(PvPFlagState)
}

// PvPFlagOptions controls how long each PvP flag source lasts.
type PvPFlagOptions struct {
	Normal              time.Duration
	Flagged             time.Duration
	AwardPKKillPVPPoint bool

	UnsupportedKeys []string
}

// DefaultPvPFlagOptions returns the shipped players.properties defaults.
func DefaultPvPFlagOptions() PvPFlagOptions {
	return PvPFlagOptions{
		Normal:              40 * time.Second,
		Flagged:             20 * time.Second,
		AwardPKKillPVPPoint: true,
	}
}

var unsupportedPvPFlagKeys = []string{
	"CanGMDropEquipment",
	"KarmaPlayerCanShop",
	"KarmaPlayerCanTrade",
	"KarmaPlayerCanUseGK",
	"KarmaPlayerCanUseWareHouse",
	"ListOfNonDroppableItemsForPK",
	"ListOfPetItems",
	"MinimumPKRequiredToDrop",
}

// PvPFlagOptionsFromProperties reads the PvP flag settings from
// players.properties.
func PvPFlagOptionsFromProperties(props *config.Properties) (PvPFlagOptions, error) {
	opts := DefaultPvPFlagOptions()
	if props == nil {
		return opts, nil
	}

	f := config.NewFields(props, "pvp flag options")
	normal := f.Int("PvPVsNormalTime", int(opts.Normal/time.Millisecond))
	flagged := f.Int("PvPVsPvPTime", int(opts.Flagged/time.Millisecond))
	if err := f.Err(); err != nil {
		return PvPFlagOptions{}, err
	}
	opts.Normal = time.Duration(normal) * time.Millisecond
	opts.Flagged = time.Duration(flagged) * time.Millisecond
	opts.AwardPKKillPVPPoint = f.Bool("AwardPKKillPVPPoint", opts.AwardPKKillPVPPoint)

	for _, key := range unsupportedPvPFlagKeys {
		if _, ok := props.Lookup(key); ok {
			opts.UnsupportedKeys = append(opts.UnsupportedKeys, key)
		}
	}
	sort.Strings(opts.UnsupportedKeys)
	return opts, nil
}

// PvPFlags tracks timed PvP flags and clears or blinks them on the fixed
// one-second task.
type PvPFlags struct {
	opts PvPFlagOptions
	now  func() time.Time

	*deadlineRegistry[int32, PvPFlagActor]
}

// NewPvPFlags returns an empty PvP flag tracker.
func NewPvPFlags(opts PvPFlagOptions, now func() time.Time) *PvPFlags {
	if now == nil {
		now = time.Now
	}
	return &PvPFlags{opts: opts, now: now, deadlineRegistry: newDeadlineRegistry[int32, PvPFlagActor]()}
}

// Start launches the fixed one-second PvP flag task.
func (p *PvPFlags) Start(log zerolog.Logger) *scheduler.Ticker {
	return scheduler.Start(PvPFlagTick, p.Tick, log)
}

// AddNormal tracks actor for the configured normal-player timeout.
func (p *PvPFlags) AddNormal(actor PvPFlagActor) {
	p.Add(actor, p.opts.Normal)
	if actor != nil {
		actor.UpdatePvPFlag(PvPFlagOn)
	}
}

// AddFlagged tracks actor for the configured flagged-player timeout.
func (p *PvPFlags) AddFlagged(actor PvPFlagActor) {
	p.Add(actor, p.opts.Flagged)
	if actor != nil {
		actor.UpdatePvPFlag(PvPFlagOn)
	}
}

// Add tracks actor until duration elapses. Re-adding actor replaces the
// previous deadline.
func (p *PvPFlags) Add(actor PvPFlagActor, duration time.Duration) {
	if actor == nil {
		return
	}
	p.add(actor.ObjectID(), actor, p.now().Add(duration))
}

// Remove stops tracking actor. If reset is true, actor's flag is cleared.
func (p *PvPFlags) Remove(actor PvPFlagActor, reset bool) {
	if actor == nil {
		return
	}
	p.remove(actor.ObjectID())
	if reset {
		actor.UpdatePvPFlag(PvPFlagNone)
	}
}

// Tick updates every tracked actor, blinking during the last five seconds
// and clearing the flag only after the deadline has passed.
func (p *PvPFlags) Tick() {
	now := p.now()
	// Both transitions are resolved again on the actor's queue against the
	// deadline this sweep saw, so a flag refreshed ahead of a queued
	// transition keeps its fresh state instead of being cleared or blinked
	// by the stale one.
	p.tickPending(now,
		func(actor PvPFlagActor, expiresAt time.Time) {
			if q := actor.Queue(); q != nil {
				q.Post(func() { p.expire(actor, expiresAt) })
				return
			}
			p.expire(actor, expiresAt)
		},
		func(actor PvPFlagActor, expiresAt time.Time) {
			state := PvPFlagOn
			if now.After(expiresAt.Add(-5 * time.Second)) {
				state = PvPFlagBlinking
			}
			if q := actor.Queue(); q != nil {
				q.Post(func() { p.update(actor, expiresAt, state) })
				return
			}
			p.update(actor, expiresAt, state)
		},
	)
}

// expire clears actor's flag unless its deadline was refreshed after the
// sweep read it.
func (p *PvPFlags) expire(actor PvPFlagActor, expiresAt time.Time) {
	if p.expireIf(actor.ObjectID(), expiresAt) {
		actor.UpdatePvPFlag(PvPFlagNone)
	}
}

// update applies a blink or steady transition unless actor's deadline has
// moved since the sweep computed it.
func (p *PvPFlags) update(actor PvPFlagActor, expiresAt time.Time, state PvPFlagState) {
	if p.hasDeadline(actor.ObjectID(), expiresAt) {
		actor.UpdatePvPFlag(state)
	}
}

// Len returns the number of tracked actors.
func (p *PvPFlags) Len() int {
	return p.len()
}
