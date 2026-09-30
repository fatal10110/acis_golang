package player

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// mountFeedPeriod is the fixed rate a rider's mount eats at.
const mountFeedPeriod = 10 * time.Second

// MountData is a mount's pet data at the level its rider mounted it: how it
// is fed, and how fast it carries its rider.
type MountData struct {
	MaxMeal int
	// MealInNormal is the mount's ride rate out of combat. MealInBattle is
	// its unmounted battle rate: a rider in combat pays that one, not the
	// ride battle rate.
	MealInNormal, MealInBattle int
	Food1, Food2               int32
	AutoFeedLimit              float64
	// HungryLimit is the share of MaxMeal below which a fed mount is
	// hungry and carries its rider at half its speeds.
	HungryLimit float64
	// RunSpeed, SwimSpeed and FlySpeed are the mount's base speeds on land,
	// in water and in the air; AtkSpd is its rider's base P.Atk. speed on a
	// strider.
	RunSpeed, SwimSpeed, FlySpeed int
	AtkSpd                        float64
	// PAtk and MAtk are the rider's base P.Atk. and M.Atk. while mounted.
	PAtk, MAtk float64
}

// MountDataSource resolves a mount's pet data for a rider of level.
type MountDataSource interface {
	MountData(npcID int32, level int) (MountData, bool)
}

// mountFeedState is the feed gauge of the mount a character rides. mu
// guards it: the feed task runs on the rider's queue, while a killer's or a
// reviver's queue stops and restarts it.
type mountFeedState struct {
	mu   sync.Mutex
	data MountData
	// found is whether the ridden mount has pet data for its level at all;
	// without it the rider keeps its own speeds.
	found   bool
	canFeed bool
	// fed is set by the first feed start on this character and never
	// cleared, and current survives a dismount: until a new mount's feed
	// starts, its hunger is judged by the gauge the last mount left.
	fed     bool
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

// hungryLocked reports whether the mount is fed below its hungry limit.
func (f *mountFeedState) hungryLocked() bool {
	return f.fed && float64(f.current) < float64(f.data.MaxMeal)*f.data.HungryLimit
}

func (f *mountFeedState) stopLocked() {
	f.gen++
	if f.ticker != nil {
		f.ticker.Stop()
		f.ticker = nil
	}
}

// loadMountFeed resolves the pet data of the mount npcID for the
// character's current level. A mount with no usable feeding data is never
// fed. The gauge is left as it was until the feed starts.
func (c *Character) loadMountFeed(npcID int32) {
	var data MountData
	found := false
	if c.mountData != nil {
		data, found = c.mountData.MountData(npcID, c.Level())
	}
	ok := found && data.MaxMeal > 0 && data.MealInNormal > 0 && data.MealInBattle > 0
	f := &c.mountFeed
	f.mu.Lock()
	f.stopLocked()
	f.data, f.found, f.canFeed = data, found, ok
	f.mu.Unlock()
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
	f.fed = true
	gauge := f.setCurrentLocked(f.data.MaxMeal, inCombat)
	if q := c.Queue(); q != nil && !c.Dead() {
		gen := f.gen
		f.ticker = q.Every(mountFeedPeriod, func() { c.tickMountFeed(gen) })
	}
	f.mu.Unlock()
	c.refreshMoveSpeed()
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
	c.refreshMoveSpeed()
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
		c.refreshMoveSpeed()
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
// task, and reports whether it was mounted. Leaving a flying mount takes
// Wyvern Breath away.
func (c *Character) Dismount() bool {
	c.stateMu.Lock()
	if c.mountNPCID == 0 {
		c.stateMu.Unlock()
		return false
	}
	wasFlying := c.flying
	c.mountNPCID, c.mountObjectID, c.mountType, c.mountLevel = 0, 0, 0, 0
	c.flying = false
	c.stateMu.Unlock()
	if wasFlying {
		c.SetSkillLevel(int(modelskill.WyvernBreathSkillID), 0)
	}

	f := &c.mountFeed
	f.mu.Lock()
	f.stopLocked()
	f.data, f.found, f.canFeed = MountData{}, false, false
	f.mu.Unlock()
	c.refreshMoveSpeed()
	c.emit(event.Dismounted{})
	return true
}
