package player

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// mountFeedPeriod is the fixed rate a rider's mount eats at.
const mountFeedPeriod = 10 * time.Second

// MountFeed is a mount's feeding data at the level its rider mounted it.
type MountFeed struct {
	MaxMeal int
	// MealInNormal is the mount's ride rate out of combat. MealInBattle is
	// its unmounted battle rate: a rider in combat pays that one, not the
	// ride battle rate.
	MealInNormal, MealInBattle int
	Food1, Food2               int32
	AutoFeedLimit              float64
}

// MountFeeds resolves a mount's feeding data for a rider of level.
type MountFeeds interface {
	MountFeed(npcID int32, level int) (MountFeed, bool)
}

// mountFeedState is the feed gauge of the mount a character rides. mu
// guards it: the feed task runs on the rider's queue, while a killer's or a
// reviver's queue stops and restarts it.
type mountFeedState struct {
	mu      sync.Mutex
	data    MountFeed
	canFeed bool
	current int
	ticker  *sim.Ticker
	// gen tells a tick of a stopped task from one of the running task.
	gen uint64
}

// consumeLocked is the meal one feed tick takes.
func (f *mountFeedState) consumeLocked(inCombat bool) int {
	if inCombat {
		return f.data.MealInBattle
	}
	return f.data.MealInNormal
}

// setCurrentLocked sets the gauge, capped at the mount's max meal, and
// returns the gauge the client is shown for it.
func (f *mountFeedState) setCurrentLocked(n int, inCombat bool) event.MountFeedGauge {
	f.current = min(n, f.data.MaxMeal)
	return f.gaugeLocked(inCombat)
}

func (f *mountFeedState) gaugeLocked(inCombat bool) event.MountFeedGauge {
	consume := f.consumeLocked(inCombat)
	return event.MountFeedGauge{
		Current: f.current * 10000 / consume,
		Max:     f.data.MaxMeal * 10000 / consume,
	}
}

func (f *mountFeedState) stopLocked() {
	f.gen++
	if f.ticker != nil {
		f.ticker.Stop()
		f.ticker = nil
	}
}

// loadMountFeed resolves the feeding data of the mount npcID for the
// character's current level. A mount with no usable data is never fed.
func (c *Character) loadMountFeed(npcID int32) {
	var data MountFeed
	ok := false
	if c.mountFeeds != nil {
		data, ok = c.mountFeeds.MountFeed(npcID, c.Level())
	}
	ok = ok && data.MaxMeal > 0 && data.MealInNormal > 0 && data.MealInBattle > 0
	f := &c.mountFeed
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopLocked()
	f.data, f.canFeed, f.current = data, ok, 0
}

// StartMountFeed fills the ridden mount's gauge and shows it, then, unless
// the rider is dead, starts the task that feeds the mount every
// mountFeedPeriod. It runs once the mount transition was shown, and again
// when a dead rider is revived: the revived rider's mount is full again.
func (c *Character) StartMountFeed() {
	if !c.Mounted() {
		return
	}
	inCombat := c.InCombat()
	f := &c.mountFeed
	f.mu.Lock()
	if !f.canFeed {
		f.mu.Unlock()
		return
	}
	f.stopLocked()
	gauge := f.setCurrentLocked(f.data.MaxMeal, inCombat)
	if q := c.Queue(); q != nil && !c.Dead() {
		gen := f.gen
		f.ticker = q.Every(mountFeedPeriod, func() { c.tickMountFeed(gen) })
	}
	f.mu.Unlock()
	// Setting the gauge shows it, and starting the feed shows it again.
	c.emit(gauge)
	c.emit(gauge)
}

// stopMountFeed stops the feed task. The gauge keeps its value.
func (c *Character) stopMountFeed() {
	f := &c.mountFeed
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopLocked()
}

// AddMountFeed raises the ridden mount's gauge by amount, capped at its max
// meal, and shows it. It does nothing for a mount that is never fed.
func (c *Character) AddMountFeed(amount int) {
	if !c.Mounted() {
		return
	}
	inCombat := c.InCombat()
	f := &c.mountFeed
	f.mu.Lock()
	if !f.canFeed {
		f.mu.Unlock()
		return
	}
	gauge := f.setCurrentLocked(f.current+amount, inCombat)
	f.mu.Unlock()
	c.emit(gauge)
}

// tickMountFeed is one run of the feed task started as generation gen. The
// mount eats; a mount with too little left for the meal throws its rider,
// who is told the mount left for lack of feed. Otherwise a mount below its
// auto-feed limit eats its food from the rider's inventory.
func (c *Character) tickMountFeed(gen uint64) {
	if !c.Mounted() {
		c.stopMountFeed()
		return
	}
	inCombat := c.InCombat()
	f := &c.mountFeed
	f.mu.Lock()
	if f.gen != gen {
		f.mu.Unlock()
		return
	}
	if consume := f.consumeLocked(inCombat); f.current > consume {
		gauge := f.setCurrentLocked(f.current-consume, inCombat)
		food1, food2 := f.data.Food1, f.data.Food2
		hungry := float64(f.current) < float64(f.data.MaxMeal)*f.data.AutoFeedLimit
		f.mu.Unlock()
		c.emit(gauge)
		c.autoFeedMount(food1, food2, hungry)
		return
	}
	gauge := f.setCurrentLocked(0, inCombat)
	f.stopLocked()
	f.mu.Unlock()
	c.emit(gauge)

	wasFlying := c.Flying()
	c.Dismount()
	c.emit(event.MountOutOfFeed{WasFlying: wasFlying})
}

// autoFeedMount has a hungry mount eat its first food, then its second,
// from the rider's inventory.
func (c *Character) autoFeedMount(food1, food2 int32, hungry bool) {
	if c.inventory == nil {
		return
	}
	food := c.inventory.ItemByTemplateID(food1)
	if food == nil && food2 != 0 {
		food = c.inventory.ItemByTemplateID(food2)
	}
	if food == nil || !hungry {
		return
	}
	c.emit(event.MountFoodDue{ObjectID: food.ObjectID})
}

// Dismount takes the character off its mount and stops the mount's feed
// task, and reports whether it was mounted.
func (c *Character) Dismount() bool {
	c.stateMu.Lock()
	if c.mountNPCID == 0 {
		c.stateMu.Unlock()
		return false
	}
	c.mountNPCID, c.mountObjectID, c.mountType = 0, 0, 0
	c.flying = false
	c.stateMu.Unlock()

	f := &c.mountFeed
	f.mu.Lock()
	f.stopLocked()
	f.data, f.canFeed = MountFeed{}, false
	f.mu.Unlock()
	c.emit(event.Dismounted{})
	return true
}
