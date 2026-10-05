package zonepulse

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// occupant is a zone actor standing at a settable position.
type occupant struct {
	id    int32
	class zone.Class
	at    location.Location
	flags zone.Flags
}

func (o *occupant) ObjectID() int32             { return o.id }
func (o *occupant) Position() location.Location { return o.at }
func (o *occupant) ZoneFlags() *zone.Flags      { return &o.flags }
func (o *occupant) Class() zone.Class           { return o.class }

// creature is the world object of an occupant: it records the damage it
// takes and the skills landed on it.
type creature struct {
	world.Presence
	id   int32
	dead bool
	// vuln is its DAMAGE_ZONE_VULN.
	vuln float64

	mu     sync.Mutex
	hits   []float64
	landed []modelskill.ID
}

func (c *creature) ObjectID() int32  { return c.id }
func (c *creature) Kind() actor.Kind { return actor.KindPlayer }
func (c *creature) Dead() bool       { return c.dead }

func (c *creature) CalcStat(s stat.Stat, base float64) float64 {
	if s == stat.DamageZoneVuln {
		return base + c.vuln
	}
	return base
}

func (c *creature) ReduceHPWithoutCastBreak(amount float64, attacker attackable.Combatant, _ modelskill.Definition) {
	if attacker != nil {
		panic("a zone's damage has no attacker")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hits = append(c.hits, amount)
}

func (c *creature) takeHits() []float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := c.hits
	c.hits = nil
	return h
}

type objects map[int32]world.Tracked

func (o objects) Object(id int32) (world.Tracked, bool) {
	t, ok := o[id]
	return t, ok
}

type skills map[modelskill.Ref]modelskill.Definition

func (s skills) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	d, ok := s[ref]
	return d, ok
}

// effects lands skills by recording them on the creature; refuse fails the
// conditions of the skill ids it holds.
type effects struct{ refuse map[modelskill.ID]bool }

func (e effects) Conditions(_ Target, def modelskill.Definition) bool { return !e.refuse[def.ID] }

func (effects) Land(t Target, def modelskill.Definition) {
	c := t.(*creature)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.landed = append(c.landed, def.ID)
}

func (c *creature) takeLanded() []modelskill.ID {
	c.mu.Lock()
	defer c.mu.Unlock()
	l := c.landed
	c.landed = nil
	return l
}

