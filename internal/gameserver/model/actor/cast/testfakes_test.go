package cast

import (
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type testTarget struct{}

func (testTarget) ObjectID() int32         { return 1 }
func (testTarget) Position() (x, y, z int) { return 0, 0, 0 }

type testActor struct {
	mp, hp int

	mAtkSpd, pAtkSpd                  int
	magicReuseRate, physicalReuseRate float64
	initialCost, hitCost              int
	spiritshot, blessedSpiritshot     bool
	magicMuted, physicalMuted         bool
	mastery                           bool

	items        map[int]int
	disabledKeys map[int32]bool
	disabled     []testCooldown
	reuses       []testReuse

	cubicFull   bool
	allDisabled bool
}

func (a *testActor) CubicListFull() bool { return a.cubicFull }

func (a *testActor) AllSkillsDisabled() bool { return a.allDisabled }
func (a *testActor) EnableAllSkills()        { a.allDisabled = false }

type testCooldown struct {
	key   int32
	delay time.Duration
}

type testReuse struct {
	ref   modelskill.Ref
	key   int32
	delay time.Duration
}

func (a *testActor) AttackSpeed(magic bool) int {
	if magic {
		if a.mAtkSpd == 0 {
			return 333
		}
		return a.mAtkSpd
	}
	if a.pAtkSpd == 0 {
		return 333
	}
	return a.pAtkSpd
}

func (a *testActor) ReuseRate(magic bool) float64 {
	if magic {
		if a.magicReuseRate == 0 {
			return 1
		}
		return a.magicReuseRate
	}
	if a.physicalReuseRate == 0 {
		return 1
	}
	return a.physicalReuseRate
}

func (a *testActor) MP() int { return a.mp }
func (a *testActor) HP() int { return a.hp }

func (a *testActor) MPInitialCost(def modelskill.Definition) int {
	if a.initialCost != 0 {
		return a.initialCost
	}
	return def.MPInitialConsume
}

func (a *testActor) MPCost(def modelskill.Definition) int {
	if a.hitCost != 0 {
		return a.hitCost
	}
	return def.MPConsume
}

func (a *testActor) ReduceMP(n int) { a.mp -= n }
func (a *testActor) ReduceHP(n int) { a.hp -= n }

func (a *testActor) SkillDisabled(key int32) bool {
	return a.disabledKeys[key]
}

func (a *testActor) DisableSkill(key int32, delay time.Duration) {
	a.disabled = append(a.disabled, testCooldown{key: key, delay: delay})
}

func (a *testActor) AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration) {
	a.reuses = append(a.reuses, testReuse{ref: ref, key: key, delay: delay})
}

func (a *testActor) MagicMuted() bool               { return a.magicMuted }
func (a *testActor) PhysicalMuted() bool            { return a.physicalMuted }
func (a *testActor) SpiritshotCharged() bool        { return a.spiritshot }
func (a *testActor) BlessedSpiritshotCharged() bool { return a.blessedSpiritshot }
func (a *testActor) SkillMastery(modelskill.Definition) bool {
	return a.mastery
}

func (a *testActor) ItemCount(itemID int) int {
	if a.items == nil {
		return 0
	}
	return a.items[itemID]
}

func (a *testActor) ConsumeItem(itemID, count int) bool {
	if a.items == nil || a.items[itemID] < count {
		return false
	}
	a.items[itemID] -= count
	return true
}
