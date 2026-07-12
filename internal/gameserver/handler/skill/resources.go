package skill

type hpPercentTarget interface {
	CanBeHealed() bool
	MaxHP() float64
	AddHP(float64) float64
}

type mpPercentTarget interface {
	CanBeHealed() bool
	MaxMP() float64
	AddMP(float64) float64
}

type manaHealTarget interface {
	CanBeHealed() bool
	AddMP(float64) float64
}

type rechargeTarget interface {
	RechargeMP(float64) float64
}

type cpHealTarget interface {
	Dead() bool
	Invulnerable() bool
	CP() float64
	MaxCP() float64
	SetCP(float64)
}

type balanceLifeTarget interface {
	Dead() bool
	HP() float64
	MaxHP() float64
	SetHP(float64)
}

type expSPTarget interface {
	AddExpAndSP(exp, sp int)
}

type realDamageTarget interface {
	Dead() bool
	HP() float64
	SetHP(float64)
	Die(killer any)
}

type healPercentHandler struct{}

func (healPercentHandler) Types() []string { return []string{"HEAL_PERCENT", "MANAHEAL_PERCENT"} }

func (healPercentHandler) Use(cast Cast) {
	if skillTypeKey(cast.Skill.SkillType) == "HEAL_PERCENT" {
		for _, obj := range cast.Targets {
			target, ok := obj.(hpPercentTarget)
			if !ok || !target.CanBeHealed() {
				continue
			}
			target.AddHP(target.MaxHP() * float64(cast.Skill.Power) / 100)
		}
		return
	}

	for _, obj := range cast.Targets {
		target, ok := obj.(mpPercentTarget)
		if !ok || !target.CanBeHealed() {
			continue
		}
		target.AddMP(target.MaxMP() * float64(cast.Skill.Power) / 100)
	}
}

type manaHealHandler struct{}

func (manaHealHandler) Types() []string { return []string{"MANAHEAL", "MANARECHARGE"} }

func (manaHealHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := obj.(manaHealTarget)
		if !ok || !target.CanBeHealed() {
			continue
		}
		mp := float64(cast.Skill.Power)
		if skillTypeKey(cast.Skill.SkillType) == "MANARECHARGE" {
			if r, ok := obj.(rechargeTarget); ok {
				mp = r.RechargeMP(mp)
			}
		}
		target.AddMP(mp)
	}
}

type combatPointHealHandler struct{}

func (combatPointHealHandler) Types() []string { return []string{"COMBATPOINTHEAL"} }

func (combatPointHealHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := obj.(cpHealTarget)
		if !ok || target.Dead() || target.Invulnerable() {
			continue
		}
		amount := float64(cast.Skill.Power)
		if target.CP()+amount > target.MaxCP() {
			amount = target.MaxCP() - target.CP()
		}
		target.SetCP(target.CP() + amount)
	}
}

type balanceLifeHandler struct{}

func (balanceLifeHandler) Types() []string { return []string{"BALANCE_LIFE"} }

func (balanceLifeHandler) Use(cast Cast) {
	targets := make([]balanceLifeTarget, 0, len(cast.Targets))
	var fullHP, currentHP float64
	casterCursed := cursed(cast.Caster)

	for _, obj := range cast.Targets {
		target, ok := obj.(balanceLifeTarget)
		if !ok || target.Dead() {
			continue
		}
		if !sameObject(obj, cast.Caster) && (casterCursed || cursed(obj)) {
			continue
		}
		fullHP += target.MaxHP()
		currentHP += target.HP()
		targets = append(targets, target)
	}

	if len(targets) == 0 || fullHP == 0 {
		return
	}

	ratio := currentHP / fullHP
	for _, target := range targets {
		target.SetHP(target.MaxHP() * ratio)
	}
}

type giveSPHandler struct{}

func (giveSPHandler) Types() []string { return []string{"GIVE_SP"} }

func (giveSPHandler) Use(cast Cast) {
	sp := int(cast.Skill.Power)
	for _, obj := range cast.Targets {
		if target, ok := obj.(expSPTarget); ok {
			target.AddExpAndSP(0, sp)
		}
	}
}

type realDamageHandler struct{}

func (realDamageHandler) Types() []string { return []string{"REAL_DAMAGE"} }

func (realDamageHandler) Use(cast Cast) {
	for _, obj := range cast.Targets {
		target, ok := obj.(realDamageTarget)
		if !ok || target.Dead() {
			continue
		}
		hpLeft := target.HP() - float64(cast.Skill.Power)
		if hpLeft <= 0 {
			target.Die(cast.Caster)
			continue
		}
		target.SetHP(hpLeft)
	}
}