func box(t *testing.T) zone.Form {
	t.Helper()
	f, err := zone.NewCuboid(0, 1_000, 0, 1_000, -100, 100)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func statSet(kv ...string) *commons.StatSet {
	set := commons.NewStatSet()
	for i := 0; i < len(kv); i += 2 {
		set.Set(kv[i], kv[i+1])
	}
	return set
}

var (
	inside  = location.Location{X: 500, Y: 500}
	outside = location.Location{X: 1_500, Y: 500}
)

// move puts o at loc and revalidates its zones.
func move(ix *zone.Index, o *occupant, loc location.Location) {
	o.at = loc
	ix.Revalidate(o)
}

func assertHits(t *testing.T, when string, c *creature, want ...float64) {
	t.Helper()
	got := c.takeHits()
	if len(got) != len(want) {
		t.Fatalf("%s: hits = %v, want %v", when, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: hits = %v, want %v", when, got, want)
		}
	}
}

// TestDamagePulseHurtsOccupantsAtAFixedRate pins DamageZone.onEnter's task
// (DamageZone.java:63-90): the first entry schedules it at initialDelay,
// then every reuseDelay; each pulse takes hpDamage * (1 + DAMAGE_ZONE_VULN /
// 100) from every living occupant; a dead one is skipped; the pulse that
// finds the zone empty stops the task, and the next entry starts a fresh one
// at initialDelay again.
func TestDamagePulseHurtsOccupantsAtAFixedRate(t *testing.T) {
	z, err := zone.NewDamage(1, box(t), statSet("hpDamage", "40", "initialDelay", "1000", "reuseDelay", "5000"))
	if err != nil {
		t.Fatal(err)
	}
	ix := zone.NewIndex()
	ix.Add(z)
	in := sim.NewInline(time.Unix(0, 0))
	victim := &creature{id: 1, vuln: 50}
	corpse := &creature{id: 2, dead: true}
	Wire(ix, Config{Queue: in.NewQueue("pulses"), Objects: objects{1: victim, 2: corpse}})

	a := &occupant{id: 1, class: zone.ClassPlayer}
	b := &occupant{id: 2, class: zone.ClassPlayer}
	move(ix, a, inside)
	move(ix, b, inside)

	in.Advance(999 * time.Millisecond)
	assertHits(t, "before the initial delay", victim)
	in.Advance(time.Millisecond)
	assertHits(t, "at the initial delay", victim, 60)
	in.Advance(4_999 * time.Millisecond)
	assertHits(t, "before the reuse delay", victim)
	in.Advance(time.Millisecond)
	assertHits(t, "at the reuse delay", victim, 60)
	if got := corpse.takeHits(); len(got) != 0 {
		t.Fatalf("dead occupant took %v", got)
	}

	move(ix, a, outside)
	move(ix, b, outside)
	in.Advance(10 * time.Second)
	assertHits(t, "after leaving", victim)

	// The empty zone stopped its task: re-entering starts a fresh one, at
	// initialDelay from the entry.
	move(ix, a, inside)
	in.Advance(999 * time.Millisecond)
	assertHits(t, "re-entered, before the initial delay", victim)
	in.Advance(time.Millisecond)
	assertHits(t, "re-entered, at the initial delay", victim, 60)
}

// TestDamagePulseStopsForADormantCastleTrap pins the castle trap rules: a
// trap whose castle's siege is not in progress never starts; once armed
// during its siege, the first entry tells the defenders it tripped
// (DamageZone.java:86-88); and the trap going dormant stops the task.
func TestDamagePulseStopsForADormantCastleTrap(t *testing.T) {
	z, err := zone.NewDamage(1, box(t), statSet("castleId", "5", "hpDamage", "10", "initialDelay", "0", "reuseDelay", "1000"))
	if err != nil {
		t.Fatal(err)
	}
	var siege bool
	z.SiegeActive = func() bool { return siege }
	z.Armed = true
	ix := zone.NewIndex()
	ix.Add(z)
	in := sim.NewInline(time.Unix(0, 0))
	victim := &creature{id: 1}
	var tripped []int
	Wire(ix, Config{
		Queue: in.NewQueue("pulses"), Objects: objects{1: victim},
		TrapTripped: func(castleID int) { tripped = append(tripped, castleID) },
	})

	a := &occupant{id: 1, class: zone.ClassPlayer}
	move(ix, a, inside)
	in.Advance(3 * time.Second)
	assertHits(t, "dormant trap", victim)
	if len(tripped) != 0 {
		t.Fatalf("dormant trap tripped %v", tripped)
	}

	siege = true
	move(ix, a, outside)
	move(ix, a, inside)
	in.Advance(0)
	if len(tripped) != 1 || tripped[0] != 5 {
		t.Fatalf("trap trips = %v, want one for castle 5", tripped)
	}
	assertHits(t, "live trap, first pulse", victim, 10)
	in.Advance(time.Second)
	assertHits(t, "live trap, second pulse", victim, 10)

	siege = false
	in.Advance(5 * time.Second)
	assertHits(t, "trap gone dormant", victim)
	siege = true
	in.Advance(5 * time.Second)
	assertHits(t, "dormant trap's task stays stopped", victim)
}

// TestPulseRestartsForAnEntryRacingItsStop pins the latch handoff: when
// the stopping task's latch reset reports an occupant that entered after its
// last look (and so started nothing), the stop schedules a fresh task at
// the initial delay.
func TestPulseRestartsForAnEntryRacingItsStop(t *testing.T) {
	in := sim.NewInline(time.Unix(0, 0))
	var ticks []time.Duration
	start := in.Now()
	raced := true
	p := &pulse{
		cfg: &Config{Queue: in.NewQueue("pulses")}, initial: time.Second, reuse: 3 * time.Second,
		// The first pulse finds the zone empty; the later ones find the
		// racing occupant.
		tick: func() bool {
			ticks = append(ticks, in.Now().Sub(start))
			return len(ticks) > 1
		},
		stopped: func() bool {
			r := raced
			raced = false
			return r
		},
	}
	p.start()
	in.Advance(10 * time.Second)
	want := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 8 * time.Second}
	if len(ticks) != len(want) {
		t.Fatalf("pulses at %v, want %v", ticks, want)
	}
	for i := range want {
		if ticks[i] != want[i] {
			t.Fatalf("pulses at %v, want %v", ticks, want)
		}
	}
}

