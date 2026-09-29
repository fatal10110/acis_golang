package summon

import (
	"math"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// A baby pet heals its owner on its own: once it has been in the world for
// babyHealDelay, it considers a heal every babyHealPeriod.
const (
	babyHealDelay  = 3 * time.Second
	babyHealPeriod = time.Second

	// babyWeakHealID is the heal a baby pet may use on an owner below
	// babyWeakHealBelow of max HP, when babyWeakHealChance comes up.
	babyWeakHealID     modelskill.ID = 4717
	babyWeakHealChance               = 25
	babyWeakHealBelow                = 0.8
	// babyStrongHealID is the heal it may use instead on an owner below
	// babyStrongHealBelow, when babyStrongHealChance comes up.
	babyStrongHealID     modelskill.ID = 4718
	babyStrongHealChance               = 75
	babyStrongHealBelow                = 0.15
)

// babyHealTask is a baby pet's owner-heal ticker. Its ticks run on the
// pet's queue. mu guards ticker and gone: the task is started on spawn and
// revive and stopped on death and departure, and death can come from the
// queue of whoever landed the killing hit.
type babyHealTask struct {
	mu     sync.Mutex
	ticker *sim.Ticker
	// gone is set once the pet has left the world; the task never starts
	// again after that.
	gone bool
}

// IsBabyPet reports whether a is a baby pet, which heals its owner.
func (a *Actor) IsBabyPet() bool { return a.babyPet }

// startBabyHeal starts a living baby pet's owner-heal task on its queue,
// unless it already runs.
func (a *Actor) startBabyHeal() {
	if !a.babyPet {
		return
	}
	t := &a.babyHeal
	t.mu.Lock()
	defer t.mu.Unlock()
	// Read under mu, so a death that stops the task after marking the pet
	// dead cannot be overtaken by a start that saw it alive.
	if t.gone || t.ticker != nil || a.Dead() {
		return
	}
	q := a.Queue()
	if q == nil {
		return
	}
	// The first heal comes babyHealDelay after the start, then one every
	// babyHealPeriod: a period ticker whose first ticks are skipped keeps
	// the whole task on one fixed-rate grid behind a single handle. The
	// count is touched only by this ticker's own runs, all on q.
	skip := int(babyHealDelay/babyHealPeriod) - 1
	t.ticker = q.Every(babyHealPeriod, func() {
		if skip > 0 {
			skip--
			return
		}
		a.babyHealTick()
	})
}

// stopBabyHeal stops a baby pet's owner-heal task, if it runs. gone also
// keeps it from ever starting again.
func (a *Actor) stopBabyHeal(gone bool) {
	if !a.babyPet {
		return
	}
	t := &a.babyHeal
	t.mu.Lock()
	defer t.mu.Unlock()
	if gone {
		t.gone = true
	}
	if t.ticker != nil {
		t.ticker.Stop()
		t.ticker = nil
	}
}

// babyHealTick is one owner-heal decision. Nothing happens while the owner
// is missing, dead or invulnerable. Otherwise the weak heal comes up on
// babyWeakHealChance and is cast when the owner is below babyWeakHealBelow
// of max HP; failing that, the strong heal comes up on babyStrongHealChance
// and is cast below babyStrongHealBelow. Each heal also needs to be off its
// reuse delay and within the pet's MP. A heal cast is handed to the pet's AI
// and its owner reads PET_USES_S1 naming it, whatever the AI then makes of
// the request.
func (a *Actor) babyHealTick() {
	owner := a.currentOwner()
	if owner == nil || owner.Dead() || owner.Invul() {
		return
	}
	// The ratio is over max HP in whole points, as the client sees it.
	maxHP := math.Floor(owner.MaxHPValue())
	if maxHP <= 0 {
		return
	}
	hpRatio := owner.HP() / maxHP

	if a.Roll(100) <= babyWeakHealChance && a.tryBabyHeal(owner, babyWeakHealID, hpRatio < babyWeakHealBelow) {
		return
	}
	if a.Roll(100) <= babyStrongHealChance {
		a.tryBabyHeal(owner, babyStrongHealID, hpRatio < babyStrongHealBelow)
	}
}

// tryBabyHeal casts heal skillID on owner, at the level a's own level gives
// it, when wounded holds and the heal is ready and affordable, and reports
// whether it did.
func (a *Actor) tryBabyHeal(owner Owner, skillID modelskill.ID, wounded bool) bool {
	if a.skillDefs == nil || a.brain == nil {
		return false
	}
	ref := modelskill.Ref{ID: skillID, Level: petmodel.BabyPetSkillLevel(a.Level())}
	def, ok := a.skillDefs.Definition(ref)
	if !ok {
		return false
	}
	if (a.cast != nil && a.cast.SkillOnCooldown(def)) || a.MPValue() < float64(def.MPConsume) || !wounded {
		return false
	}
	a.brain.TryToCast(owner, ref, false)
	a.emit(event.PetUsedSkill{SkillID: int32(ref.ID), Level: int32(ref.Level)})
	return true
}
