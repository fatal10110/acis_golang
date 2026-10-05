// Package zonepulse runs the periodic tasks of the effect and damage zones.
//
// The first occupant to enter such a zone starts its pulse: after the
// zone's initial delay, then every reuse delay at a fixed rate, a damage
// zone hurts each living occupant (DamageZone.onEnter's task) and an effect
// zone lands its skills on each living occupant that wins the chance roll
// (EffectZone.applyEffect). A pulse that finds its zone empty, or a damage
// zone gone dormant, stops; the next entry starts a fresh one.
//
// Each pulse runs on the queue Wire is given. What it does to one occupant
// runs on that occupant's own queue, when it has one.
package zonepulse

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Objects resolves an occupant's world object by its object id.
type Objects interface {
	Object(id int32) (world.Tracked, bool)
}

// Skills resolves a skill reference to its loaded definition.
type Skills interface {
	Definition(modelskill.Ref) (modelskill.Definition, bool)
}

// Target is an occupant's world object as a pulse acts on it.
type Target interface {
	ObjectID() int32
	Dead() bool
}

// Damageable is a target a damage zone hurts.
type Damageable interface {
	Target
	CalcStat(s stat.Stat, base float64) float64
	// ReduceHPWithoutCastBreak takes the HP of a hit with no cast-break
	// roll: the zone's damage never breaks a cast.
	ReduceHPWithoutCastBreak(amount float64, attacker attackable.Combatant, skill modelskill.Definition)
}

// Effects is how an effect pulse lands one skill on an occupant, the
// occupant being both the skill's caster and its target.
type Effects interface {
	// Conditions reports whether def's conditions hold for target using it
	// on itself, telling target why not when they do not
	// (L2Skill.checkCondition).
	Conditions(target Target, def modelskill.Definition) bool
	// Land lands def's effects on target (L2Skill.getEffects(target,
	// target)).
	Land(target Target, def modelskill.Definition)
}

// Config is what the pulses act through.
type Config struct {
	// Queue runs every pulse; it is the pulses' clock.
	Queue   *sim.Queue
	Objects Objects
	Skills  Skills
	Effects Effects
	// TrapTripped tells the defenders of castleID that one of its traps
	// started hurting; nil tells nobody.
	TrapTripped func(castleID int)
	// Roll returns a number in [0, n); nil rolls rnd.Get.
	Roll func(n int) int
}

// Wire gives every effect and damage zone of ix its pulse. A zone whose
// reuse delay is not positive never pulses: the reference fails to
// schedule such a task. Call it once at boot, before any actor enters a
// zone.
func Wire(ix *zone.Index, cfg Config) {
	if ix == nil || cfg.Queue == nil || cfg.Objects == nil {
		return
	}
	if cfg.Roll == nil {
		cfg.Roll = rnd.Get
	}
	for _, z := range zone.OfKind[*zone.Damage](ix) {
		if z.ReuseDelay <= 0 {
			continue
		}
		p := &pulse{cfg: &cfg, initial: z.InitialDelay, reuse: z.ReuseDelay, stopped: z.PulseStopped}
		p.tick = func() bool { return damageTick(&cfg, z) }
		if z.CastleID > 0 {
			castleID := z.CastleID
			p.started = func() {
				if cfg.TrapTripped != nil {
					cfg.TrapTripped(castleID)
				}
			}
		}
		z.StartPulse = p.start
	}
	if cfg.Skills == nil || cfg.Effects == nil {
		return
	}
	for _, z := range zone.OfKind[*zone.Effect](ix) {
		if z.ReuseDelay <= 0 {
			continue
		}
		p := &pulse{cfg: &cfg, initial: z.InitialDelay, reuse: z.ReuseDelay, stopped: z.PulseStopped}
		p.tick = func() bool { return effectTick(&cfg, z) }
		z.StartPulse = p.start
	}
}