// TestEffectPulseLandsSkillsOnTheTargetScope pins EffectZone.applyEffect
// (EffectZone.java:258-283): each pulse rolls the chance per living
// occupant and lands each listed skill whose conditions hold and none of
// whose effects it holds; an occupant outside the target scope is never in
// the zone; a disabled zone skips its pulses without stopping; and the pulse
// that finds the zone empty stops the task.
func TestEffectPulseLandsSkillsOnTheTargetScope(t *testing.T) {
	z, err := zone.NewEffect(1, box(t), statSet("targetType", "Npc", "skill", "4644-2;4645-3", "chance", "70", "initialDelay", "1000", "reuseDelay", "6000"))
	if err != nil {
		t.Fatal(err)
	}
	ix := zone.NewIndex()
	ix.Add(z)
	in := sim.NewInline(time.Unix(0, 0))
	monster := &creature{id: 1}
	player := &creature{id: 2}
	rolls := []int{69, 70}
	roll := func(n int) int {
		if n != 100 {
			t.Fatalf("roll over %d, want 100", n)
		}
		r := rolls[0]
		rolls = append(rolls[1:], r)
		return r
	}
	Wire(ix, Config{
		Queue: in.NewQueue("pulses"), Objects: objects{1: monster, 2: player},
		Skills: skills{
			{ID: 4644, Level: 2}: {ID: 4644, Level: 2},
			{ID: 4645, Level: 3}: {ID: 4645, Level: 3},
		},
		Effects: effects{refuse: map[modelskill.ID]bool{4645: true}},
		Roll:    roll,
	})

	npcOccupant := &occupant{id: 1, class: zone.ClassNPC}
	playerOccupant := &occupant{id: 2, class: zone.ClassPlayer}
	move(ix, npcOccupant, inside)
	move(ix, playerOccupant, inside)
	if z.Core().Inside(playerOccupant) {
		t.Fatal("a player is an occupant of an NPC-scoped effect zone")
	}

	in.Advance(time.Second)
	if got := monster.takeLanded(); len(got) != 1 || got[0] != 4644 {
		t.Fatalf("first pulse (roll 69 < 70) landed %v, want [4644]: 4645's conditions fail", got)
	}
	if got := player.takeLanded(); len(got) != 0 {
		t.Fatalf("player outside the scope got %v", got)
	}
	in.Advance(6 * time.Second)
	if got := monster.takeLanded(); len(got) != 0 {
		t.Fatalf("second pulse (roll 70, not < 70) landed %v, want nothing", got)
	}

	z.SetEnabled(false)
	in.Advance(12 * time.Second)
	if got := monster.takeLanded(); len(got) != 0 {
		t.Fatalf("disabled zone landed %v", got)
	}
	z.SetEnabled(true)
	in.Advance(6 * time.Second)
	if got := monster.takeLanded(); len(got) != 1 {
		t.Fatalf("re-enabled zone landed %v, want one skill: the disabled pulses kept the task", got)
	}

	move(ix, npcOccupant, outside)
	in.Advance(6 * time.Second)
	// Stopped: the next entry starts a fresh task at initialDelay.
	move(ix, npcOccupant, inside)
	rolls = []int{0}
	in.Advance(999 * time.Millisecond)
	if got := monster.takeLanded(); len(got) != 0 {
		t.Fatalf("re-entered, before the initial delay: landed %v", got)
	}
	in.Advance(time.Millisecond)
	if got := monster.takeLanded(); len(got) != 1 {
		t.Fatalf("re-entered, at the initial delay: landed %v, want one skill", got)
	}
}
