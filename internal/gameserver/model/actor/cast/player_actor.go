package cast

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// PlayerActor adapts a live player character to the cast controller's actor
// contract.
type PlayerActor struct {
	Character *player.Character
}

func (a PlayerActor) AttackSpeed(magic bool) int {
	if a.Character == nil {
		return 1
	}
	if magic {
		return a.Character.MagicAttackSpeed()
	}
	return a.Character.AttackSpeed()
}

func (PlayerActor) ReuseRate(bool) float64 { return 1 }

func (a PlayerActor) MP() int {
	if a.Character == nil {
		return 0
	}
	return a.Character.CurrentMP()
}

func (a PlayerActor) HP() int {
	if a.Character == nil {
		return 0
	}
	return a.Character.CurrentHP()
}

func (PlayerActor) MPInitialCost(def modelskill.Definition) int { return def.MPInitialConsume }

func (PlayerActor) MPCost(def modelskill.Definition) int { return def.MPConsume }

func (a PlayerActor) ReduceMP(amount int) {
	if a.Character == nil || amount <= 0 {
		return
	}
	a.Character.ReduceCurrentMP(amount)
}

func (a PlayerActor) ReduceHP(amount int) {
	if a.Character == nil || amount <= 0 {
		return
	}
	a.Character.ReduceCurrentHP(amount)
}

func (a PlayerActor) SkillDisabled(key int32) bool {
	return a.Character != nil && a.Character.SkillDisabled(key)
}

func (a PlayerActor) DisableSkill(key int32, delay time.Duration) {
	if a.Character != nil {
		a.Character.DisableSkill(key, delay)
	}
}

func (a PlayerActor) AddSkillReuse(ref modelskill.Ref, key int32, delay time.Duration) {
	if a.Character != nil {
		a.Character.AddSkillReuse(ref, key, delay)
	}
}

func (PlayerActor) MagicMuted() bool { return false }

func (PlayerActor) PhysicalMuted() bool { return false }

func (PlayerActor) SpiritshotCharged() bool { return false }

func (PlayerActor) BlessedSpiritshotCharged() bool { return false }

func (PlayerActor) SkillMastery(modelskill.Definition) bool { return false }

func (a PlayerActor) ItemCount(itemID int) int {
	if a.Character == nil || a.Character.Inventory() == nil {
		return 0
	}
	return a.Character.Inventory().ItemCount(int32(itemID), -1, true)
}

func (a PlayerActor) ConsumeItem(itemID, count int) bool {
	if a.Character == nil || a.Character.Inventory() == nil {
		return false
	}
	return a.Character.Inventory().DestroyByTemplateID(int32(itemID), count) != nil
}