// pulse is one zone's periodic task. Every field but the immutable ones is
// owned by cfg.Queue; the zone's latch keeps a single pulse running.
type pulse struct {
	cfg            *Config
	initial, reuse time.Duration
	// tick runs one pulse and reports whether the task goes on.
	tick func() bool
	// stopped resets the zone's latch, reporting whether an occupant that
	// entered meanwhile needs a fresh pulse.
	stopped func() bool
	// started runs once each task is scheduled; nil for none.
	started func()

	ticker *sim.Ticker
}

// start is the zone's StartPulse hook. It runs inside the zone's entry
// rules, so the task is scheduled from the pulse queue.
func (p *pulse) start() { p.cfg.Queue.Post(p.schedule) }

// schedule arms the task: its first pulse after the initial delay.
func (p *pulse) schedule() {
	p.cfg.Queue.After(max(p.initial, 0), p.first)
	if p.started != nil {
		p.started()
	}
}

// first runs the first pulse, then keeps pulsing every reuse delay.
func (p *pulse) first() {
	if !p.tick() {
		p.stop()
		return
	}
	p.ticker = p.cfg.Queue.Every(p.reuse, p.next)
}

func (p *pulse) next() {
	if p.tick() {
		return
	}
	p.ticker.Stop()
	p.ticker = nil
	p.stop()
}

// stop ends the task, starting a fresh one when an occupant entered after
// the last pulse looked.
func (p *pulse) stop() {
	if p.stopped() {
		p.schedule()
	}
}

// damageTick hurts every living occupant of z, the damage scaled by its
// damage zone vulnerability, and reports whether the task goes on: not
// once z is empty, deals no damage or is a dormant castle trap.
func damageTick(cfg *Config, z *zone.Damage) bool {
	occupants := z.Core().Occupants()
	if len(occupants) == 0 || !z.Live() {
		return false
	}
	hp := float64(z.HPDamage)
	for _, a := range occupants {
		obj, ok := cfg.Objects.Object(a.ObjectID())
		if !ok {
			continue
		}
		t, ok := obj.(Damageable)
		if !ok || t.Dead() {
			continue
		}
		onQueueOf(t, func() {
			if t.Dead() {
				return
			}
			damage := hp * (1 + t.CalcStat(stat.DamageZoneVuln, 0)/100)
			t.ReduceHPWithoutCastBreak(damage, nil, modelskill.Definition{})
		})
	}
	return true
}

// effectTick lands z's skills on every living occupant of z that wins the
// chance roll, and reports whether the task goes on: not once z is empty.
// A disabled zone does nothing and goes on, whether or not anyone is in it.
func effectTick(cfg *Config, z *zone.Effect) bool {
	if !z.Enabled() {
		return true
	}
	occupants := z.Core().Occupants()
	if len(occupants) == 0 {
		return false
	}
	for _, a := range occupants {
		obj, ok := cfg.Objects.Object(a.ObjectID())
		if !ok {
			continue
		}
		t, ok := obj.(Target)
		if !ok || t.Dead() || cfg.Roll(100) >= z.Chance {
			continue
		}
		onQueueOf(t, func() { landSkills(cfg, z, t) })
	}
	return true
}

// landSkills lands each of z's skills on t whose conditions t meets and
// none of whose effects t already holds.
func landSkills(cfg *Config, z *zone.Effect, t Target) {
	for _, ref := range z.Skills {
		def, ok := cfg.Skills.Definition(modelskill.Ref{ID: modelskill.ID(ref.ID), Level: ref.Level})
		if !ok {
			continue
		}
		if !cfg.Effects.Conditions(t, def) || holds(t, ref.ID) {
			continue
		}
		cfg.Effects.Land(t, def)
	}
}

// holds reports whether t holds an effect of skill id.
func holds(t Target, id int) bool {
	h, ok := t.(interface{ EffectList() *effect.List })
	if !ok {
		return false
	}
	_, held := h.EffectList().ActiveBySkillID(id)
	return held
}

// onQueueOf runs fn on t's own queue, or at once when t has none. A closed
// queue drops it: t has left the world.
func onQueueOf(t Target, fn func()) {
	if q, ok := t.(interface{ Queue() *sim.Queue }); ok {
		if queue := q.Queue(); queue != nil {
			queue.Post(fn)
			return
		}
	}
	fn()
}
